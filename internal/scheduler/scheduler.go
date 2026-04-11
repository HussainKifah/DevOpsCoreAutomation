package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	huawei "github.com/Flafl/DevOpsCore/internal/excessCommands/Huawei"
	nokiabackup "github.com/Flafl/DevOpsCore/internal/excessCommands/Nokia"
	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
	"github.com/go-co-op/gocron/v2"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
)

type Scheduler struct {
	cfg            *config.Config
	hub            *websocket.Hub
	pool           *shell.ConnectionPool
	powerRepo      repository.PowerRepository
	descRepo       repository.DescriptionRepository
	healthRepo     repository.HealthRepository
	healthHistRepo repository.HealthHistoryRepository
	portRepo       repository.PortProtectionRepository
	portHistRepo   repository.PortHistoryRepository
	backupRepo     repository.BackupRepository
	inventoryRepo  repository.InventoryRepository
	db             *gorm.DB // purge soft-deleted volatile tombstones after each power/desc/port job

	healthBuf   map[string]*models.HealthSnapshot
	healthBufMu sync.Mutex
	scanSem     chan struct{}
}

func New(
	cfg *config.Config,
	hub *websocket.Hub,
	pool *shell.ConnectionPool,
	pr repository.PowerRepository,
	dr repository.DescriptionRepository,
	hr repository.HealthRepository,
	hhr repository.HealthHistoryRepository,
	pp repository.PortProtectionRepository,
	phr repository.PortHistoryRepository,
	br repository.BackupRepository,
	ir repository.InventoryRepository,
	database *gorm.DB,
) *Scheduler {
	return &Scheduler{
		cfg:            cfg,
		hub:            hub,
		pool:           pool,
		powerRepo:      pr,
		descRepo:       dr,
		healthRepo:     hr,
		healthHistRepo: hhr,
		portRepo:       pp,
		portHistRepo:   phr,
		backupRepo:     br,
		inventoryRepo:  ir,
		db:             database,
		healthBuf:      make(map[string]*models.HealthSnapshot),
		scanSem:        make(chan struct{}, 1),
	}
}

func (s *Scheduler) Start() {
	sched, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("scheduler: %v", err)
	}

	// Nokia jobs
	mustAdd(sched, s.cfg.PowerScanInterval, s.runPowerScan, "power-scan")
	mustAdd(sched, s.cfg.DescScanInterval, s.runDescScan, "desc-scan")
	mustAdd(sched, s.cfg.HealthScanInterval, s.runHealthScan, "health-scan")
	mustAdd(sched, s.cfg.PortScanInterval, s.runPortScan, "port-scan")

	// Huawei jobs (same intervals as Nokia)
	mustAdd(sched, s.cfg.HealthScanInterval, s.runHuaweiHealthScan, "huawei-health-scan")
	mustAdd(sched, s.cfg.PowerScanInterval, s.runHuaweiPowerScan, "huawei-power-scan")
	mustAdd(sched, s.cfg.PortScanInterval, s.runHuaweiPortScan, "huawei-port-scan")

	mustAddCron(sched, "0 21 * * *", s.runBackup, "backup")       // 9:00 PM daily
	mustAddCron(sched, "0 21 * * *", s.runHuaweiBackup, "backup") // 9:00 PM daily
	mustAddCron(sched, "0 1 * * *", s.runHistoryCleanup, "history-cleanup")
	mustAddCron(sched, "0 2 1 * *", s.runInventoryScan, "inventory-scan") // Runs at 02:00 on the 1st of every month
	mustAddCron(sched, "0 3 1 * *", s.runOntInterfaceScan, "ont-interface-scan")
	mustAddCron(sched, "0 2 1 * *", s.runHuaweiInventoryScan, "huawei-inventory-scan")

	sched.Start()
	log.Println("scheduler started")

	// Run all jobs immediately in background without blocking
	go func() {
		log.Println("[startup] running all jobs immediately")
		// Nokia
		// s.runOntInterfaceScan()
		// s.runPowerScan()
		// s.runDescScan()
		// s.runHealthScan()
		// s.runPortScan()
		// s.runBackup()
		// Huawei
		// s.runHuaweiHealthScan()
		// s.runHuaweiPowerScan()
		// s.runHuaweiPortScan()
		// s.runHuaweiInventoryScan()
		log.Println("[startup] initial scan complete")
	}()
}

func mustAdd(sched gocron.Scheduler, interval time.Duration, fn func(), name string) {
	_, err := sched.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(fn),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		log.Fatalf("scheduler: add job %s: %v", name, err)
	}
}

func mustAddCron(sched gocron.Scheduler, cronExpr string, fn func(), name string) {
	nextRun := nextCronTime(cronExpr)
	_, err := sched.NewJob(
		gocron.CronJob(cronExpr, false),
		gocron.NewTask(fn),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
		gocron.WithStartAt(gocron.WithStartDateTime(nextRun)),
	)
	if err != nil {
		log.Fatalf("scheduler: add cron job %s: %v", name, err)
	}
	log.Printf("scheduled cron job %q: %s (first run at %s)", name, cronExpr, nextRun.Format(time.RFC3339))
}

func nextCronTime(cronExpr string) time.Time {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	sched, err := parser.Parse(cronExpr)
	if err != nil {
		log.Fatalf("parse cron %q: %v", cronExpr, err)
	}
	return sched.Next(time.Now())
}

// purgeVolatileSoftDeleted removes soft-deleted rows from live OLT tables (power, descriptions,
// port protection, current health). Runs after each matching job; health uses upsert so bloat is
// rare, but any soft-deleted olt_health rows are stripped here too.
func (s *Scheduler) purgeVolatileSoftDeleted(jobName string) {
	if s.db == nil {
		return
	}
	pw, pd, pp, ph, err := repository.PurgeSoftDeletedVolatileRows(s.db)
	if err != nil {
		log.Printf("[job] %s: purge soft-deleted volatile rows: %v", jobName, err)
		return
	}
	if pw+pd+pp+ph > 0 {
		log.Printf("[job] %s: purged soft-deleted tombstones: power=%d descriptions=%d port_protection=%d olt_health=%d", jobName, pw, pd, pp, ph)
	}
}

// ═══════════════════════════════════════════════════════════════════════
//  Nokia jobs (existing)
// ═══════════════════════════════════════════════════════════════════════

func (s *Scheduler) runPowerScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runPowerScanWork()
}

func (s *Scheduler) runPowerScanWork() {
	defer s.purgeVolatileSoftDeleted("power-scan")
	log.Println("[job] power-scan: starting")
	cmd := "show equipment ont optics"

	var batches []repository.PowerBatch

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] power-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		powers := extractor.ExtractAllOntPower(r.Data)
		if len(powers) == 0 {
			continue
		}

		records := make([]models.PowerReading, len(powers))
		for i, p := range powers {
			records[i] = models.PowerReading{
				OntIdx: p.OntIdx,
				OltRx:  p.OltRx,
				OntRx:  p.OntRx,
			}
		}

		batches = append(batches, repository.PowerBatch{
			Device:  r.Device,
			Site:    r.Site,
			Host:    r.Host,
			Records: records,
		})
	}

	if len(batches) == 0 {
		log.Println("[job] power-scan: no results")
		return
	}

	log.Printf("[job] power-scan: collected %d OLTs, writing to DB", len(batches))
	if err := s.powerRepo.ReplaceAll(batches); err != nil {
		log.Printf("[job] power-scan: replace all failed: %v", err)
	} else {
		hosts := make([]string, len(batches))
		for i, b := range batches {
			hosts[i] = b.Host
		}
		if err := s.powerRepo.DeleteExceptHosts(hosts); err != nil {
			log.Printf("[job] power-scan: prune stale hosts failed: %v", err)
		}
	}

	s.notify("power_update")
	log.Println("[job] power-scan: done")
}

func (s *Scheduler) runDescScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runDescScanWork()
}

func (s *Scheduler) runDescScanWork() {
	defer s.purgeVolatileSoftDeleted("desc-scan")
	log.Println("[job] desc-scan: starting")
	cmd := "show equipment ont status pon"

	var batches []repository.DescBatch

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] desc-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		descs := extractor.ExtractAllDesc(r.Data)
		if len(descs) == 0 {
			continue
		}

		records := make([]models.OntDescription, len(descs))
		for i, d := range descs {
			records[i] = models.OntDescription{
				OntIdx: d.OntIdx,
				Desc1:  d.Desc1,
				Desc2:  d.Desc2,
				Vendor: "nokia",
			}
		}

		batches = append(batches, repository.DescBatch{
			Device:  r.Device,
			Site:    r.Site,
			Host:    r.Host,
			Records: records,
		})
	}

	if len(batches) == 0 {
		log.Println("[job] desc-scan: no results")
		return
	}

	log.Printf("[job] desc-scan: collected %d OLTs, writing to DB", len(batches))
	if err := s.descRepo.ReplaceAll(batches); err != nil {
		log.Printf("[job] desc-scan: replace all failed: %v", err)
	} else {
		hosts := make([]string, len(batches))
		for i, b := range batches {
			hosts[i] = b.Host
		}
		if err := s.descRepo.DeleteExceptHosts(hosts); err != nil {
			log.Printf("[job] desc-scan: prune stale hosts failed: %v", err)
		}
	}

	s.notify("desc_update")
	log.Println("[job] desc-scan: done")
}

func (s *Scheduler) runHealthScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runHealthScanWork()
}

func (s *Scheduler) runHealthScanWork() {
	defer s.purgeVolatileSoftDeleted("health-scan")
	log.Println("[job] health-scan: starting")
	cmds := []string{
		"show system cpu-load detail",
		"show core1-uptime",
		"show equipment temperature",
	}

	type healthResult struct {
		record *models.OltHealth
		snap   *models.HealthSnapshot
	}
	var collected []healthResult

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmds...) {
		if r.Err != nil {
			log.Printf("[job] health-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		h := extractor.ExtractHealth(r.Data)

		cpuJSON, _ := json.Marshal(h.CpuLoads)
		tempJSON, _ := json.Marshal(h.Temperatures)

		var cpuSlice models.JSONSlice
		var tempSlice models.JSONSlice
		json.Unmarshal(cpuJSON, &cpuSlice)
		json.Unmarshal(tempJSON, &tempSlice)

		uptime := h.Uptime
		if uptime == "" {
			if existing, err := s.healthRepo.GetByHost(r.Host, "nokia"); err == nil && existing != nil {
				uptime = existing.Uptime
			}
		}

		now := time.Now()
		collected = append(collected, healthResult{
			record: &models.OltHealth{
				Device:       r.Device,
				Site:         r.Site,
				Host:         r.Host,
				Vendor:       "nokia",
				Uptime:       uptime,
				CpuLoads:     cpuSlice,
				Temperatures: tempSlice,
				MeasuredAt:   now,
			},
			snap: &models.HealthSnapshot{
				Device:       r.Device,
				Site:         r.Site,
				Host:         r.Host,
				Vendor:       "nokia",
				Uptime:       uptime,
				CpuLoads:     cpuSlice,
				Temperatures: tempSlice,
				MeasuredAt:   now,
			},
		})
	}

	if len(collected) == 0 {
		log.Println("[job] health-scan: no results")
		return
	}

	log.Printf("[job] health-scan: collected %d OLTs, writing to DB", len(collected))

	records := make([]*models.OltHealth, len(collected))
	for i, c := range collected {
		records[i] = c.record
	}
	if err := s.healthRepo.BulkUpsert(records); err != nil {
		log.Printf("[job] health-scan: bulk upsert failed: %v", err)
	}

	s.healthBufMu.Lock()
	var toInsert []*models.HealthSnapshot
	for _, c := range collected {
		prev, hasPrev := s.healthBuf[c.snap.Host]
		if !hasPrev {
			s.healthBuf[c.snap.Host] = c.snap
		} else {
			toInsert = append(toInsert, averageSnapshots(prev, c.snap))
			delete(s.healthBuf, c.snap.Host)
		}
	}
	s.healthBufMu.Unlock()

	if len(toInsert) > 0 {
		if err := s.healthHistRepo.BulkInsert(toInsert); err != nil {
			log.Printf("[job] health-scan: history bulk insert failed: %v", err)
		}
	}

	s.notify("health_update")
	log.Println("[job] health-scan: done")
}

// TryRun* acquires the semaphore synchronously; if busy returns false.
// On success the actual work runs in a background goroutine.
func (s *Scheduler) RunHealthScan() bool    { return s.tryRun(s.runHealthScanWork) }
func (s *Scheduler) RunPowerScan() bool     { return s.tryRun(s.runPowerScanWork) }
func (s *Scheduler) RunPortScan() bool      { return s.tryRun(s.runPortScanWork) }
func (s *Scheduler) RunInventoryScan() bool { return s.tryRun(s.runInventoryScanWork) }
func (s *Scheduler) RunBackup()             { go s.runBackup() }

func (s *Scheduler) tryRun(work func()) bool {
	select {
	case s.scanSem <- struct{}{}:
		go func() {
			defer func() { <-s.scanSem }()
			work()
		}()
		return true
	default:
		return false
	}
}

func (s *Scheduler) FlushHealthBuffer() {
	s.healthBufMu.Lock()
	defer s.healthBufMu.Unlock()
	for host, snap := range s.healthBuf {
		if err := s.healthHistRepo.Insert(snap); err != nil {
			log.Printf("[shutdown] flush history %s: %v", host, err)
		}
		delete(s.healthBuf, host)
	}
	log.Println("[shutdown] flushed health buffer")
}

func averageSnapshots(a, b *models.HealthSnapshot) *models.HealthSnapshot {
	return &models.HealthSnapshot{
		Device:       a.Device,
		Site:         a.Site,
		Host:         a.Host,
		Vendor:       a.Vendor,
		Uptime:       b.Uptime,
		CpuLoads:     averageJSONSlice(a.CpuLoads, b.CpuLoads, "average_pct"),
		Temperatures: averageJSONSlice(a.Temperatures, b.Temperatures, "act_temp"),
		MeasuredAt:   b.MeasuredAt,
	}
}

func averageJSONSlice(a, b models.JSONSlice, numericField string) models.JSONSlice {
	if len(a) != len(b) {
		return b
	}

	result := make(models.JSONSlice, len(a))
	for i := range a {
		aMap, aOk := a[i].(map[string]any)
		bMap, bOk := b[i].(map[string]any)
		if !aOk || !bOk {
			result[i] = b[i]
			continue
		}

		merged := make(map[string]any)
		for k, v := range bMap {
			merged[k] = v
		}

		aVal := toFloat(aMap[numericField])
		bVal := toFloat(bMap[numericField])
		merged[numericField] = (aVal + bVal) / 2.0

		result[i] = merged
	}
	return result
}

func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func (s *Scheduler) runHistoryCleanup() {
	cutoff := time.Now().AddDate(0, -1, 0)

	healthDel, err := s.healthHistRepo.DeleteOlderThan(cutoff)
	if err != nil {
		log.Printf("[job] history-cleanup: health ERROR %v", err)
	} else {
		log.Printf("[job] history-cleanup: deleted %d old health snapshots", healthDel)
	}

	portDel, err := s.portHistRepo.DeleteOlderThan(cutoff)
	if err != nil {
		log.Printf("[job] history-cleanup: port ERROR %v", err)
	} else {
		log.Printf("[job] history-cleanup: deleted %d old port snapshots", portDel)
	}
}

func (s *Scheduler) runPortScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runPortScanWork()
}

func (s *Scheduler) runPortScanWork() {
	defer s.purgeVolatileSoftDeleted("port-scan")
	log.Println("[job] port-scan: starting")
	cmd := "show port-protection"
	now := time.Now()

	nokia, _, err := shell.OLTsData()
	if err != nil {
		log.Printf("[job] port-scan: OLT list failed: %v", err)
		return
	}
	expectedOLTs := len(nokia)

	var portBatches []repository.PortBatch
	var allHistorySnaps []models.PortSnapshot

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] port-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		ports := extractor.ExtractPortProtection(r.Data)

		var filtered []models.PortProtectionRecord
		for _, p := range ports {
			if strings.Contains(p.PortState, "down") || strings.Contains(p.PairedState, "down") {
				filtered = append(filtered, models.PortProtectionRecord{
					Port:        p.Port,
					PairedPort:  p.PairedPort,
					PortState:   p.PortState,
					PairedState: p.PairedState,
					SwoReason:   p.SwoReason,
					NumSwo:      p.NumSwo,
					Vendor:      "nokia",
				})
				allHistorySnaps = append(allHistorySnaps, models.PortSnapshot{
					Device:      r.Device,
					Site:        r.Site,
					Host:        r.Host,
					Vendor:      "nokia",
					Port:        p.Port,
					PairedPort:  p.PairedPort,
					PortState:   p.PortState,
					PairedState: p.PairedState,
					SwoReason:   p.SwoReason,
					NumSwo:      p.NumSwo,
					MeasuredAt:  now,
				})
			}
		}

		portBatches = append(portBatches, repository.PortBatch{
			Device:  r.Device,
			Site:    r.Site,
			Host:    r.Host,
			Records: filtered,
		})
	}

	if len(portBatches) == 0 {
		log.Println("[job] port-scan: no results")
		return
	}

	successCount := len(portBatches)
	successRate := 1.0
	if expectedOLTs > 0 {
		successRate = float64(successCount) / float64(expectedOLTs)
	}
	if successRate < 0.75 && expectedOLTs > 5 {
		log.Printf("[job] port-scan: only %d/%d OLTs responded (%.0f%%), skipping DB update to preserve data", successCount, expectedOLTs, successRate*100)
		s.notify("port_update")
		return
	}

	log.Printf("[job] port-scan: collected %d/%d OLTs, writing to DB", successCount, expectedOLTs)
	if err := s.portRepo.ReplaceAll(portBatches); err != nil {
		log.Printf("[job] port-scan: replace all failed: %v", err)
	} else {
		hosts := make([]string, len(portBatches))
		for i, b := range portBatches {
			hosts[i] = b.Host
		}
		if err := s.portRepo.DeleteExceptHosts(hosts); err != nil {
			log.Printf("[job] port-scan: prune stale hosts failed: %v", err)
		}
	}

	if len(allHistorySnaps) > 0 {
		if err := s.portHistRepo.BulkInsert(allHistorySnaps); err != nil {
			log.Printf("[job] port-scan: history bulk insert failed: %v", err)
		}
	}

	s.notify("port_update")
	log.Println("[job] port-scan: done")
}

func (s *Scheduler) runBackup() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	log.Println("[job] backup: starting")

	results := nokiabackup.BackupsWithPool(s.pool, false, nil)
	for _, r := range results {
		if r.Err != nil {
			log.Printf("[job] backup: %s: %v", r.Host, r.Err)
		}
		if r.FilePath == "" {
			continue
		}
		site := strings.ReplaceAll(r.Site, "/", "-")
		if site == "" {
			site = "unknown"
		}
		if err := s.backupRepo.Create(&models.OltBackups{
			Device:   r.Device,
			Site:     site,
			Host:     r.Host,
			Vendor:   "nokia",
			FilePath: r.FilePath,
		}); err != nil {
			log.Printf("[job] backup: db %s: %v", r.Host, err)
		}
	}
	s.notify("backup_update")
	log.Println("[job] backup: done")
}

func (s *Scheduler) runInventoryScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runInventoryScanWork()
}

func (s *Scheduler) runInventoryScanWork() {
	log.Println("[job] inventory-scan: starting")
	if err := s.inventoryRepo.DeleteInventorySnapshot("nokia"); err != nil {
		log.Printf("[job] inventory-scan: failed to delete old inventory data: %v", err)
		return
	}

	cmd := "show equipment ont interface detail"

	totals := make(map[string]int)
	firstVendor := make(map[string]string)
	var order []string
	var oltInventories []models.OltInventory
	now := time.Now()

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] inventory-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}

		counts := extractor.CountEquipIDs(r.Data)
		oltTotal := 0
		vendorTotals := make(map[string]int)
		var vendorOrder []string

		for _, c := range counts {
			oltTotal += c.Count

			if totals[c.ID] == 0 {
				order = append(order, c.ID)
			}
			if firstVendor[c.ID] == "" {
				firstVendor[c.ID] = c.VendorDisplay()
			}
			totals[c.ID] += c.Count

			// For this specific OLT
			vendor := c.VendorDisplay()
			if vendorTotals[vendor] == 0 {
				vendorOrder = append(vendorOrder, vendor)
			}
			vendorTotals[vendor] += c.Count
		}

		oltVendorCounts := make([]extractor.VendorCount, 0, len(vendorOrder))
		for _, v := range vendorOrder {
			oltVendorCounts = append(oltVendorCounts, extractor.VendorCount{Vendor: v, Count: vendorTotals[v]})
		}

		swVerCounts := extractor.CountBySwVerAct(r.Data)

		// Save per-ONT model/serial for devices tab
		perOnt := extractor.ExtractPerOntInventory(r.Data)
		var ontItems []models.OntInventoryItem
		needFallback := false
		for _, p := range perOnt {
			if p.OntIdx != "" {
				ontItems = append(ontItems, models.OntInventoryItem{OntIdx: p.OntIdx, EquipID: p.EquipID, SerialNo: p.SerialNo})
			} else {
				needFallback = true
				ontItems = append(ontItems, models.OntInventoryItem{OntIdx: "", EquipID: p.EquipID, SerialNo: p.SerialNo})
			}
		}
		// Fallback: when ont-id is missing from Nokia output, match by position using ont_idx from power readings
		if needFallback && len(ontItems) > 0 {
			ontIdxList, err := s.powerRepo.GetOntIndicesByHost(r.Host)
			if err == nil && len(ontIdxList) > 0 {
				for i := range ontItems {
					if ontItems[i].OntIdx == "" && i < len(ontIdxList) {
						ontItems[i].OntIdx = ontIdxList[i]
					}
				}
			}
		}
		if err := s.inventoryRepo.ReplaceOntInventoryByHost(r.Host, "nokia", ontItems); err != nil {
			log.Printf("[job] inventory-scan: per-ONT inventory for %s: %v", r.Host, err)
		}

		oltInventories = append(oltInventories, models.OltInventory{
			Host:         r.Host,
			Device:       r.Device,
			Site:         r.Site,
			Vendor:       "nokia",
			Counts:       counts,
			VendorCounts: oltVendorCounts,
			SwVerCounts:  swVerCounts,
			Total:        oltTotal,
			MeasuredAt:   now,
		})
	}

	if err := s.inventoryRepo.SaveOltInventory(oltInventories); err != nil {
		log.Printf("[job] inventory-scan: failed to save OLT inventories: %v", err)
	}

	globalCounts := make([]extractor.EquipIDCount, 0, len(order))
	globalTotal := 0
	globalVendorTotals := make(map[string]int)
	var globalVendorOrder []string

	for _, id := range order {
		c := totals[id]
		vendor := firstVendor[id]
		if vendor == "" {
			vendor = "Unknown"
		}
		globalCounts = append(globalCounts, extractor.EquipIDCount{ID: id, Count: c, Vendor: vendor})
		globalTotal += c

		if globalVendorTotals[vendor] == 0 {
			globalVendorOrder = append(globalVendorOrder, vendor)
		}
		globalVendorTotals[vendor] += c
	}

	globalVendorCounts := make([]extractor.VendorCount, 0, len(globalVendorOrder))
	for _, v := range globalVendorOrder {
		globalVendorCounts = append(globalVendorCounts, extractor.VendorCount{Vendor: v, Count: globalVendorTotals[v]})
	}

	// Aggregate software version counts from all OLTs
	swVerTotals := make(map[string]int)
	swVerFirst := make(map[string]*extractor.SwVerCount)
	var swVerOrder []string
	for _, olt := range oltInventories {
		for _, sv := range olt.SwVerCounts {
			if swVerTotals[sv.SwVerAct] == 0 {
				swVerOrder = append(swVerOrder, sv.SwVerAct)
				svCopy := sv
				swVerFirst[sv.SwVerAct] = &svCopy
			}
			swVerTotals[sv.SwVerAct] += sv.Count
		}
	}
	globalSwVerCounts := make([]extractor.SwVerCount, 0, len(swVerOrder))
	for _, ver := range swVerOrder {
		c := swVerTotals[ver]
		r := extractor.SwVerCount{SwVerAct: ver, Count: c}
		if f := swVerFirst[ver]; f != nil && f.Vendor != "" {
			r.Vendor = f.Vendor
		}
		globalSwVerCounts = append(globalSwVerCounts, r)
	}

	summary := &models.InventorySummary{
		Vendor:       "nokia",
		Count:        globalCounts,
		VendorCounts: globalVendorCounts,
		SwVerCounts:  globalSwVerCounts,
		Total:        globalTotal,
		MeasuredAt:   now,
	}

	if err := s.inventoryRepo.SaveSummary(summary); err != nil {
		log.Printf("[job] inventory-scan: failed to save summary: %v", err)
	}

	s.notify("inventory_update")
	log.Println("[job] inventory-scan: done")
}

func (s *Scheduler) runOntInterfaceScan() {
	s.scanSem <- struct{}{}
	defer func() { <-s.scanSem }()
	s.runOntInterfaceScanWork()
}

func (s *Scheduler) runOntInterfaceScanWork() {
	log.Println("[job] ont-interface-scan: starting")
	if err := s.inventoryRepo.DeleteOntInterfaces("nokia"); err != nil {
		log.Printf("[job] ont-interface-scan: failed to delete old ONT interface data: %v", err)
		return
	}

	cmd := "show equipment ont interface"
	total := 0
	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] ont-interface-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		rows := mapNokiaOntInterfaces(extractor.ExtractOntInterfaces(r.Data))
		total += len(rows)
		if err := s.inventoryRepo.ReplaceOntInterfacesByHost(r.Device, r.Site, r.Host, "nokia", rows); err != nil {
			log.Printf("[job] ont-interface-scan: store %s: %v", r.Host, err)
		}
	}
	s.notify("inventory_update")
	log.Printf("[job] ont-interface-scan: done rows=%d", total)
}

func mapNokiaOntInterfaces(rows []extractor.OntInterface) []models.OntInterface {
	out := make([]models.OntInterface, 0, len(rows))
	for _, row := range rows {
		out = append(out, models.OntInterface{
			OntIdx:         row.OntIdx,
			EqptVerNum:     row.EqptVerNum,
			SwVerAct:       row.SwVerAct,
			ActualNumSlots: row.ActualNumSlots,
			VersionNumber:  row.VersionNumber,
			SerNum:         row.SerNum,
			YpSerialNo:     row.YpSerialNo,
			CfgFile1VerAct: row.CfgFile1VerAct,
			CfgFile2VerAct: row.CfgFile2VerAct,
		})
	}
	return out
}

// ═══════════════════════════════════════════════════════════════════════
//  Huawei jobs
// ═══════════════════════════════════════════════════════════════════════

func (s *Scheduler) RunHuaweiHealthScan()    { go s.runHuaweiHealthScan() }
func (s *Scheduler) RunHuaweiPowerScan()     { go s.runHuaweiPowerScan() }
func (s *Scheduler) RunHuaweiPortScan()      { go s.runHuaweiPortScan() }
func (s *Scheduler) RunHuaweiBackup()        { go s.runHuaweiBackup() }
func (s *Scheduler) RunHuaweiInventoryScan() { go s.runHuaweiInventoryScan() }

func (s *Scheduler) runHuaweiHealthScan() {
	log.Println("[job] huawei-health-scan: starting")
	results := huawei.CollectHealth(s.cfg.HuaweiOLTUser, s.cfg.HuaweiOLTPass)
	if len(results) == 0 {
		log.Println("[job] huawei-health-scan: no results")
		return
	}

	now := time.Now()
	var records []*models.OltHealth
	var snapshots []*models.HealthSnapshot

	for _, r := range results {
		if r.Err != "" && len(r.Health.CpuLoads) == 0 && len(r.Health.Temperatures) == 0 {
			log.Printf("[job] huawei-health-scan: skip %s: %s", r.Host, r.Err)
			continue
		}

		cpuSlice := make(models.JSONSlice, len(r.Health.CpuLoads))
		for i, c := range r.Health.CpuLoads {
			cpuSlice[i] = map[string]any{"slot": c.Slot, "average_pct": float64(c.CpuUsage)}
		}
		tempSlice := make(models.JSONSlice, len(r.Health.Temperatures))
		for i, t := range r.Health.Temperatures {
			tempSlice[i] = map[string]any{"slot": t.Slot, "act_temp": float64(t.TempC)}
		}

		uptime := r.Health.Uptime
		if uptime == "" {
			if existing, err := s.healthRepo.GetByHost(r.Host, "huawei"); err == nil && existing != nil {
				uptime = existing.Uptime
			}
		}

		rec := &models.OltHealth{
			Device:       r.Device,
			Site:         r.Site,
			Host:         r.Host,
			Vendor:       "huawei",
			Uptime:       uptime,
			CpuLoads:     cpuSlice,
			Temperatures: tempSlice,
			MeasuredAt:   now,
		}
		records = append(records, rec)

		snapshots = append(snapshots, &models.HealthSnapshot{
			Device:       r.Device,
			Site:         r.Site,
			Host:         r.Host,
			Vendor:       "huawei",
			Uptime:       uptime,
			CpuLoads:     cpuSlice,
			Temperatures: tempSlice,
			MeasuredAt:   now,
		})
	}

	if len(records) == 0 {
		log.Println("[job] huawei-health-scan: no valid results")
		return
	}

	log.Printf("[job] huawei-health-scan: collected %d OLTs, writing to DB", len(records))
	if err := s.healthRepo.BulkUpsert(records); err != nil {
		log.Printf("[job] huawei-health-scan: bulk upsert failed: %v", err)
	}

	if len(snapshots) > 0 {
		if err := s.healthHistRepo.BulkInsert(snapshots); err != nil {
			log.Printf("[job] huawei-health-scan: history insert failed: %v", err)
		}
	}

	s.notify("health_update")
	log.Println("[job] huawei-health-scan: done")
}

func (s *Scheduler) runHuaweiPowerScan() {
	log.Println("[job] huawei-power-scan: starting")
	olts := shell.GetHuaweiOLTs()
	if len(olts) == 0 {
		log.Println("[job] huawei-power-scan: no Huawei OLTs")
		return
	}
	log.Printf("[job] huawei-power-scan: %d Huawei OLT(s) — ONT power journal: %s", len(olts), huawei.HuaweiPowerOntJournalFile())
	log.Printf("[job] huawei-power-scan: per-OLT raw/tsv under logs/huawei/huawei_<ip>_<timestamp>_*.log")

	var batches []repository.PowerBatch
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, olt := range olts {
		olt := olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := huawei.CollectOpticalPowers(olt.Ip, s.cfg.HuaweiOLTUser, s.cfg.HuaweiOLTPass, olt.Name, olt.Site)
			if err != nil {
				log.Printf("[job] huawei-power-scan: %s (%s): %v", olt.Name, olt.Ip, err)
			}
			if res != nil {
				log.Printf("[job] huawei-power-scan: %s (%s) — readings=%d cmds=%d chunks ok=%d fail=%d | raw=%s | tsv=%s",
					olt.Name, olt.Ip, len(res.Powers), res.CommandsRun, res.ChunksOK, res.ChunksFailed,
					res.RawLogPath, res.PowersLogPath)
			}
			if res == nil || len(res.Powers) == 0 {
				return
			}

			records := make([]models.PowerReading, len(res.Powers))
			for i, p := range res.Powers {
				records[i] = models.PowerReading{
					OntIdx: fmt.Sprintf("0/%d/%d/%d", p.Slot, p.Pon, p.Ont),
					OntRx:  p.RxPower,
					OltRx:  p.OltRxOntPower,
					Vendor: "huawei",
				}
			}

			mu.Lock()
			batches = append(batches, repository.PowerBatch{
				Device:  res.Device,
				Site:    res.Site,
				Host:    res.Host,
				Records: records,
			})
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(batches) == 0 {
		log.Println("[job] huawei-power-scan: no results")
		return
	}

	var totalOntReadings int
	for _, b := range batches {
		totalOntReadings += len(b.Records)
	}
	log.Printf("[job] huawei-power-scan: total ONT power readings (all OLTs): %d", totalOntReadings)

	log.Printf("[job] huawei-power-scan: collected %d OLTs, writing to DB", len(batches))
	if err := s.powerRepo.ReplaceAll(batches); err != nil {
		log.Printf("[job] huawei-power-scan: replace all failed: %v", err)
	}

	s.notify("power_update")
	log.Println("[job] huawei-power-scan: done")
}

func (s *Scheduler) runHuaweiPortScan() {
	log.Println("[job] huawei-port-scan: starting")
	results := huawei.CollectProtectGroups(s.cfg.HuaweiOLTUser, s.cfg.HuaweiOLTPass)
	if len(results) == 0 {
		log.Println("[job] huawei-port-scan: no results")
		return
	}

	now := time.Now()
	var portBatches []repository.PortBatch
	var allHistorySnaps []models.PortSnapshot

	for _, r := range results {
		if r.Err != "" && len(r.Groups) == 0 {
			log.Printf("[job] huawei-port-scan: skip %s: %s", r.Host, r.Err)
			continue
		}

		var records []models.PortProtectionRecord
		for _, g := range r.Groups {
			switched := false
			if len(g.Members) == 2 {
				switched = g.Members[0].Role != "work" || g.Members[1].Role != "protect"
			}
			for _, m := range g.Members {
				rec := models.PortProtectionRecord{
					Port:        fmt.Sprintf("group%d/%s", g.GroupID, m.Member),
					PortState:   m.Role,
					PairedState: m.State,
					SwoReason:   m.Operation,
					NumSwo:      g.GroupID,
					Vendor:      "huawei",
				}
				if switched {
					rec.SwoReason = "switchover|" + m.Operation
				}
				records = append(records, rec)

				allHistorySnaps = append(allHistorySnaps, models.PortSnapshot{
					Device:      r.Device,
					Site:        r.Site,
					Host:        r.Host,
					Vendor:      "huawei",
					Port:        rec.Port,
					PortState:   m.Role,
					PairedState: m.State,
					SwoReason:   rec.SwoReason,
					MeasuredAt:  now,
				})
			}
		}

		portBatches = append(portBatches, repository.PortBatch{
			Device:  r.Device,
			Site:    r.Site,
			Host:    r.Host,
			Records: records,
		})
	}

	if len(portBatches) == 0 {
		log.Println("[job] huawei-port-scan: no valid batches")
		return
	}

	log.Printf("[job] huawei-port-scan: collected %d OLTs, writing to DB", len(portBatches))
	if err := s.portRepo.ReplaceAll(portBatches); err != nil {
		log.Printf("[job] huawei-port-scan: replace all failed: %v", err)
	}

	if len(allHistorySnaps) > 0 {
		if err := s.portHistRepo.BulkInsert(allHistorySnaps); err != nil {
			log.Printf("[job] huawei-port-scan: history bulk insert failed: %v", err)
		}
	}

	s.notify("port_update")
	log.Println("[job] huawei-port-scan: done")
}

func (s *Scheduler) runHuaweiBackup() {
	log.Println("[job] huawei-backup: starting")
	results := huawei.Backups(s.cfg.HuaweiOLTUser, s.cfg.HuaweiOLTPass)

	for _, r := range results {
		if r.FilePath == "" {
			if r.Err != "" {
				log.Printf("[job] huawei-backup: %s: %s", r.Host, r.Err)
			}
			continue
		}
		if err := s.backupRepo.Create(&models.OltBackups{
			Device:   r.Device,
			Site:     r.Site,
			Host:     r.Host,
			Vendor:   "huawei",
			FilePath: r.FilePath,
		}); err != nil {
			log.Printf("[job] huawei-backup: db %s: %v", r.Host, err)
		}
	}

	s.notify("backup_update")
	log.Println("[job] huawei-backup: done")
}

func (s *Scheduler) runHuaweiInventoryScan() {
	log.Println("[job] huawei-inventory-scan: starting")
	results := huawei.CollectInventory(s.cfg.HuaweiOLTUser, s.cfg.HuaweiOLTPass)
	if len(results) == 0 {
		log.Println("[job] huawei-inventory-scan: no results")
		return
	}

	now := time.Now()
	modelTotals := make(map[string]int)
	var modelOrder []string
	var oltInventories []models.OltInventory

	for _, r := range results {
		if r.Err != "" && len(r.ONTs) == 0 {
			log.Printf("[job] huawei-inventory-scan: skip %s: %s", r.Host, r.Err)
			continue
		}

		oltModelCounts := make(map[string]int)
		var oltModelOrder []string
		oltVendorCounts := make(map[string]int)
		var oltVendorOrder []string
		for _, ont := range r.ONTs {
			model := ont.OntModel
			if model == "" {
				model = "Unknown"
			}
			if oltModelCounts[model] == 0 {
				oltModelOrder = append(oltModelOrder, model)
			}
			oltModelCounts[model]++

			if modelTotals[model] == 0 {
				modelOrder = append(modelOrder, model)
			}
			modelTotals[model]++

			vid := ont.VendorID
			if vid == "" {
				vid = "Unknown"
			}
			if oltVendorCounts[vid] == 0 {
				oltVendorOrder = append(oltVendorOrder, vid)
			}
			oltVendorCounts[vid]++
		}

		counts := make([]extractor.EquipIDCount, 0, len(oltModelOrder))
		for _, m := range oltModelOrder {
			counts = append(counts, extractor.EquipIDCount{ID: m, Count: oltModelCounts[m], Vendor: "huawei"})
		}

		vendorCounts := make([]extractor.VendorCount, 0, len(oltVendorOrder))
		for _, v := range oltVendorOrder {
			vendorCounts = append(vendorCounts, extractor.VendorCount{Vendor: v, Count: oltVendorCounts[v]})
		}

		swVerCounts := make(map[string]int)
		var swVerOrder []string
		for _, ont := range r.ONTs {
			sw := ont.SwVersion
			if sw == "" {
				sw = "Unknown"
			}
			if swVerCounts[sw] == 0 {
				swVerOrder = append(swVerOrder, sw)
			}
			swVerCounts[sw]++
		}
		swVers := make([]extractor.SwVerCount, 0, len(swVerOrder))
		for _, sw := range swVerOrder {
			swVers = append(swVers, extractor.SwVerCount{SwVerAct: sw, Count: swVerCounts[sw], Vendor: "Huawei"})
		}

		oltInventories = append(oltInventories, models.OltInventory{
			Host:         r.Host,
			Device:       r.Device,
			Site:         r.Site,
			Vendor:       "huawei",
			Counts:       counts,
			VendorCounts: vendorCounts,
			SwVerCounts:  swVers,
			Total:        r.Total,
			MeasuredAt:   now,
		})

		var ontItems []models.OntInventoryItem
		for _, ont := range r.ONTs {
			if ont.Index != "" {
				ontItems = append(ontItems, models.OntInventoryItem{
					OntIdx:  ont.Index,
					EquipID: ont.OntModel,
					Vendor:  "huawei",
				})
			}
		}
		if len(ontItems) > 0 {
			if err := s.inventoryRepo.ReplaceOntInventoryByHost(r.Host, "huawei", ontItems); err != nil {
				log.Printf("[job] huawei-inventory-scan: per-ONT inventory %s: %v", r.Host, err)
			}
		}
	}

	if err := s.inventoryRepo.SaveOltInventory(oltInventories); err != nil {
		log.Printf("[job] huawei-inventory-scan: save OLT inventories: %v", err)
	}

	globalCounts := make([]extractor.EquipIDCount, 0, len(modelOrder))
	globalTotal := 0
	for _, m := range modelOrder {
		c := modelTotals[m]
		globalCounts = append(globalCounts, extractor.EquipIDCount{ID: m, Count: c, Vendor: "Huawei"})
		globalTotal += c
	}

	swVerTotals := make(map[string]int)
	var swVerOrder []string
	for _, olt := range oltInventories {
		for _, sv := range olt.SwVerCounts {
			if swVerTotals[sv.SwVerAct] == 0 {
				swVerOrder = append(swVerOrder, sv.SwVerAct)
			}
			swVerTotals[sv.SwVerAct] += sv.Count
		}
	}
	globalSwVerCounts := make([]extractor.SwVerCount, 0, len(swVerOrder))
	for _, ver := range swVerOrder {
		globalSwVerCounts = append(globalSwVerCounts, extractor.SwVerCount{SwVerAct: ver, Count: swVerTotals[ver], Vendor: "Huawei"})
	}

	globalVendorTotals := make(map[string]int)
	var globalVendorOrder []string
	for _, olt := range oltInventories {
		for _, vc := range olt.VendorCounts {
			if globalVendorTotals[vc.Vendor] == 0 {
				globalVendorOrder = append(globalVendorOrder, vc.Vendor)
			}
			globalVendorTotals[vc.Vendor] += vc.Count
		}
	}
	globalVendorCounts := make([]extractor.VendorCount, 0, len(globalVendorOrder))
	for _, v := range globalVendorOrder {
		globalVendorCounts = append(globalVendorCounts, extractor.VendorCount{Vendor: v, Count: globalVendorTotals[v]})
	}

	summary := &models.InventorySummary{
		Vendor:       "huawei",
		Count:        globalCounts,
		VendorCounts: globalVendorCounts,
		SwVerCounts:  globalSwVerCounts,
		Total:        globalTotal,
		MeasuredAt:   now,
	}

	if err := s.inventoryRepo.SaveSummary(summary); err != nil {
		log.Printf("[job] huawei-inventory-scan: save summary: %v", err)
	}

	s.notify("inventory_update")
	log.Println("[job] huawei-inventory-scan: done")
}

// --- notify ---

func (s *Scheduler) notify(eventType string) {
	msg, _ := json.Marshal(map[string]string{
		"type": eventType,
	})
	s.hub.Broadcast(msg)
}

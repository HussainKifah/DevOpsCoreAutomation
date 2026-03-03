package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
	"github.com/go-co-op/gocron/v2"
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

	healthBuf   map[string]*models.HealthSnapshot
	healthBufMu sync.Mutex
	scanMu      sync.Mutex
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
		healthBuf:      make(map[string]*models.HealthSnapshot),
	}
}

func (s *Scheduler) Start() {
	sched, err := gocron.NewScheduler()
	if err != nil {
		log.Fatalf("scheduler: %v", err)
	}

	mustAdd(sched, s.cfg.PowerScanInterval, s.runPowerScan, "power-scan")
	mustAdd(sched, s.cfg.DescScanInterval, s.runDescScan, "desc-scan")
	mustAdd(sched, s.cfg.HealthScanInterval, s.runHealthScan, "health-scan")
	mustAdd(sched, s.cfg.PortScanInterval, s.runPortScan, "port-scan")
	mustAddCron(sched, "0 0 * * *", s.runBackup, "backup")
	mustAddCron(sched, "0 1 * * *", s.runHistoryCleanup, "history-cleanup")
	mustAddCron(sched, "0 2 1 * *", s.runInventoryScan, "inventory-scan") // Runs at 02:00 on the 1st of every month

	sched.Start()
	log.Println("scheduler started")

	// Run ll jobs immediately in background without blocking
	go func() {
		log.Println("[startup] running all jobs immediately")
		s.runInventoryScan()
		s.runHealthScan()
		s.runPowerScan()
		s.runHealthScan()
		s.runDescScan()
		s.runHealthScan()
		s.runPortScan()
		s.runHealthScan()
		s.runBackup()
		s.runHealthScan()
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

func mustAddCron(sched gocron.Scheduler, cron string, fn func(), name string) {
	_, err := sched.NewJob(
		gocron.CronJob(cron, false),
		gocron.NewTask(fn),
		gocron.WithName(name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		log.Fatalf("scheduler: add cron job %s: %v", name, err)
	}
	log.Printf("scheduled cron job %q: %s", name, cron)
}

// --- Power scan job ---

func (s *Scheduler) runPowerScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] power-scan: starting")
	cmd := "show equipment ont optics"

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
			}
		}

		if err := s.powerRepo.DeleteByHost(r.Host); err != nil {
			log.Printf("[job] power-scan: delete %s: %v", r.Host, err)
		}
		if err := s.powerRepo.BulkInsert(r.Device, r.Site, r.Host, records); err != nil {
			log.Printf("[job] power-scan: insert %s: %v", r.Host, err)
		}
	}
	s.notify("power_update")
	log.Println("[job] power-scan: done")
}

// --- Description scan job ---

func (s *Scheduler) runDescScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] desc-scan: starting")
	cmd := "show equipment ont status pon"

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
			}
		}

		if err := s.descRepo.DeleteByHost(r.Host); err != nil {
			log.Printf("[job] desc-scan: delete %s: %v", r.Host, err)
		}
		if err := s.descRepo.BulkInsert(r.Device, r.Site, r.Host, records); err != nil {
			log.Printf("[job] desc-scan: insert %s: %v", r.Host, err)
		}
	}
	s.notify("desc_update")
	log.Println("[job] desc-scan: done")
}

// --- Health scan job ---

func (s *Scheduler) runHealthScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] health-scan: starting")
	cmds := []string{
		"show system cpu-load detail",
		"show core1-uptime",
		"show equipment temperature",
	}

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

		record := &models.OltHealth{
			Device:       r.Device,
			Site:         r.Site,
			Host:         r.Host,
			Uptime:       h.Uptime,
			CpuLoads:     cpuSlice,
			Temperatures: tempSlice,
			MeasuredAt:   time.Now(),
		}

		if err := s.healthRepo.Upsert(record); err != nil {
			log.Printf("[job] health-scan: upsert %s: %v", r.Host, err)
		}

		snap := &models.HealthSnapshot{
			Device:       r.Device,
			Site:         r.Site,
			Host:         r.Host,
			Uptime:       h.Uptime,
			CpuLoads:     cpuSlice,
			Temperatures: tempSlice,
			MeasuredAt:   time.Now(),
		}

		s.healthBufMu.Lock()
		prev, hasPrev := s.healthBuf[r.Host]
		if !hasPrev {
			s.healthBuf[r.Host] = snap
		} else {
			averaged := averageSnapshots(prev, snap)
			if err := s.healthHistRepo.Insert(averaged); err != nil {
				log.Printf("[job] health-scan: history insert %s: %v", r.Host, err)
			}
			delete(s.healthBuf, r.Host)
		}
		s.healthBufMu.Unlock()
	}
	s.notify("health_update")
	log.Println("[job] health-scan: done")
}

// FlushHealthBuffer saves any buffered snapshots as-is (used on shutdown).
func (s *Scheduler) RunHealthScan()    { go s.runHealthScan() }
func (s *Scheduler) RunPowerScan()     { go s.runPowerScan() }
func (s *Scheduler) RunPortScan()      { go s.runPortScan() }
func (s *Scheduler) RunInventoryScan() { go s.runInventoryScan() }

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

// --- History cleanup job ---

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

// --- Port protection scan job ---

func (s *Scheduler) runPortScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] port-scan: starting")
	cmd := "show port-protection"
	now := time.Now()

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] port-scan: ERROR %s: %v", r.Host, r.Err)
			continue
		}
		ports := extractor.ExtractPortProtection(r.Data)

		var filtered []models.PortProtectionRecord
		var historySnaps []models.PortSnapshot
		for _, p := range ports {
			if strings.Contains(p.PortState, "down") || strings.Contains(p.PairedState, "down") {
				filtered = append(filtered, models.PortProtectionRecord{
					Port:        p.Port,
					PortState:   p.PortState,
					PairedState: p.PairedState,
					SwoReason:   p.SwoReason,
					NumSwo:      p.NumSwo,
				})
				historySnaps = append(historySnaps, models.PortSnapshot{
					Device:      r.Device,
					Site:        r.Site,
					Host:        r.Host,
					Port:        p.Port,
					PortState:   p.PortState,
					PairedState: p.PairedState,
					SwoReason:   p.SwoReason,
					NumSwo:      p.NumSwo,
					MeasuredAt:  now,
				})
			}
		}

		if err := s.portRepo.DeleteByHost(r.Host); err != nil {
			log.Printf("[job] port-scan: delete %s: %v", r.Host, err)
		}
		if len(filtered) > 0 {
			if err := s.portRepo.BulkInsert(r.Device, r.Site, r.Host, filtered); err != nil {
				log.Printf("[job] port-scan: insert %s: %v", r.Host, err)
			}
		}
		if len(historySnaps) > 0 {
			if err := s.portHistRepo.BulkInsert(historySnaps); err != nil {
				log.Printf("[job] port-scan: history %s: %v", r.Host, err)
			}
		}
	}
	s.notify("port_update")
	log.Println("[job] port-scan: done")
}

// --- Backup job ---

func (s *Scheduler) runBackup() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] backup: starting")
	cmd := "info configure flat"

	for r := range shell.SendCommandNokiaOLTsPooled(s.pool, cmd) {
		if r.Err != nil {
			log.Printf("[job] backup: ERROR %s: %v", r.Host, r.Err)
			continue
		}

		site := strings.ReplaceAll(r.Site, "/", "-")
		if site == "" {
			site = "unknown"
		}

		folder := filepath.Join("backups", site, time.Now().Format("2006-01-02"))
		if err := os.MkdirAll(folder, 0o755); err != nil {
			log.Printf("[job] backup: mkdir %s: %v", folder, err)
			continue
		}

		cleaned := extractor.CleanBackupOutput(r.Data)

		name := strings.ReplaceAll(r.Device, "/", "-")
		filename := fmt.Sprintf("%s_%s.txt", name, r.Host)
		path := filepath.Join(folder, filename)

		if err := os.WriteFile(path, []byte(cleaned), 0o644); err != nil {
			log.Printf("[job] backup: write %s: %v", path, err)
			continue
		}
		log.Printf("[job] backup: saved %s", path)

		if err := s.backupRepo.Create(&models.OltBackups{
			Device:   r.Device,
			Site:     site,
			Host:     r.Host,
			FilePath: path,
		}); err != nil {
			log.Printf("[job] backup: db %s: %v", r.Host, err)
		}
	}
	s.notify("backup_update")
	log.Println("[job] backup: done")
}

// --- Inventory scan job ---

func (s *Scheduler) runInventoryScan() {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	log.Println("[job] inventory-scan: starting")

	cmd := "show equipment ont interface detail | match exact:equip-id | count"

	totals := make(map[string]int)
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

			// For global totals
			if totals[c.ID] == 0 {
				order = append(order, c.ID)
			}
			totals[c.ID] += c.Count

			// For this specific OLT
			vendor := extractor.GetVender(c.ID)
			if vendorTotals[vendor] == 0 {
				vendorOrder = append(vendorOrder, vendor)
			}
			vendorTotals[vendor] += c.Count
		}

		oltVendorCounts := make([]extractor.VendorCount, 0, len(vendorOrder))
		for _, v := range vendorOrder {
			oltVendorCounts = append(oltVendorCounts, extractor.VendorCount{Vendor: v, Count: vendorTotals[v]})
		}

		oltInventories = append(oltInventories, models.OltInventory{
			Host:         r.Host,
			Device:       r.Device,
			Site:         r.Site,
			Counts:       counts,
			VendorCounts: oltVendorCounts,
			Total:        oltTotal,
			MeasuredAt:   now,
		})
	}

	// Save individual OLT inventories
	if err := s.inventoryRepo.SaveOltInventory(oltInventories); err != nil {
		log.Printf("[job] inventory-scan: failed to save OLT inventories: %v", err)
	}

	// Calculate and save global summary
	globalCounts := make([]extractor.EquipIDCount, 0, len(order))
	globalTotal := 0
	globalVendorTotals := make(map[string]int)
	var globalVendorOrder []string

	for _, id := range order {
		c := totals[id]
		globalCounts = append(globalCounts, extractor.EquipIDCount{ID: id, Count: c})
		globalTotal += c

		vendor := extractor.GetVender(id)
		if globalVendorTotals[vendor] == 0 {
			globalVendorOrder = append(globalVendorOrder, vendor)
		}
		globalVendorTotals[vendor] += c
	}

	globalVendorCounts := make([]extractor.VendorCount, 0, len(globalVendorOrder))
	for _, v := range globalVendorOrder {
		globalVendorCounts = append(globalVendorCounts, extractor.VendorCount{Vendor: v, Count: globalVendorTotals[v]})
	}

	summary := &models.InventorySummary{
		Count:        globalCounts,
		VendorCounts: globalVendorCounts,
		Total:        globalTotal,
		MeasuredAt:   now,
	}

	if err := s.inventoryRepo.SaveSummary(summary); err != nil {
		log.Printf("[job] inventory-scan: failed to save summary: %v", err)
	}

	s.notify("inventory_update")
	log.Println("[job] inventory-scan: done")
}

// --- notify ---

func (s *Scheduler) notify(eventType string) {
	msg, _ := json.Marshal(map[string]string{
		"type": eventType,
	})
	s.hub.Broadcast(msg)
}

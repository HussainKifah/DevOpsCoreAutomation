package scheduler

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/go-co-op/gocron/v2"
)

type Scheduler struct {
	cfg        *config.Config
	powerRepo  repository.PowerRepository
	descRepo   repository.DescriptionRepository
	healthRepo repository.HealthRepository
	portRepo   repository.PortProtectionRepository
	backupRepo repository.BackupRepository
}

func New(
	cfg *config.Config,
	pr repository.PowerRepository,
	dr repository.DescriptionRepository,
	hr repository.HealthRepository,
	pp repository.PortProtectionRepository,
	br repository.BackupRepository,
) *Scheduler {
	return &Scheduler{
		cfg:        cfg,
		powerRepo:  pr,
		descRepo:   dr,
		healthRepo: hr,
		portRepo:   pp,
		backupRepo: br,
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
	mustAdd(sched, s.cfg.BackupInterval, s.runBackup, "backup")

	sched.Start()
	log.Println("scheduler started")

	// Run ll jobs immediately in background without blocking
	// go func() {
	// log.Println("[startup] running all jobs immediately")
	// 	// 	s.runHealthScan()
	// 	// 	s.runPowerScan()
	// s.runDescScan()
	// 	// 	s.runPortScan()
	// 	// 	s.runBackup()
	// 	log.Println("[startup] initial scan complete")
	// }()
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
	log.Printf("scheduled job %q every %s", name, interval)
}

// --- Power scan job ---

func (s *Scheduler) runPowerScan() {
	log.Println("[job] power-scan: starting")
	cmd := "show equipment ont optics"

	for r := range shell.SendCommandNokiaOLTs(s.cfg.OLTUser, s.cfg.OLTPass, cmd) {
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
	log.Println("[job] power-scan: done")
}

// --- Description scan job ---

func (s *Scheduler) runDescScan() {
	log.Println("[job] desc-scan: starting")
	cmd := "show equipment ont status pon"

	for r := range shell.SendCommandNokiaOLTs(s.cfg.OLTUser, s.cfg.OLTPass, cmd) {
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
	log.Println("[job] desc-scan: done")
}

// --- Health scan job ---

func (s *Scheduler) runHealthScan() {
	log.Println("[job] health-scan: starting")
	cmds := []string{
		"show system cpu-load detail",
		"show core1-uptime",
		"show equipment temperature",
	}

	for r := range shell.SendCommandNokiaOLTs(s.cfg.OLTUser, s.cfg.OLTPass, cmds...) {
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
	}
	log.Println("[job] health-scan: done")
}

// --- Port protection scan job ---

func (s *Scheduler) runPortScan() {
	log.Println("[job] port-scan: starting")
	cmd := "show port-protection"

	for r := range shell.SendCommandNokiaOLTs(s.cfg.OLTUser, s.cfg.OLTPass, cmd) {
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
					PortState:   p.PortState,
					PairedState: p.PairedState,
					SwoReason:   p.SwoReason,
					NumSwo:      p.NumSwo,
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
	}
	log.Println("[job] port-scan: done")
}

// --- Backup job ---

func (s *Scheduler) runBackup() {
	log.Println("[job] backup: starting")
	cmd := "info configure flat"

	for r := range shell.SendCommandNokiaOLTs(s.cfg.OLTUser, s.cfg.OLTPass, cmd) {
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
	log.Println("[job] backup: done")
}

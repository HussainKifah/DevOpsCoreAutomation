package scheduler

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/go-co-op/gocron/v2"
)

// WorkflowScheduler manages team workflow jobs and writes activity logs.
type WorkflowScheduler struct {
	repo        repository.WorkflowRepository
	nocDataRepo repository.NocDataRepository
	jwtSecret   []byte
	sched       gocron.Scheduler
	scope       string
	// jobMap stores the gocron tag string for each registered WorkflowJob ID.
	// We use the tag (not the gocron.Job value) because gocron.Job.ID() returns
	// a uuid.UUID, not a uint, making it unsuitable as a map key for our purposes.
	jobMap map[uint]string
}

func NewWorkflowScheduler(repo repository.WorkflowRepository, jwtSecret []byte) (*WorkflowScheduler, error) {
	return NewWorkflowSchedulerForScope(repo, jwtSecret, "ip")
}

func NewWorkflowSchedulerForScope(repo repository.WorkflowRepository, jwtSecret []byte, scope string, nocDataRepo ...repository.NocDataRepository) (*WorkflowScheduler, error) {
	if scope == "" {
		scope = "ip"
	}
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	var ndRepo repository.NocDataRepository
	if len(nocDataRepo) > 0 {
		ndRepo = nocDataRepo[0]
	}
	return &WorkflowScheduler{
		repo:        repo,
		nocDataRepo: ndRepo,
		jwtSecret:   jwtSecret,
		sched:       s,
		scope:       scope,
		jobMap:      make(map[uint]string),
	}, nil
}

func (ws *WorkflowScheduler) Start() {
	ws.sched.Start()
	ws.writeSystemLog("system", "info", "scheduler_started", "Workflow scheduler started")
	ws.ReloadAll()
	log.Printf("[workflow-scheduler:%s] started", ws.scope)
}

// ReloadAll removes all previously registered gocron jobs and re-registers
// every enabled WorkflowJob from the database.
func (ws *WorkflowScheduler) ReloadAll() {
	// Remove all jobs we previously registered using their stored tags.
	for _, tag := range ws.jobMap {
		ws.sched.RemoveByTags(tag)
	}
	ws.jobMap = make(map[uint]string)

	jobs, err := ws.repo.ListEnabledJobs()
	if err != nil {
		log.Printf("[workflow-scheduler:%s] reload error: %v", ws.scope, err)
		ws.writeSystemLog("system", "error", "scheduler_reload",
			fmt.Sprintf("Failed to reload jobs: %v", err))
		return
	}
	for i := range jobs {
		ws.register(jobs[i])
	}
	ws.writeSystemLog("system", "info", "scheduler_reload",
		fmt.Sprintf("Scheduler reloaded — %d active jobs registered", len(jobs)))
	log.Printf("[workflow-scheduler:%s] registered %d jobs", ws.scope, len(jobs))
}

func (ws *WorkflowScheduler) register(j models.WorkflowJob) {
	if strings.TrimSpace(j.Schedule) == "once" {
		log.Printf("[workflow-scheduler:%s] job %d is run-once — skipping scheduler registration", ws.scope, j.ID)
		return
	}
	def, valid := ws.jobDefinition(j.Schedule)
	if !valid {
		log.Printf("[workflow-scheduler:%s] invalid schedule for job %d: %q", ws.scope, j.ID, j.Schedule)
		ws.writeJobLog(&j, nil, "warning", "job_skipped",
			fmt.Sprintf("Invalid schedule %q — job will not run until fixed", j.Schedule), 0)
		return
	}

	tag := fmt.Sprintf("wf-%s-%d", ws.scope, j.ID)
	jobID := j.ID // capture for closure

	_, err := ws.sched.NewJob(
		def,
		gocron.NewTask(func() { ws.runJob(jobID) }),
		gocron.WithName(tag),
		gocron.WithTags(tag),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		log.Printf("[workflow-scheduler:%s] schedule job %d: %v", ws.scope, j.ID, err)
		ws.writeJobLog(&j, nil, "error", "job_skipped",
			fmt.Sprintf("Failed to register with gocron: %v", err), 0)
		return
	}

	ws.jobMap[j.ID] = tag
}

// jobDefinition converts a schedule string into a gocron.JobDefinition.
// Returns (definition, true) on success or (nil, false) if unparseable.
// Accepts Go duration strings ("6h", "30m") or 5-field cron expressions ("0 21 * * *").
func (ws *WorkflowScheduler) jobDefinition(schedule string) (gocron.JobDefinition, bool) {
	s := strings.TrimSpace(schedule)
	if s == "" || s == "once" {
		return nil, false
	}
	// Try Go duration first (e.g. "30m", "6h", "24h")
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return gocron.DurationJob(d), true
	}
	// Fall through to cron — gocron accepts 5-field standard cron.
	// gocron.CronJob never returns an error itself; validation happens at NewJob time.
	return gocron.CronJob(s, false), true
}

// RunJobNow triggers a job immediately (called from the UI "Run Now" button).
func (ws *WorkflowScheduler) RunJobNow(jobID uint) {
	go ws.runJob(jobID)
}

func (ws *WorkflowScheduler) runJob(jobID uint) {
	log.Printf("[workflow:%s] RUN JOB %d", ws.scope, jobID)

	j, err := ws.repo.GetJob(jobID)
	if err != nil {
		log.Printf("[workflow:%s] job %d: DB lookup FAILED: %v", ws.scope, jobID, err)
		ws.writeSystemLog("system", "error", "job_failed",
			fmt.Sprintf("Job %d not found in database: %v", jobID, err))
		return
	}
	targetDevices, targetLabel, err := ws.resolveTargetDevices(j)
	if err != nil {
		log.Printf("[workflow:%s] job %d: target resolution failed: %v", ws.scope, j.ID, err)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", err.Error(), time.Now())
		ws.writeSystemLog("system", "error", "job_failed", fmt.Sprintf("Job %d target resolution failed: %v", j.ID, err))
		return
	}
	if len(targetDevices) == 0 {
		msg := fmt.Sprintf("no matching devices found for target %q", targetLabel)
		log.Printf("[workflow:%s] job %d: %s", ws.scope, j.ID, msg)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
		return
	}

	successCount := 0
	var failures []string
	lastRunAt := time.Now()

	for i := range targetDevices {
		device := targetDevices[i]
		runErr := ws.runJobForDevice(j, &device)
		lastRunAt = time.Now()
		if runErr != nil {
			failures = append(failures, fmt.Sprintf("%s (%s): %v", device.Name, device.Host, runErr))
			continue
		}
		successCount++
	}

	switch {
	case len(failures) == 0:
		msg := fmt.Sprintf("completed for %d device(s) on target %s", successCount, targetLabel)
		_ = ws.repo.UpdateJobStatus(j.ID, "ok", msg, lastRunAt)
	case successCount == 0:
		msg := strings.Join(failures, "; ")
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, lastRunAt)
	default:
		msg := fmt.Sprintf("completed for %d device(s), %d failed on target %s", successCount, len(failures), targetLabel)
		if len(failures) > 0 {
			msg += ": " + strings.Join(failures, "; ")
		}
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, lastRunAt)
	}
	log.Printf("[workflow:%s] END JOB %d", ws.scope, jobID)
}

func (ws *WorkflowScheduler) runJobForDevice(job *models.WorkflowJob, device *models.WorkflowDevice) error {
	if strings.TrimSpace(device.Host) == "" {
		return fmt.Errorf("device has no host")
	}

	jobView := *job
	jobView.Device = *device

	log.Printf("[workflow:%s] job %d: device=%s host=%s vendor=%s type=%s cmd=%q schedule=%s",
		ws.scope, job.ID, device.Name, device.Host, device.Vendor, job.JobType, job.Command, job.Schedule)
	log.Printf("[workflow:%s] job %d: decrypting credentials...", ws.scope, job.ID)

	user, err := crypto.Decrypt(ws.jwtSecret, device.EncUsername)
	if err != nil {
		msg := "credential decrypt failed: " + err.Error()
		ws.writeJobLog(&jobView, nil, "error", "job_failed", msg, 0)
		log.Printf("[workflow:%s] job %d: %s", ws.scope, job.ID, msg)
		return err
	}
	pass, err := crypto.Decrypt(ws.jwtSecret, device.EncPassword)
	if err != nil {
		msg := "credential decrypt failed: " + err.Error()
		ws.writeJobLog(&jobView, nil, "error", "job_failed", msg, 0)
		log.Printf("[workflow:%s] job %d: %s", ws.scope, job.ID, msg)
		return err
	}
	log.Printf("[workflow:%s] job %d: credentials decrypted OK (user=%q)", ws.scope, job.ID, user)

	startedAt := time.Now()
	run := &models.WorkflowRun{
		JobID:      job.ID,
		DeviceName: device.Name,
		Host:       device.Host,
		Vendor:     device.Vendor,
		StartedAt:  startedAt,
		Status:     "pending",
	}
	if createErr := ws.repo.CreateRun(run); createErr != nil {
		msg := "failed to create run record: " + createErr.Error()
		log.Printf("[workflow:%s] job %d: %s", ws.scope, job.ID, msg)
		return createErr
	}

	ws.writeJobLog(&jobView, &run.ID, "info", "job_started",
		fmt.Sprintf("Started %s job on %s (%s)", job.JobType, device.Name, device.Host), 0)

	var cmd string
	if job.JobType == "backup" {
		cmd, err = shell.IPBackupCommand(device.Vendor)
		if err != nil {
			msg := "cannot resolve backup command for vendor: " + err.Error()
			ws.writeJobLog(&jobView, &run.ID, "error", "job_failed", msg, 0)
			_ = ws.repo.FinishRun(run.ID, "error", "", msg, time.Now())
			return err
		}
	} else {
		cmd = job.Command
	}

	log.Printf("[workflow:%s] job %d: connecting to %s (vendor=%s, transport=%s)...",
		ws.scope, job.ID, device.Host, device.Vendor, "auto")
	log.Printf("[workflow:%s] job %d: sending command: %q", ws.scope, job.ID, cmd)

	output, method, execErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), device.Host, user, pass, device.Vendor, "", cmd)

	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()

	if execErr != nil {
		errMsg := execErr.Error()
		_ = ws.repo.FinishRun(run.ID, "error", output, errMsg, finishedAt)
		ws.writeJobLog(&jobView, &run.ID, "error", "job_failed",
			fmt.Sprintf("FAILED after %dms: %s", durationMs, errMsg), durationMs)
		log.Printf("[workflow:%s] job %d: FAILED in %dms: %s", ws.scope, job.ID, durationMs, errMsg)
		if len(output) > 0 {
			preview := output
			if len(preview) > 500 {
				preview = preview[:500] + "...(truncated)"
			}
			log.Printf("[workflow:%s] job %d: partial output:\n%s", ws.scope, job.ID, preview)
		}
		return execErr
	}

	successMsg := fmt.Sprintf("Completed in %dms — %d bytes collected", durationMs, len(output))
	if job.JobType == "backup" {
		savedPath := ws.saveBackupFile(device, output, startedAt)
		successMsg = fmt.Sprintf("Backup completed in %dms — %d bytes — saved to %s",
			durationMs, len(output), savedPath)
		if saveCmd, ok := ciscoSaveConfigCommand(device.Vendor); ok {
			log.Printf("[workflow:%s] job %d: saving running config on %s with %q", ws.scope, job.ID, device.Host, saveCmd)
			_, _, saveErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), device.Host, user, pass, device.Vendor, method, saveCmd)
			finishedAt = time.Now()
			durationMs = finishedAt.Sub(startedAt).Milliseconds()
			if saveErr != nil {
				errMsg := "backup collected, but save config failed: " + saveErr.Error()
				_ = ws.repo.FinishRun(run.ID, "error", output, errMsg, finishedAt)
				ws.writeJobLog(&jobView, &run.ID, "error", "job_failed",
					fmt.Sprintf("Backup saved to %s, but Cisco save config failed after %dms: %s", savedPath, durationMs, saveErr.Error()), durationMs)
				return saveErr
			}
			successMsg = fmt.Sprintf("Backup completed in %dms — %d bytes — saved to %s — startup config saved",
				durationMs, len(output), savedPath)
		}
	}

	_ = ws.repo.FinishRun(run.ID, "ok", output, "", finishedAt)
	ws.writeJobLog(&jobView, &run.ID, "success", "job_success", successMsg, durationMs)
	log.Printf("[workflow:%s] job %d: OK in %dms (%d bytes) via %s", ws.scope, job.ID, durationMs, len(output), method)
	if len(output) > 0 {
		preview := output
		if len(preview) > 300 {
			preview = preview[:300] + "...(truncated)"
		}
		log.Printf("[workflow:%s] job %d: output preview:\n%s", ws.scope, job.ID, preview)
	}
	return nil
}

func ciscoSaveConfigCommand(vendor string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco", "cisco_ios", "cisco_nexus", "nexus":
		return "write memory", true
	default:
		return "", false
	}
}

func (ws *WorkflowScheduler) resolveTargetDevices(job *models.WorkflowJob) ([]models.WorkflowDevice, string, error) {
	targetType := strings.ToLower(strings.TrimSpace(job.TargetType))
	if targetType == "" {
		targetType = "device"
	}
	targetLabel := strings.TrimSpace(job.TargetLabel)
	if targetLabel == "" {
		targetLabel = strings.TrimSpace(job.TargetValue)
	}

	if targetType == "device" {
		if strings.TrimSpace(job.Device.Host) != "" {
			if targetLabel == "" {
				targetLabel = job.Device.Name
			}
			return []models.WorkflowDevice{job.Device}, targetLabel, nil
		}
		if job.DeviceID == nil {
			return nil, targetLabel, fmt.Errorf("device target has no device_id")
		}
		device, err := ws.repo.GetDevice(*job.DeviceID)
		if err != nil {
			return nil, targetLabel, fmt.Errorf("device not found")
		}
		if targetLabel == "" {
			targetLabel = device.Name
		}
		return []models.WorkflowDevice{*device}, targetLabel, nil
	}

	if ws.nocDataRepo == nil {
		allDevices, err := ws.repo.ListDevices()
		if err != nil {
			return nil, targetLabel, err
		}
		devices := make([]models.WorkflowDevice, 0)
		for i := range allDevices {
			if ws.workflowDeviceTargetMatches(job, &allDevices[i]) {
				devices = append(devices, allDevices[i])
			}
		}
		return devices, targetLabel, nil
	}

	rows, err := ws.nocDataRepo.ListAll()
	if err != nil {
		return nil, targetLabel, err
	}

	seenHosts := make(map[string]struct{})
	devices := make([]models.WorkflowDevice, 0)
	for i := range rows {
		row := rows[i]
		if !ws.nocTargetMatches(job, &row) {
			continue
		}
		host := strings.ToLower(strings.TrimSpace(row.Host))
		if host == "" {
			continue
		}
		if _, ok := seenHosts[host]; ok {
			continue
		}
		device, err := ws.repo.GetDeviceByHost(row.Host)
		if err != nil {
			continue
		}
		seenHosts[host] = struct{}{}
		devices = append(devices, *device)
	}

	return devices, targetLabel, nil
}

func (ws *WorkflowScheduler) workflowDeviceTargetMatches(job *models.WorkflowJob, device *models.WorkflowDevice) bool {
	targetType := strings.ToLower(strings.TrimSpace(job.TargetType))
	targetValue := strings.TrimSpace(job.TargetValue)
	switch targetType {
	case "all_networks":
		return true
	case "network_type":
		return strings.EqualFold(strings.TrimSpace(device.NetworkType), targetValue)
	case "device_type":
		return strings.EqualFold(strings.TrimSpace(device.DeviceType), targetValue)
	case "province":
		return strings.EqualFold(strings.TrimSpace(device.Province), targetValue)
	case "vendor":
		return strings.EqualFold(strings.TrimSpace(device.Vendor), targetValue)
	default:
		return false
	}
}

func (ws *WorkflowScheduler) nocTargetMatches(job *models.WorkflowJob, row *models.NocDataDevice) bool {
	targetType := strings.ToLower(strings.TrimSpace(job.TargetType))
	targetValue := strings.TrimSpace(job.TargetValue)
	switch targetType {
	case "all_networks":
		return true
	case "network_type":
		return nocWorkflowNetworkTypeFromSite(row.Site) == strings.ToLower(targetValue)
	case "province":
		return strings.EqualFold(nocWorkflowProvinceFromSite(row.Site), targetValue)
	case "vendor":
		return strings.EqualFold(normalizeNocWorkflowSchedulerVendor(row.Vendor, row.DeviceModel), targetValue)
	case "model":
		return strings.EqualFold(strings.TrimSpace(row.DeviceModel), targetValue)
	default:
		return false
	}
}

func normalizeNocWorkflowSchedulerVendor(vendor, model string) string {
	normalizedVendor := strings.ToLower(strings.TrimSpace(vendor))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if (normalizedVendor == "cisco_ios" || normalizedVendor == "cisco_nexus" || normalizedVendor == "cisco") &&
		(strings.Contains(normalizedModel, "nexus") || strings.Contains(normalizedModel, "n9k") || strings.Contains(normalizedModel, "nexus9000")) {
		return "cisco_nexus"
	}
	return normalizedVendor
}

func nocWorkflowProvinceFromSite(site string) string {
	trimmed := strings.TrimSpace(site)
	if trimmed == "" {
		return "Unknown"
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "Unknown"
	}
	first := strings.ToLower(parts[0])
	return strings.ToUpper(first[:1]) + first[1:]
}

func nocWorkflowNetworkTypeFromSite(site string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(site)), "ftth") {
		return "ftth"
	}
	return "wifi"
}

func (ws *WorkflowScheduler) saveBackupFile(device *models.WorkflowDevice, output string, t time.Time) string {
	vendor := strings.ToLower(device.Vendor)
	folder := filepath.Join("backups", ws.scope+"-team", vendor, t.Format("2006-01-02"))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		log.Printf("[workflow:%s] saveBackupFile mkdir %s: %v", ws.scope, folder, err)
	}
	name := strings.ReplaceAll(device.Name, "/", "-")
	filename := fmt.Sprintf("%s_%s.txt", name, device.Host)
	path := filepath.Join(folder, filename)
	if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
		log.Printf("[workflow:%s] saveBackupFile write %s: %v", ws.scope, path, err)
	}
	return path
}

// ──────────────────────── Log helpers ────────────────────────────────────────

func (ws *WorkflowScheduler) writeJobLog(
	j *models.WorkflowJob,
	runID *uint,
	level, event, msg string,
	durMs int64,
) {
	jid := j.ID
	entry := &models.WorkflowLog{
		JobID:      &jid,
		RunID:      runID,
		DeviceName: j.Device.Name,
		Host:       j.Device.Host,
		Vendor:     j.Device.Vendor,
		JobType:    j.JobType,
		Command:    j.Command,
		Level:      level,
		Event:      event,
		Message:    msg,
		DurationMs: durMs,
	}
	if err := ws.repo.WriteLog(entry); err != nil {
		log.Printf("[workflow-log:%s] write failed: %v", ws.scope, err)
	}
}

func (ws *WorkflowScheduler) writeSystemLog(jobType, level, event, msg string) {
	entry := &models.WorkflowLog{
		JobType: jobType,
		Level:   level,
		Event:   event,
		Message: msg,
	}
	if err := ws.repo.WriteLog(entry); err != nil {
		log.Printf("[workflow-log:%s] write failed: %v", ws.scope, err)
	}
}

func (ws *WorkflowScheduler) Stop() {
	ws.writeSystemLog("system", "info", "scheduler_stopped", "Workflow scheduler stopped cleanly")
	if err := ws.sched.Shutdown(); err != nil {
		log.Printf("[workflow-scheduler:%s] shutdown error: %v", ws.scope, err)
	}
}

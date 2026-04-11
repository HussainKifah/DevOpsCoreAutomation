package scheduler

import (
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
	repo      repository.WorkflowRepository
	jwtSecret []byte
	sched     gocron.Scheduler
	scope     string
	// jobMap stores the gocron tag string for each registered WorkflowJob ID.
	// We use the tag (not the gocron.Job value) because gocron.Job.ID() returns
	// a uuid.UUID, not a uint, making it unsuitable as a map key for our purposes.
	jobMap map[uint]string
}

func NewWorkflowScheduler(repo repository.WorkflowRepository, jwtSecret []byte) (*WorkflowScheduler, error) {
	return NewWorkflowSchedulerForScope(repo, jwtSecret, "ip")
}

func NewWorkflowSchedulerForScope(repo repository.WorkflowRepository, jwtSecret []byte, scope string) (*WorkflowScheduler, error) {
	if scope == "" {
		scope = "ip"
	}
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}
	return &WorkflowScheduler{
		repo:      repo,
		jwtSecret: jwtSecret,
		sched:     s,
		scope:     scope,
		jobMap:    make(map[uint]string),
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
	if j.Device.Host == "" {
		msg := fmt.Sprintf("device for job %d has no host (may have been deleted)", jobID)
		log.Printf("[workflow:%s] job %d: %s", ws.scope, jobID, msg)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
		return
	}
	log.Printf("[workflow:%s] job %d: device=%s host=%s vendor=%s type=%s cmd=%q schedule=%s",
		ws.scope, j.ID, j.Device.Name, j.Device.Host, j.Device.Vendor, j.JobType, j.Command, j.Schedule)

	// ── Decrypt credentials ──────────────────────────────────────────────────
	log.Printf("[workflow:%s] job %d: decrypting credentials...", ws.scope, j.ID)
	user, err := crypto.Decrypt(ws.jwtSecret, j.Device.EncUsername)
	if err != nil {
		msg := "credential decrypt failed: " + err.Error()
		ws.writeJobLog(j, nil, "error", "job_failed", msg, 0)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
		log.Printf("[workflow:%s] job %d: %s", ws.scope, j.ID, msg)
		return
	}
	pass, err := crypto.Decrypt(ws.jwtSecret, j.Device.EncPassword)
	if err != nil {
		msg := "credential decrypt failed: " + err.Error()
		ws.writeJobLog(j, nil, "error", "job_failed", msg, 0)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
		log.Printf("[workflow:%s] job %d: %s", ws.scope, j.ID, msg)
		return
	}
	log.Printf("[workflow:%s] job %d: credentials decrypted OK (user=%q)", ws.scope, j.ID, user)

	// ── Create run record ────────────────────────────────────────────────────
	startedAt := time.Now()
	run := &models.WorkflowRun{
		JobID:     j.ID,
		StartedAt: startedAt,
		Status:    "pending",
	}
	if createErr := ws.repo.CreateRun(run); createErr != nil {
		msg := "failed to create run record: " + createErr.Error()
		log.Printf("[workflow:%s] job %d: %s", ws.scope, j.ID, msg)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
		return
	}
	log.Printf("[workflow:%s] job %d: run record created (run_id=%d)", ws.scope, j.ID, run.ID)

	ws.writeJobLog(j, &run.ID, "info", "job_started",
		fmt.Sprintf("Started %s job on %s (%s)", j.JobType, j.Device.Name, j.Device.Host), 0)

	// ── Resolve command ──────────────────────────────────────────────────────
	var cmd string
	if j.JobType == "backup" {
		cmd, err = shell.IPBackupCommand(j.Device.Vendor)
		if err != nil {
			msg := "cannot resolve backup command for vendor: " + err.Error()
			ws.writeJobLog(j, &run.ID, "error", "job_failed", msg, 0)
			_ = ws.repo.FinishRun(run.ID, "error", "", msg, time.Now())
			_ = ws.repo.UpdateJobStatus(j.ID, "error", msg, time.Now())
			log.Printf("[workflow:%s] job %d: %s", ws.scope, j.ID, msg)
			return
		}
	} else {
		cmd = j.Command
	}
	log.Printf("[workflow:%s] job %d: SSH connecting to %s (vendor=%s, transport=%s)...",
		ws.scope, j.ID, j.Device.Host, j.Device.Vendor, "auto")
	log.Printf("[workflow:%s] job %d: sending command: %q", ws.scope, j.ID, cmd)

	// ── Execute via SSH ──────────────────────────────────────────────────────
	output, execErr := shell.IPSendCommand(j.Device.Host, user, pass, j.Device.Vendor, cmd)

	finishedAt := time.Now()
	durationMs := finishedAt.Sub(startedAt).Milliseconds()

	if execErr != nil {
		errMsg := execErr.Error()
		_ = ws.repo.FinishRun(run.ID, "error", output, errMsg, finishedAt)
		_ = ws.repo.UpdateJobStatus(j.ID, "error", errMsg, finishedAt)
		ws.writeJobLog(j, &run.ID, "error", "job_failed",
			fmt.Sprintf("FAILED after %dms: %s", durationMs, errMsg), durationMs)
		log.Printf("[workflow:%s] job %d: FAILED in %dms: %s", ws.scope, j.ID, durationMs, errMsg)
		if len(output) > 0 {
			preview := output
			if len(preview) > 500 {
				preview = preview[:500] + "...(truncated)"
			}
			log.Printf("[workflow:%s] job %d: partial output:\n%s", ws.scope, j.ID, preview)
		}
		return
	}

	// ── Success ──────────────────────────────────────────────────────────────
	_ = ws.repo.FinishRun(run.ID, "ok", output, "", finishedAt)
	_ = ws.repo.UpdateJobStatus(j.ID, "ok", "", finishedAt)

	successMsg := fmt.Sprintf("Completed in %dms — %d bytes collected", durationMs, len(output))
	if j.JobType == "backup" {
		savedPath := ws.saveBackupFile(j, output, startedAt)
		successMsg = fmt.Sprintf("Backup completed in %dms — %d bytes — saved to %s",
			durationMs, len(output), savedPath)
	}

	ws.writeJobLog(j, &run.ID, "success", "job_success", successMsg, durationMs)
	log.Printf("[workflow:%s] job %d: OK in %dms (%d bytes)", ws.scope, j.ID, durationMs, len(output))
	if len(output) > 0 {
		preview := output
		if len(preview) > 300 {
			preview = preview[:300] + "...(truncated)"
		}
		log.Printf("[workflow:%s] job %d: output preview:\n%s", ws.scope, j.ID, preview)
	}
	log.Printf("[workflow:%s] END JOB %d", ws.scope, jobID)
}

func (ws *WorkflowScheduler) saveBackupFile(j *models.WorkflowJob, output string, t time.Time) string {
	vendor := strings.ToLower(j.Device.Vendor)
	folder := filepath.Join("backups", ws.scope+"-team", vendor, t.Format("2006-01-02"))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		log.Printf("[workflow:%s] saveBackupFile mkdir %s: %v", ws.scope, folder, err)
	}
	name := strings.ReplaceAll(j.Device.Name, "/", "-")
	filename := fmt.Sprintf("%s_%s.txt", name, j.Device.Host)
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

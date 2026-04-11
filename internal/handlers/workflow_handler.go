package handlers

import (
	"net/http"
	"sort"
	"strconv"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

// WorkflowSchedulerInterface is the subset of WorkflowScheduler the handler needs.
type WorkflowSchedulerInterface interface {
	ReloadAll()
	RunJobNow(jobID uint)
}

type WorkflowHandler struct {
	repo      repository.WorkflowRepository
	sched     WorkflowSchedulerInterface
	cryptoKey []byte
}

func NewWorkflowHandler(
	repo repository.WorkflowRepository,
	sched WorkflowSchedulerInterface,
	cryptoKey []byte,
) *WorkflowHandler {
	return &WorkflowHandler{repo: repo, sched: sched, cryptoKey: cryptoKey}
}

// ──────────────────────── Devices ────────────────────────

func (h *WorkflowHandler) ListDevices(c *gin.Context) {
	devices, err := h.repo.ListDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Credentials (EncUsername / EncPassword) are never sent to the client.
	type safeDevice struct {
		ID     uint   `json:"ID"`
		Name   string `json:"name"`
		Host   string `json:"host"`
		Vendor string `json:"vendor"`
	}
	out := make([]safeDevice, len(devices))
	for i, d := range devices {
		out[i] = safeDevice{ID: d.ID, Name: d.Name, Host: d.Host, Vendor: d.Vendor}
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkflowHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Name     string `json:"name"     binding:"required"`
		Host     string `json:"host"     binding:"required"`
		Vendor   string `json:"vendor"   binding:"required,oneof=nokia cisco mikrotik huawei"`
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	encUser, err := crypto.Encrypt(h.cryptoKey, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt username: " + err.Error()})
		return
	}
	encPass, err := crypto.Encrypt(h.cryptoKey, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password: " + err.Error()})
		return
	}

	dev := &models.WorkflowDevice{
		Name:        req.Name,
		Host:        req.Host,
		Vendor:      req.Vendor,
		EncUsername: encUser,
		EncPassword: encPass,
		CreatedByID: callerID(c),
	}
	if err := h.repo.CreateDevice(dev); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"ID":     dev.ID,
		"name":   dev.Name,
		"host":   dev.Host,
		"vendor": dev.Vendor,
	})
}

func (h *WorkflowHandler) UpdateDevice(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	dev, err := h.repo.GetDevice(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	var req struct {
		Name     string `json:"name"`
		Host     string `json:"host"`
		Vendor   string `json:"vendor"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Name != "" {
		dev.Name = req.Name
	}
	if req.Host != "" {
		dev.Host = req.Host
	}
	if req.Vendor != "" {
		validVendors := map[string]bool{"nokia": true, "cisco": true, "mikrotik": true, "huawei": true}
		if !validVendors[req.Vendor] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "vendor must be nokia, cisco, mikrotik, or huawei"})
			return
		}
		dev.Vendor = req.Vendor
	}
	// Only re-encrypt credentials if new values were provided.
	if req.Username != "" {
		enc, encErr := crypto.Encrypt(h.cryptoKey, req.Username)
		if encErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt username: " + encErr.Error()})
			return
		}
		dev.EncUsername = enc
	}
	if req.Password != "" {
		enc, encErr := crypto.Encrypt(h.cryptoKey, req.Password)
		if encErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password: " + encErr.Error()})
			return
		}
		dev.EncPassword = enc
	}

	if err := h.repo.UpdateDevice(dev); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *WorkflowHandler) DeleteDevice(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteDevice(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ──────────────────────── Jobs ────────────────────────

func (h *WorkflowHandler) ListJobs(c *gin.Context) {
	jobs, err := h.repo.ListJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *WorkflowHandler) CreateJob(c *gin.Context) {
	var req struct {
		DeviceID uint   `json:"device_id" binding:"required"`
		JobType  string `json:"job_type"  binding:"required,oneof=backup command"`
		Command  string `json:"command"`
		Schedule string `json:"schedule"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.JobType == "command" && req.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required when job_type is command"})
		return
	}
	if _, err := h.repo.GetDevice(req.DeviceID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device not found"})
		return
	}

	runOnce := req.Schedule == "once"

	job := &models.WorkflowJob{
		DeviceID:    req.DeviceID,
		JobType:     req.JobType,
		Command:     req.Command,
		Schedule:    req.Schedule,
		Enabled:     !runOnce,
		LastStatus:  "pending",
		CreatedByID: callerID(c),
	}
	if err := h.repo.CreateJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if runOnce {
		h.sched.RunJobNow(job.ID)
	} else {
		h.sched.ReloadAll()
	}
	c.JSON(http.StatusCreated, job)
}

func (h *WorkflowHandler) UpdateJob(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	job, err := h.repo.GetJob(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	var req struct {
		Schedule *string `json:"schedule"`
		Enabled  *bool   `json:"enabled"`
		Command  *string `json:"command"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Schedule != nil {
		job.Schedule = *req.Schedule
	}
	if req.Enabled != nil {
		job.Enabled = *req.Enabled
	}
	if req.Command != nil {
		job.Command = *req.Command
	}
	if err := h.repo.UpdateJob(job); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.sched.ReloadAll()
	c.JSON(http.StatusOK, gin.H{"message": "updated"})
}

func (h *WorkflowHandler) DeleteJob(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteJob(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.sched.ReloadAll()
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *WorkflowHandler) RunJobNow(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h.sched.RunJobNow(id)
	c.JSON(http.StatusOK, gin.H{"status": "started"})
}

func (h *WorkflowHandler) GetRuns(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	runs, err := h.repo.ListRunsForJob(id, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, runs)
}

// ──────────────────── Runs (Backups + Command Output pages) ──────────────────────

// EnrichedRun adds device context to a run for the Backups and Command Output pages.
type EnrichedRun struct {
	ID         uint    `json:"id"`
	JobID      uint    `json:"job_id"`
	JobType    string  `json:"job_type"`
	DeviceName string  `json:"device_name"`
	Host       string  `json:"host"`
	Vendor     string  `json:"vendor"`
	Command    string  `json:"command"`
	StartedAt  *string `json:"started_at"`
	FinishedAt *string `json:"finished_at"`
	Status     string  `json:"status"`
	ErrorMsg   string  `json:"error_msg"`
	OutputSize int     `json:"output_size"`
}

// GetRunsByType — GET /api/workflows/runs?type=backup|command
// Returns all runs enriched with device info, newest first.
func (h *WorkflowHandler) GetRunsByType(c *gin.Context) {
	jobType := c.Query("type") // "backup", "command", or "" for all

	jobs, err := h.repo.ListJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Always return a JSON array, never null.
	result := make([]EnrichedRun, 0)

	for i := range jobs {
		j := jobs[i]
		if jobType != "" && j.JobType != jobType {
			continue
		}
		runs, err := h.repo.ListRunsForJob(j.ID, 500)
		if err != nil {
			continue
		}
		for _, r := range runs {
			s := r.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
			er := EnrichedRun{
				ID:         r.ID,
				JobID:      j.ID,
				JobType:    j.JobType,
				DeviceName: j.Device.Name,
				Host:       j.Device.Host,
				Vendor:     j.Device.Vendor,
				Command:    j.Command,
				StartedAt:  &s,
				Status:     r.Status,
				ErrorMsg:   r.ErrorMsg,
				OutputSize: len(r.Output),
			}
			if r.FinishedAt != nil {
				f := r.FinishedAt.UTC().Format("2006-01-02T15:04:05Z")
				er.FinishedAt = &f
			}
			result = append(result, er)
		}
	}

	// Sort newest first.
	sort.Slice(result, func(i, k int) bool {
		if result[i].StartedAt == nil {
			return false
		}
		if result[k].StartedAt == nil {
			return true
		}
		return *result[i].StartedAt > *result[k].StartedAt
	})

	c.JSON(http.StatusOK, result)
}

// GetRunOutput — GET /api/workflows/runs/:id/output
// Returns the full output text for one run (lazy-loaded by the UI to keep list fast).
func (h *WorkflowHandler) GetRunOutput(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	run, err := h.repo.GetRunByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"output": run.Output})
}

// ──────────────────────── Activity Logs ────────────────────────

// GetLogs — GET /api/workflows/logs
// Query params: level, job_type, event, search, page, per_page
func (h *WorkflowHandler) GetLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	filter := repository.LogFilter{
		Level:   c.Query("level"),
		JobType: c.Query("job_type"),
		Event:   c.Query("event"),
		Search:  c.Query("search"),
		Page:    page,
		PerPage: perPage,
	}

	logs, total, err := h.repo.ListLogs(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Guard against division by zero.
	totalPages := 1
	if perPage > 0 && total > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
	}

	// Always return an array, never null.
	if logs == nil {
		logs = []models.WorkflowLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":        logs,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
		"total_pages": totalPages,
	})
}

// ──────────────────────── private helpers ────────────────────────

// parseID extracts the ":id" URL parameter and converts it to uint.
func parseID(c *gin.Context) (uint, error) {
	v, err := strconv.ParseUint(c.Param("id"), 10, 32)
	return uint(v), err
}

// callerID returns the authenticated user's ID from the JWT claims, or 0.
func callerID(c *gin.Context) uint {
	if claims, ok := c.Get("user"); ok {
		if uc, ok := claims.(*auth.Claims); ok {
			return uc.UserID
		}
	}
	return 0
}

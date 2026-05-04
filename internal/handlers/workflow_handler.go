package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WorkflowSchedulerInterface is the subset of WorkflowScheduler the handler needs.
type WorkflowSchedulerInterface interface {
	ReloadAll()
	RunJobNow(jobID uint)
}

const (
	workflowTargetDevice      = "device"
	workflowTargetAllNetworks = "all_networks"
	workflowTargetNetworkType = "network_type"
	workflowTargetDeviceType  = "device_type"
	workflowTargetProvince    = "province"
	workflowTargetVendor      = "vendor"
	workflowTargetModel       = "model"
)

type WorkflowHandler struct {
	repo            repository.WorkflowRepository
	sched           WorkflowSchedulerInterface
	cryptoKey       []byte
	nocDataRepo     repository.NocDataRepository
	autoSyncNocData bool
}

func NewWorkflowHandler(
	repo repository.WorkflowRepository,
	sched WorkflowSchedulerInterface,
	cryptoKey []byte,
) *WorkflowHandler {
	return &WorkflowHandler{repo: repo, sched: sched, cryptoKey: cryptoKey}
}

func NewNocWorkflowHandler(
	repo repository.WorkflowRepository,
	sched WorkflowSchedulerInterface,
	cryptoKey []byte,
	nocDataRepo repository.NocDataRepository,
) *WorkflowHandler {
	return &WorkflowHandler{
		repo:            repo,
		sched:           sched,
		cryptoKey:       cryptoKey,
		nocDataRepo:     nocDataRepo,
		autoSyncNocData: true,
	}
}

// ──────────────────────── Devices ────────────────────────

func (h *WorkflowHandler) ListDevices(c *gin.Context) {
	if h.autoSyncNocData {
		if err := h.syncNocWorkflowDevices(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	devices, err := h.repo.ListDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	nocDataByHost := h.nocDataLookupByHost()
	// Credentials (EncUsername / EncPassword) are never sent to the client.
	type safeDevice struct {
		ID           uint   `json:"ID"`
		Name         string `json:"name"`
		Host         string `json:"host"`
		Vendor       string `json:"vendor"`
		NetworkType  string `json:"network_type,omitempty"`
		Province     string `json:"province,omitempty"`
		DeviceType   string `json:"device_type,omitempty"`
		SourceVendor string `json:"source_vendor,omitempty"`
		Site         string `json:"site,omitempty"`
		Model        string `json:"model,omitempty"`
	}
	out := make([]safeDevice, len(devices))
	for i, d := range devices {
		match := nocDataByHost[strings.ToLower(strings.TrimSpace(d.Host))]
		sourceVendor := d.Vendor
		site := ""
		model := ""
		if match != nil {
			sourceVendor = normalizeNocWorkflowSourceVendor(match.Vendor, match.DeviceModel)
			site = strings.TrimSpace(match.Site)
			model = strings.TrimSpace(match.DeviceModel)
		}
		networkType := firstNonEmpty(normalizeWorkflowNetworkType(d.NetworkType), "wifi")
		if h.autoSyncNocData {
			networkType = networkTypeFromSite(site)
		}
		out[i] = safeDevice{
			ID:           d.ID,
			Name:         d.Name,
			Host:         d.Host,
			Vendor:       d.Vendor,
			Province:     firstNonEmpty(strings.TrimSpace(d.Province), provinceFromSite(site)),
			DeviceType:   strings.TrimSpace(d.DeviceType),
			SourceVendor: sourceVendor,
			Site:         site,
			Model:        model,
			NetworkType:  networkType,
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *WorkflowHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Name        string `json:"name"     binding:"required"`
		Host        string `json:"host"     binding:"required"`
		Vendor      string `json:"vendor"   binding:"required,oneof=nokia cisco mikrotik huawei"`
		NetworkType string `json:"network_type"`
		Province    string `json:"province"`
		DeviceType  string `json:"device_type"`
		Username    string `json:"username" binding:"required"`
		Password    string `json:"password" binding:"required"`
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
		NetworkType: defaultWorkflowNetworkType(req.NetworkType),
		Province:    strings.TrimSpace(req.Province),
		DeviceType:  strings.TrimSpace(req.DeviceType),
		EncUsername: encUser,
		EncPassword: encPass,
		CreatedByID: callerID(c),
	}
	if err := h.repo.CreateDevice(dev); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"ID":           dev.ID,
		"name":         dev.Name,
		"host":         dev.Host,
		"vendor":       dev.Vendor,
		"network_type": dev.NetworkType,
		"province":     dev.Province,
		"device_type":  dev.DeviceType,
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
		Name        string `json:"name"`
		Host        string `json:"host"`
		Vendor      string `json:"vendor"`
		NetworkType string `json:"network_type"`
		Province    string `json:"province"`
		DeviceType  string `json:"device_type"`
		Username    string `json:"username"`
		Password    string `json:"password"`
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
	dev.NetworkType = defaultWorkflowNetworkType(req.NetworkType)
	dev.Province = strings.TrimSpace(req.Province)
	dev.DeviceType = strings.TrimSpace(req.DeviceType)
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

func (h *WorkflowHandler) nocDataLookupByHost() map[string]*models.NocDataDevice {
	lookup := map[string]*models.NocDataDevice{}
	if h.nocDataRepo == nil {
		return lookup
	}
	list, err := h.nocDataRepo.ListAll()
	if err != nil {
		return lookup
	}
	for i := range list {
		host := strings.ToLower(strings.TrimSpace(list[i].Host))
		if host == "" {
			continue
		}
		item := list[i]
		lookup[host] = &item
	}
	return lookup
}

func (h *WorkflowHandler) syncNocWorkflowDevices() error {
	if h.nocDataRepo == nil {
		return nil
	}

	rows, err := h.nocDataRepo.ListAll()
	if err != nil {
		return err
	}

	for i := range rows {
		row := rows[i]
		workflowVendor, ok := workflowVendorFromNocData(row.Vendor, row.DeviceModel)
		if !ok {
			continue
		}
		if strings.TrimSpace(row.Host) == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(row.AccessMethod), "pending") {
			continue
		}
		if len(row.EncUsername) == 0 || len(row.EncPassword) == 0 {
			continue
		}

		name := strings.TrimSpace(row.Hostname)
		if name == "" {
			name = strings.TrimSpace(row.DisplayName)
		}
		if name == "" {
			name = strings.TrimSpace(row.Host)
		}

		existing, err := h.repo.GetDeviceByHost(row.Host)
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				createErr := h.repo.CreateDevice(&models.WorkflowDevice{
					Name:        name,
					Host:        strings.TrimSpace(row.Host),
					Vendor:      workflowVendor,
					EncUsername: append([]byte(nil), row.EncUsername...),
					EncPassword: append([]byte(nil), row.EncPassword...),
					CreatedByID: 0,
				})
				if createErr != nil {
					return createErr
				}
				continue
			}
			return err
		}

		existing.Name = name
		existing.Host = strings.TrimSpace(row.Host)
		existing.Vendor = workflowVendor
		existing.EncUsername = append([]byte(nil), row.EncUsername...)
		existing.EncPassword = append([]byte(nil), row.EncPassword...)
		if err := h.repo.UpdateDevice(existing); err != nil {
			return err
		}
	}

	return nil
}

func workflowVendorFromNocData(vendor, model string) (string, bool) {
	switch normalizeNocWorkflowSourceVendor(vendor, model) {
	case "cisco_ios", "cisco_nexus", "cisco":
		return "cisco", true
	case "mikrotik":
		return "mikrotik", true
	case "huawei":
		return "huawei", true
	default:
		return "", false
	}
}

func normalizeNocWorkflowSourceVendor(vendor, model string) string {
	normalizedVendor := strings.ToLower(strings.TrimSpace(vendor))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if (normalizedVendor == "cisco_ios" || normalizedVendor == "cisco_nexus" || normalizedVendor == "cisco") &&
		(strings.Contains(normalizedModel, "nexus") || strings.Contains(normalizedModel, "n9k") || strings.Contains(normalizedModel, "nexus9000")) {
		return "cisco_nexus"
	}
	return normalizedVendor
}

func provinceFromSite(site string) string {
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

func networkTypeFromSite(site string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(site)), "ftth") {
		return "ftth"
	}
	return "wifi"
}

func normalizeWorkflowNetworkType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ftth":
		return "ftth"
	case "wifi":
		return "wifi"
	default:
		return ""
	}
}

func defaultWorkflowNetworkType(value string) string {
	if normalized := normalizeWorkflowNetworkType(value); normalized != "" {
		return normalized
	}
	return "wifi"
}

func workflowVendorDisplayLabel(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_nexus":
		return "Cisco Nexus"
	case "cisco_ios", "cisco":
		return "Cisco"
	case "mikrotik":
		return "Microtik"
	case "huawei":
		return "Huawei"
	case "nokia":
		return "Nokia"
	default:
		if vendor == "" {
			return "Unknown"
		}
		return vendor
	}
}

func (h *WorkflowHandler) defaultTargetLabel(targetType, targetValue string, device *models.WorkflowDevice) string {
	switch targetType {
	case workflowTargetDevice:
		if device != nil {
			if strings.TrimSpace(device.Name) != "" {
				return strings.TrimSpace(device.Name)
			}
			if strings.TrimSpace(device.Host) != "" {
				return strings.TrimSpace(device.Host)
			}
		}
		return "Device"
	case workflowTargetAllNetworks:
		return "All Networks"
	case workflowTargetNetworkType:
		if strings.EqualFold(strings.TrimSpace(targetValue), "ftth") {
			return "FTTH"
		}
		return "WiFi"
	case workflowTargetDeviceType:
		if strings.TrimSpace(targetValue) == "" {
			return "Unknown Type"
		}
		return strings.TrimSpace(targetValue)
	case workflowTargetProvince:
		return provinceFromSite(targetValue)
	case workflowTargetVendor:
		return workflowVendorDisplayLabel(targetValue)
	case workflowTargetModel:
		return strings.TrimSpace(targetValue)
	default:
		return strings.TrimSpace(targetValue)
	}
}

func normalizeNocWorkflowGroupTarget(targetType, targetValue string) (string, string, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(targetType))
	normalizedValue := strings.TrimSpace(targetValue)
	switch normalizedType {
	case workflowTargetAllNetworks:
		return normalizedType, "all", nil
	case workflowTargetNetworkType:
		value := strings.ToLower(normalizedValue)
		if value != "ftth" && value != "wifi" {
			return "", "", errors.New("invalid network type")
		}
		return normalizedType, value, nil
	case workflowTargetProvince:
		if normalizedValue == "" {
			return "", "", errors.New("invalid province")
		}
		return normalizedType, provinceFromSite(normalizedValue), nil
	case workflowTargetDeviceType:
		if normalizedValue == "" {
			return "", "", errors.New("invalid device type")
		}
		return normalizedType, normalizedValue, nil
	case workflowTargetVendor:
		value := normalizeNocWorkflowSourceVendor(normalizedValue, "")
		switch value {
		case "cisco_ios", "cisco", "cisco_nexus", "mikrotik", "huawei":
			return normalizedType, value, nil
		default:
			return "", "", errors.New("invalid vendor")
		}
	case workflowTargetModel:
		if normalizedValue == "" {
			return "", "", errors.New("invalid model")
		}
		return normalizedType, normalizedValue, nil
	default:
		return "", "", errors.New("invalid target type")
	}
}

// ──────────────────────── Jobs ────────────────────────

func (h *WorkflowHandler) ListJobs(c *gin.Context) {
	jobs, err := h.repo.ListJobs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range jobs {
		if strings.TrimSpace(jobs[i].TargetType) == "" {
			jobs[i].TargetType = workflowTargetDevice
		}
		if strings.TrimSpace(jobs[i].TargetLabel) == "" {
			device := &jobs[i].Device
			if jobs[i].DeviceID == nil {
				device = nil
			}
			jobs[i].TargetLabel = h.defaultTargetLabel(jobs[i].TargetType, jobs[i].TargetValue, device)
		}
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *WorkflowHandler) CreateJob(c *gin.Context) {
	var req struct {
		DeviceID    uint   `json:"device_id"`
		TargetType  string `json:"target_type"`
		TargetValue string `json:"target_value"`
		TargetLabel string `json:"target_label"`
		JobType     string `json:"job_type"  binding:"required,oneof=backup command"`
		Command     string `json:"command"`
		Schedule    string `json:"schedule"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	req.Command = strings.TrimSpace(req.Command)
	if req.JobType == "command" && req.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "command is required when job_type is command"})
		return
	}
	runOnce := req.Schedule == "once"
	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	targetValue := strings.TrimSpace(req.TargetValue)
	targetLabel := strings.TrimSpace(req.TargetLabel)

	var device *models.WorkflowDevice
	if targetType == "" && req.DeviceID > 0 {
		targetType = workflowTargetDevice
	}

	if h.autoSyncNocData {
		switch targetType {
		case "", workflowTargetDevice:
			if req.DeviceID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required for device targets"})
				return
			}
			var err error
			device, err = h.repo.GetDevice(req.DeviceID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "device not found"})
				return
			}
			targetType = workflowTargetDevice
			targetLabel = h.defaultTargetLabel(targetType, targetValue, device)
		case workflowTargetAllNetworks, workflowTargetNetworkType, workflowTargetProvince, workflowTargetVendor, workflowTargetModel:
			var err error
			targetType, targetValue, err = normalizeNocWorkflowGroupTarget(targetType, targetValue)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid group target"})
				return
			}
			if targetLabel == "" {
				targetLabel = h.defaultTargetLabel(targetType, targetValue, nil)
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
			return
		}
	} else {
		switch targetType {
		case "", workflowTargetDevice:
			if req.DeviceID == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
				return
			}
			var err error
			device, err = h.repo.GetDevice(req.DeviceID)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "device not found"})
				return
			}
			targetType = workflowTargetDevice
			targetLabel = h.defaultTargetLabel(targetType, targetValue, device)
		case workflowTargetAllNetworks:
			targetValue = "all"
			if targetLabel == "" {
				targetLabel = "All Devices"
			}
		case workflowTargetNetworkType:
			targetValue = defaultWorkflowNetworkType(targetValue)
			if targetLabel == "" {
				targetLabel = h.defaultTargetLabel(targetType, targetValue, nil)
			}
		case workflowTargetDeviceType, workflowTargetProvince, workflowTargetVendor:
			targetValue = strings.TrimSpace(targetValue)
			if targetValue == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "target_value is required"})
				return
			}
			if targetLabel == "" {
				targetLabel = h.defaultTargetLabel(targetType, targetValue, nil)
			}
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
			return
		}
	}

	var deviceID *uint
	if targetType == workflowTargetDevice {
		deviceID = &req.DeviceID
	}

	job := &models.WorkflowJob{
		DeviceID:    deviceID,
		TargetType:  targetType,
		TargetValue: targetValue,
		TargetLabel: targetLabel,
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
	var dayStart, dayEnd time.Time
	if rawDate := strings.TrimSpace(c.Query("date")); rawDate != "" {
		day, err := time.Parse("2006-01-02", rawDate)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
			return
		}
		dayStart = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
		dayEnd = dayStart.Add(24 * time.Hour)
	}

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
			if !dayStart.IsZero() {
				started := r.StartedAt.UTC()
				if started.Before(dayStart) || !started.Before(dayEnd) {
					continue
				}
			}
			s := r.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
			er := EnrichedRun{
				ID:         r.ID,
				JobID:      j.ID,
				JobType:    j.JobType,
				DeviceName: firstNonEmpty(strings.TrimSpace(r.DeviceName), strings.TrimSpace(j.TargetLabel), strings.TrimSpace(j.Device.Name)),
				Host:       firstNonEmpty(strings.TrimSpace(r.Host), strings.TrimSpace(j.Device.Host)),
				Vendor:     firstNonEmpty(strings.TrimSpace(r.Vendor), strings.TrimSpace(j.Device.Vendor)),
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

type BackupCompareLine struct {
	Text  string `json:"text"`
	Count int    `json:"count"`
}

type BackupCompareDevice struct {
	Host             string              `json:"host"`
	DeviceName       string              `json:"device_name"`
	Vendor           string              `json:"vendor"`
	BaseRunID        uint                `json:"base_run_id,omitempty"`
	CompareRunID     uint                `json:"compare_run_id,omitempty"`
	BaseStartedAt    *string             `json:"base_started_at,omitempty"`
	CompareStartedAt *string             `json:"compare_started_at,omitempty"`
	BaseSize         int                 `json:"base_size"`
	CompareSize      int                 `json:"compare_size"`
	AddedLines       []BackupCompareLine `json:"added_lines,omitempty"`
	RemovedLines     []BackupCompareLine `json:"removed_lines,omitempty"`
}

type BackupCompareResponse struct {
	BaseDate    string                `json:"base_date"`
	CompareDate string                `json:"compare_date"`
	Changed     []BackupCompareDevice `json:"changed"`
	Added       []BackupCompareDevice `json:"added"`
	Missing     []BackupCompareDevice `json:"missing"`
	Unchanged   int                   `json:"unchanged"`
}

type BNGSyncBackupInfo struct {
	RunID      uint    `json:"run_id"`
	StartedAt  *string `json:"started_at,omitempty"`
	DeviceID   uint    `json:"device_id"`
	DeviceName string  `json:"device_name"`
	Host       string  `json:"host"`
	Vendor     string  `json:"vendor"`
	OutputSize int     `json:"output_size"`
	CreatedNow bool    `json:"created_now"`
}

type BNGSyncResponse struct {
	InSync       bool                `json:"in_sync"`
	Message      string              `json:"message"`
	Left         BNGSyncBackupInfo   `json:"left"`
	Right        BNGSyncBackupInfo   `json:"right"`
	AddedLines   []BackupCompareLine `json:"added_lines"`
	MissingLines []BackupCompareLine `json:"missing_lines"`
}

// CompareBackups compares latest successful backup output per device between two dates.
func (h *WorkflowHandler) CompareBackups(c *gin.Context) {
	baseRaw := strings.TrimSpace(c.Query("base"))
	compareRaw := strings.TrimSpace(c.Query("compare"))
	baseDay, err := time.Parse("2006-01-02", baseRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid base date"})
		return
	}
	compareDay, err := time.Parse("2006-01-02", compareRaw)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid compare date"})
		return
	}

	baseRuns, err := h.latestBackupRunsByHost(baseDay)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	compareRuns, err := h.latestBackupRunsByHost(compareDay)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := BackupCompareResponse{
		BaseDate:    baseRaw,
		CompareDate: compareRaw,
		Changed:     []BackupCompareDevice{},
		Added:       []BackupCompareDevice{},
		Missing:     []BackupCompareDevice{},
	}

	seen := make(map[string]struct{}, len(baseRuns)+len(compareRuns))
	for host, baseRun := range baseRuns {
		seen[host] = struct{}{}
		compareRun, ok := compareRuns[host]
		if !ok {
			resp.Missing = append(resp.Missing, backupCompareDeviceFromRun(baseRun, nil, nil, nil))
			continue
		}
		if baseRun.Output == compareRun.Output {
			resp.Unchanged++
			continue
		}
		added, removed := compareBackupLines(baseRun.Output, compareRun.Output)
		if len(added) == 0 && len(removed) == 0 {
			resp.Unchanged++
			continue
		}
		resp.Changed = append(resp.Changed, backupCompareDeviceFromRun(baseRun, compareRun, added, removed))
	}
	for host, compareRun := range compareRuns {
		if _, ok := seen[host]; ok {
			continue
		}
		resp.Added = append(resp.Added, backupCompareDeviceFromRun(nil, compareRun, nil, nil))
	}

	sort.Slice(resp.Changed, func(i, j int) bool { return resp.Changed[i].Host < resp.Changed[j].Host })
	sort.Slice(resp.Added, func(i, j int) bool { return resp.Added[i].Host < resp.Added[j].Host })
	sort.Slice(resp.Missing, func(i, j int) bool { return resp.Missing[i].Host < resp.Missing[j].Host })
	c.JSON(http.StatusOK, resp)
}

func (h *WorkflowHandler) CheckBNGSync(c *gin.Context) {
	var req struct {
		LeftDeviceID  uint `json:"left_device_id" binding:"required"`
		RightDeviceID uint `json:"right_device_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.LeftDeviceID == req.RightDeviceID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select two different BNG devices"})
		return
	}

	leftDevice, err := h.repo.GetDevice(req.LeftDeviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "left device not found"})
		return
	}
	rightDevice, err := h.repo.GetDevice(req.RightDeviceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "right device not found"})
		return
	}
	if !isBNGWorkflowDevice(leftDevice) || !isBNGWorkflowDevice(rightDevice) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "BNG Sync Checker only supports devices with type BNG"})
		return
	}

	leftRun, leftCreated, err := h.latestOrCreateDeviceBackup(leftDevice)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%s backup failed: %v", leftDevice.Name, err)})
		return
	}
	rightRun, rightCreated, err := h.latestOrCreateDeviceBackup(rightDevice)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("%s backup failed: %v", rightDevice.Name, err)})
		return
	}

	added, missing := compareBackupLines(leftRun.Output, rightRun.Output)
	inSync := len(added) == 0 && len(missing) == 0
	message := "BNG devices have the same configuration."
	if !inSync {
		message = "BNG devices do not have the same configuration."
	}

	c.JSON(http.StatusOK, BNGSyncResponse{
		InSync:       inSync,
		Message:      message,
		Left:         bngSyncBackupInfo(leftDevice, leftRun, leftCreated),
		Right:        bngSyncBackupInfo(rightDevice, rightRun, rightCreated),
		AddedLines:   added,
		MissingLines: missing,
	})
}

func (h *WorkflowHandler) latestOrCreateDeviceBackup(device *models.WorkflowDevice) (*models.WorkflowRun, bool, error) {
	run, err := h.latestSuccessfulBackupForDevice(device)
	if err != nil {
		return nil, false, err
	}
	if run != nil {
		return run, false, nil
	}
	run, err = h.runDeviceBackupNow(device)
	return run, true, err
}

func isBNGWorkflowDevice(device *models.WorkflowDevice) bool {
	return strings.EqualFold(strings.TrimSpace(device.DeviceType), "BNG")
}

func (h *WorkflowHandler) latestSuccessfulBackupForDevice(device *models.WorkflowDevice) (*models.WorkflowRun, error) {
	hostKey := strings.ToLower(strings.TrimSpace(device.Host))
	if hostKey == "" {
		return nil, fmt.Errorf("device has no host")
	}
	jobs, err := h.repo.ListJobs()
	if err != nil {
		return nil, err
	}
	var latest *models.WorkflowRun
	for i := range jobs {
		j := jobs[i]
		if j.JobType != "backup" {
			continue
		}
		if j.DeviceID != nil && *j.DeviceID != device.ID {
			continue
		}
		if j.DeviceID == nil {
			targetHost := strings.ToLower(strings.TrimSpace(j.Device.Host))
			if targetHost != "" && targetHost != hostKey {
				continue
			}
		}
		runs, err := h.repo.ListRunsForJob(j.ID, 500)
		if err != nil {
			continue
		}
		for idx := range runs {
			r := runs[idx]
			if r.Status != "ok" {
				continue
			}
			runHost := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.Host, j.Device.Host)))
			if runHost != hostKey {
				continue
			}
			if strings.TrimSpace(r.Host) == "" {
				r.Host = device.Host
			}
			if strings.TrimSpace(r.DeviceName) == "" {
				r.DeviceName = device.Name
			}
			if strings.TrimSpace(r.Vendor) == "" {
				r.Vendor = device.Vendor
			}
			if latest == nil || r.StartedAt.After(latest.StartedAt) {
				runCopy := r
				latest = &runCopy
			}
		}
	}
	return latest, nil
}

func (h *WorkflowHandler) runDeviceBackupNow(device *models.WorkflowDevice) (*models.WorkflowRun, error) {
	cmd, err := shell.IPBackupCommand(device.Vendor)
	if err != nil {
		return nil, err
	}
	user, err := crypto.Decrypt(h.cryptoKey, device.EncUsername)
	if err != nil {
		return nil, fmt.Errorf("credential decrypt failed: %w", err)
	}
	pass, err := crypto.Decrypt(h.cryptoKey, device.EncPassword)
	if err != nil {
		return nil, fmt.Errorf("credential decrypt failed: %w", err)
	}

	deviceID := device.ID
	job := &models.WorkflowJob{
		DeviceID:    &deviceID,
		TargetType:  workflowTargetDevice,
		TargetValue: device.Host,
		TargetLabel: firstNonEmpty(device.Name, device.Host),
		JobType:     "backup",
		Schedule:    "once",
		Enabled:     false,
		LastStatus:  "pending",
	}
	if err := h.repo.CreateJob(job); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	run := &models.WorkflowRun{
		JobID:      job.ID,
		DeviceName: device.Name,
		Host:       device.Host,
		Vendor:     device.Vendor,
		StartedAt:  startedAt,
		Status:     "pending",
	}
	if err := h.repo.CreateRun(run); err != nil {
		return nil, err
	}

	output, _, execErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), device.Host, user, pass, device.Vendor, "ssh", cmd)
	finishedAt := time.Now()
	if execErr != nil {
		errMsg := execErr.Error()
		_ = h.repo.FinishRun(run.ID, "error", output, errMsg, finishedAt)
		_ = h.repo.UpdateJobStatus(job.ID, "error", errMsg, finishedAt)
		return nil, execErr
	}
	if err := h.repo.FinishRun(run.ID, "ok", output, "", finishedAt); err != nil {
		return nil, err
	}
	_ = h.repo.UpdateJobStatus(job.ID, "ok", fmt.Sprintf("completed on demand for BNG sync checker - %d bytes collected", len(output)), finishedAt)
	run.Status = "ok"
	run.Output = output
	run.FinishedAt = &finishedAt
	return run, nil
}

func bngSyncBackupInfo(device *models.WorkflowDevice, run *models.WorkflowRun, createdNow bool) BNGSyncBackupInfo {
	started := run.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
	return BNGSyncBackupInfo{
		RunID:      run.ID,
		StartedAt:  &started,
		DeviceID:   device.ID,
		DeviceName: firstNonEmpty(run.DeviceName, device.Name, device.Host),
		Host:       firstNonEmpty(run.Host, device.Host),
		Vendor:     firstNonEmpty(run.Vendor, device.Vendor),
		OutputSize: len(run.Output),
		CreatedNow: createdNow,
	}
}

func (h *WorkflowHandler) latestBackupRunsByHost(day time.Time) (map[string]*models.WorkflowRun, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	jobs, err := h.repo.ListJobs()
	if err != nil {
		return nil, err
	}
	out := make(map[string]*models.WorkflowRun)
	for i := range jobs {
		j := jobs[i]
		if j.JobType != "backup" {
			continue
		}
		runs, err := h.repo.ListRunsForJob(j.ID, 500)
		if err != nil {
			continue
		}
		for i := range runs {
			r := runs[i]
			if r.Status != "ok" {
				continue
			}
			started := r.StartedAt.UTC()
			if started.Before(start) || !started.Before(end) {
				continue
			}
			hostKey := strings.ToLower(strings.TrimSpace(firstNonEmpty(r.Host, j.Device.Host, r.DeviceName, j.TargetLabel)))
			if hostKey == "" {
				continue
			}
			if strings.TrimSpace(r.Host) == "" {
				r.Host = firstNonEmpty(j.Device.Host, r.DeviceName, j.TargetLabel)
			}
			if strings.TrimSpace(r.DeviceName) == "" {
				r.DeviceName = firstNonEmpty(j.Device.Name, j.TargetLabel, r.Host)
			}
			if strings.TrimSpace(r.Vendor) == "" {
				r.Vendor = j.Device.Vendor
			}
			current := out[hostKey]
			if current == nil || r.StartedAt.After(current.StartedAt) {
				runCopy := r
				out[hostKey] = &runCopy
			}
		}
	}
	return out, nil
}

func backupCompareDeviceFromRun(baseRun, compareRun *models.WorkflowRun, added, removed []BackupCompareLine) BackupCompareDevice {
	source := compareRun
	if source == nil {
		source = baseRun
	}
	item := BackupCompareDevice{}
	if source != nil {
		item.Host = source.Host
		item.DeviceName = source.DeviceName
		item.Vendor = source.Vendor
	}
	if baseRun != nil {
		baseStarted := baseRun.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
		item.BaseRunID = baseRun.ID
		item.BaseStartedAt = &baseStarted
		item.BaseSize = len(baseRun.Output)
	}
	if compareRun != nil {
		compareStarted := compareRun.StartedAt.UTC().Format("2006-01-02T15:04:05Z")
		item.CompareRunID = compareRun.ID
		item.CompareStartedAt = &compareStarted
		item.CompareSize = len(compareRun.Output)
	}
	item.AddedLines = added
	item.RemovedLines = removed
	return item
}

func compareBackupLines(baseOutput, compareOutput string) ([]BackupCompareLine, []BackupCompareLine) {
	baseCounts := backupLineCounts(baseOutput)
	compareCounts := backupLineCounts(compareOutput)
	added := make([]BackupCompareLine, 0)
	removed := make([]BackupCompareLine, 0)
	for line, compareCount := range compareCounts {
		if diff := compareCount - baseCounts[line]; diff > 0 {
			added = append(added, BackupCompareLine{Text: line, Count: diff})
		}
	}
	for line, baseCount := range baseCounts {
		if diff := baseCount - compareCounts[line]; diff > 0 {
			removed = append(removed, BackupCompareLine{Text: line, Count: diff})
		}
	}
	sort.Slice(added, func(i, j int) bool { return added[i].Text < added[j].Text })
	sort.Slice(removed, func(i, j int) bool { return removed[i].Text < removed[j].Text })
	const maxLines = 200
	if len(added) > maxLines {
		added = added[:maxLines]
	}
	if len(removed) > maxLines {
		removed = removed[:maxLines]
	}
	return added, removed
}

func backupLineCounts(output string) map[string]int {
	counts := make(map[string]int)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if shouldIgnoreBackupCompareLine(line) {
			continue
		}
		counts[line]++
	}
	return counts
}

func shouldIgnoreBackupCompareLine(line string) bool {
	if line == "" {
		return true
	}
	lower := strings.ToLower(line)
	ignorePrefixes := []string{
		"!time:",
		"# time:",
		"# ",
		"! last configuration change at ",
		"! nvram config last updated at ",
		"! no configuration change since last restart",
		"current configuration : ",
		"ntp clock-period ",
		"transport input ss",
	}
	for _, prefix := range ignorePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
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

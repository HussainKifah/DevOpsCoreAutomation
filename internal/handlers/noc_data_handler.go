package handlers

import (
	"encoding/csv"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/nocdata"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NocDataRunner interface {
	RunAllNow()
	CollectDeviceNow(id uint)
	RecoverFailedDeviceNow(id uint)
}

type NocDataHandler struct {
	repo   repository.NocDataRepository
	key    []byte
	runner NocDataRunner
	cfg    *config.Config
}

func NewNocDataHandler(repo repository.NocDataRepository, key []byte, runner NocDataRunner, cfg *config.Config) *NocDataHandler {
	return &NocDataHandler{repo: repo, key: key, runner: runner, cfg: cfg}
}

type nocDataDeviceDTO struct {
	ID              uint    `json:"id"`
	DisplayName     string  `json:"display_name"`
	Site            string  `json:"site"`
	Subnet          string  `json:"subnet"`
	DeviceRange     string  `json:"range"`
	Host            string  `json:"ip"`
	Vendor          string  `json:"vendor"`
	Method          string  `json:"method"`
	Profile         string  `json:"profile"`
	Status          string  `json:"status"`
	Hostname        string  `json:"hostname"`
	Model           string  `json:"model"`
	Version         string  `json:"version"`
	Serial          string  `json:"serial"`
	Uptime          string  `json:"uptime"`
	IFUp            int     `json:"if_up"`
	IFDown          int     `json:"if_down"`
	DefaultRouter   bool    `json:"default_router"`
	LayerMode       string  `json:"layer_mode"`
	UserCount       int     `json:"user_count"`
	Users           string  `json:"users"`
	SSHEnabled      bool    `json:"ssh"`
	TelnetEnabled   bool    `json:"telnet"`
	SNMPEnabled     bool    `json:"snmp"`
	NTPEnabled      bool    `json:"ntp"`
	AAAEnabled      bool    `json:"aaa"`
	SyslogEnabled   bool    `json:"syslog"`
	LastError       string  `json:"error"`
	LastCollectedAt *string `json:"last_collected_at,omitempty"`
}

type nocDataCredentialDTO struct {
	ID           uint   `json:"id"`
	VendorFamily string `json:"vendor_family"`
	Username     string `json:"username"`
}

type nocDataExclusionDTO struct {
	ID     uint   `json:"id"`
	Subnet string `json:"subnet"`
	Target string `json:"target"`
}

func toNocDataDTO(d *models.NocDataDevice, key []byte) nocDataDeviceDTO {
	var last *string
	if d.LastCollectedAt != nil {
		s := d.LastCollectedAt.Format(timeJSONLayout)
		last = &s
	}

	profile := ""
	if len(d.EncUsername) > 0 {
		if user, err := crypto.Decrypt(key, d.EncUsername); err == nil {
			profile = strings.TrimSpace(user)
		} else {
			log.Printf("[noc-data] decrypt device username id=%d host=%s: %v", d.ID, d.Host, err)
		}
	}
	return nocDataDeviceDTO{
		ID:              d.ID,
		DisplayName:     d.DisplayName,
		Site:            d.Site,
		Subnet:          d.Subnet,
		DeviceRange:     formatNocDataRange(d.Subnet, d.DeviceRange),
		Host:            d.Host,
		Vendor:          normalizeNocDataDeviceVendor(d.Vendor, d.DeviceModel),
		Method:          d.AccessMethod,
		Profile:         profile,
		Status:          d.LastStatus,
		Hostname:        d.Hostname,
		Model:           d.DeviceModel,
		Version:         d.Version,
		Serial:          d.Serial,
		Uptime:          d.Uptime,
		IFUp:            d.IFUp,
		IFDown:          d.IFDown,
		DefaultRouter:   d.DefaultRouter,
		LayerMode:       d.LayerMode,
		UserCount:       d.UserCount,
		Users:           d.Users,
		SSHEnabled:      d.SSHEnabled,
		TelnetEnabled:   d.TelnetEnabled,
		SNMPEnabled:     d.SNMPEnabled,
		NTPEnabled:      d.NTPEnabled,
		AAAEnabled:      d.AAAEnabled,
		SyslogEnabled:   d.SyslogEnabled,
		LastError:       d.LastError,
		LastCollectedAt: last,
	}
}

func toNocDataHistoryDTO(d *models.NocDataHistory) nocDataDeviceDTO {
	var last *string
	if d.LastCollectedAt != nil {
		s := d.LastCollectedAt.Format(timeJSONLayout)
		last = &s
	}
	return nocDataDeviceDTO{
		ID:              d.DeviceID,
		DisplayName:     d.DisplayName,
		Site:            d.Site,
		Subnet:          d.Subnet,
		DeviceRange:     formatNocDataRange(d.Subnet, d.DeviceRange),
		Host:            d.Host,
		Vendor:          normalizeNocDataDeviceVendor(d.Vendor, d.DeviceModel),
		Method:          d.AccessMethod,
		Status:          d.LastStatus,
		Hostname:        d.Hostname,
		Model:           d.DeviceModel,
		Version:         d.Version,
		Serial:          d.Serial,
		Uptime:          d.Uptime,
		IFUp:            d.IFUp,
		IFDown:          d.IFDown,
		DefaultRouter:   d.DefaultRouter,
		LayerMode:       d.LayerMode,
		UserCount:       d.UserCount,
		Users:           d.Users,
		SSHEnabled:      d.SSHEnabled,
		TelnetEnabled:   d.TelnetEnabled,
		SNMPEnabled:     d.SNMPEnabled,
		NTPEnabled:      d.NTPEnabled,
		AAAEnabled:      d.AAAEnabled,
		SyslogEnabled:   d.SyslogEnabled,
		LastError:       d.LastError,
		LastCollectedAt: last,
	}
}

func normalizeNocDataDeviceVendor(vendor, model string) string {
	normalizedVendor := strings.ToLower(strings.TrimSpace(vendor))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if (normalizedVendor == "cisco_ios" || normalizedVendor == "cisco_nexus") &&
		(strings.Contains(normalizedModel, "nexus") || strings.Contains(normalizedModel, "n9k") || strings.Contains(normalizedModel, "nexus9000")) {
		return "cisco_nexus"
	}
	return normalizedVendor
}

const timeJSONLayout = "2006-01-02T15:04:05Z07:00"

func (h *NocDataHandler) ListDevices(c *gin.Context) {
	list, err := h.repo.List(c.Query("q"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocDataDeviceDTO, 0, len(list))
	for i := range list {
		out = append(out, toNocDataDTO(&list[i], h.key))
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

func (h *NocDataHandler) ListHistoryDates(c *gin.Context) {
	dates, err := h.repo.ListHistoryDates(90)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]string, 0, len(dates))
	for _, d := range dates {
		out = append(out, d.Format("2006-01-02"))
	}
	c.JSON(http.StatusOK, gin.H{"dates": out})
}

func (h *NocDataHandler) ListHistory(c *gin.Context) {
	rawDate := strings.TrimSpace(c.Query("date"))
	if rawDate == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date is required"})
		return
	}
	day, err := time.Parse("2006-01-02", rawDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
		return
	}
	list, err := h.repo.ListHistoryByDate(day)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocDataDeviceDTO, 0, len(list))
	runAt := ""
	for i := range list {
		if runAt == "" {
			runAt = list[i].RunAt.Format(timeJSONLayout)
		}
		out = append(out, toNocDataHistoryDTO(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"date": rawDate, "run_at": runAt, "devices": out})
}

func (h *NocDataHandler) CreateDevice(c *gin.Context) {
	var req struct {
		Site        string `json:"site" binding:"required"`
		Subnet      string `json:"subnet" binding:"required"`
		DeviceRange string `json:"range" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	scanCount, err := nocdata.CountIPv4Range(req.Subnet, req.DeviceRange)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	site := strings.TrimSpace(req.Site)
	subnet := strings.TrimSpace(req.Subnet)
	deviceRange := strings.TrimSpace(req.DeviceRange)
	go h.discoverAndCreateDevices(site, subnet, deviceRange)

	c.JSON(http.StatusAccepted, gin.H{
		"ok":            true,
		"queued":        true,
		"scanned_count": scanCount,
		"message":       "Ping scan started. Reachable IPs will be added automatically as they are found.",
	})
}

func (h *NocDataHandler) discoverAndCreateDevices(site, subnet, deviceRange string) {
	exclusions, err := h.repo.ListExclusions()
	if err != nil {
		log.Printf("[noc-data] list exclusions for site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, err)
		return
	}

	encUser, err := crypto.Encrypt(h.key, "")
	if err != nil {
		log.Printf("[noc-data] encrypt username for site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, err)
		return
	}
	encPass, err := crypto.Encrypt(h.key, "")
	if err != nil {
		log.Printf("[noc-data] encrypt password for site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, err)
		return
	}

	const pingWorkers = 1000
	const collectWorkers = 3

	hostJobs := make(chan string, pingWorkers*2)
	collectJobs := make(chan uint, collectWorkers*2)

	var scanned uint64
	var eligible uint64
	var reachable uint64
	var discoveredMu sync.Mutex
	discoveredIDs := make([]uint, 0)

	var collectWG sync.WaitGroup
	if h.runner != nil {
		for i := 0; i < collectWorkers; i++ {
			collectWG.Add(1)
			go func() {
				defer collectWG.Done()
				for id := range collectJobs {
					h.runner.CollectDeviceNow(id)
				}
			}()
		}
	}

	var pingWG sync.WaitGroup
	for i := 0; i < pingWorkers; i++ {
		pingWG.Add(1)
		go func() {
			defer pingWG.Done()
			for host := range hostJobs {
				atomic.AddUint64(&scanned, 1)

				excluded, excludeErr := isNocDataHostExcluded(host, exclusions)
				if excludeErr != nil {
					log.Printf("[noc-data] exclusion check host=%s subnet=%s range=%s: %v", host, subnet, deviceRange, excludeErr)
					continue
				}
				if excluded {
					continue
				}
				atomic.AddUint64(&eligible, 1)

				if err := shell.PingNocDataHost(host, time.Second); err != nil {
					continue
				}
				atomic.AddUint64(&reachable, 1)

				id, err := h.upsertDiscoveredDevice(site, subnet, deviceRange, host, encUser, encPass)
				if err != nil {
					log.Printf("[noc-data] upsert device host=%s site=%s: %v", host, site, err)
					continue
				}
				discoveredMu.Lock()
				discoveredIDs = append(discoveredIDs, id)
				discoveredMu.Unlock()
				if h.runner != nil {
					collectJobs <- id
				}
			}
		}()
	}

	walkErr := nocdata.WalkIPv4Range(subnet, deviceRange, func(host string) error {
		hostJobs <- host
		return nil
	})
	close(hostJobs)
	pingWG.Wait()
	close(collectJobs)
	collectWG.Wait()

	if walkErr != nil {
		log.Printf("[noc-data] range discovery aborted for site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, walkErr)
		return
	}
	if atomic.LoadUint64(&reachable) == 0 {
		log.Printf("[noc-data] no reachable IPs found for site=%s subnet=%s range=%s", site, subnet, deviceRange)
		return
	}

	if h.runner != nil {
		discoveredMu.Lock()
		retryIDs := append([]uint(nil), discoveredIDs...)
		discoveredMu.Unlock()
		h.recoverFailedRangeDevicesOneByOne(site, subnet, deviceRange, retryIDs)
	}

	log.Printf(
		"[noc-data] range discovery completed for site=%s subnet=%s range=%s reachable=%d scanned=%d eligible=%d",
		site,
		subnet,
		deviceRange,
		atomic.LoadUint64(&reachable),
		atomic.LoadUint64(&scanned),
		atomic.LoadUint64(&eligible),
	)
}

func (h *NocDataHandler) recoverFailedRangeDevicesOneByOne(site, subnet, deviceRange string, ids []uint) {
	if h.runner == nil || len(ids) == 0 {
		return
	}
	targets := make([]uint, 0)
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		device, err := h.repo.GetByID(id)
		if err != nil {
			log.Printf("[noc-data] auto failed recovery get id=%d site=%s subnet=%s range=%s: %v", id, site, subnet, deviceRange, err)
			continue
		}
		if strings.ToLower(strings.TrimSpace(device.LastStatus)) != "fail" {
			continue
		}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		log.Printf("[noc-data] auto failed recovery skipped for site=%s subnet=%s range=%s: no failed devices after first pass", site, subnet, deviceRange)
		return
	}

	encUser, err := crypto.Encrypt(h.key, "")
	if err != nil {
		log.Printf("[noc-data] auto failed recovery encrypt username site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, err)
		return
	}
	encPass, err := crypto.Encrypt(h.key, "")
	if err != nil {
		log.Printf("[noc-data] auto failed recovery encrypt password site=%s subnet=%s range=%s: %v", site, subnet, deviceRange, err)
		return
	}

	log.Printf("[noc-data] auto failed recovery started for site=%s subnet=%s range=%s failed_count=%d", site, subnet, deviceRange, len(targets))
	for _, id := range targets {
		if err := h.resetNocDataDeviceForRecovery(id, encUser, encPass); err != nil {
			log.Printf("[noc-data] auto failed recovery reset id=%d site=%s subnet=%s range=%s: %v", id, site, subnet, deviceRange, err)
			continue
		}
		h.runner.RecoverFailedDeviceNow(id)
	}
	log.Printf("[noc-data] auto failed recovery completed for site=%s subnet=%s range=%s failed_count=%d", site, subnet, deviceRange, len(targets))
}

func (h *NocDataHandler) upsertDiscoveredDevice(site, subnet, deviceRange, host string, encUser, encPass []byte) (uint, error) {
	existing, err := h.repo.FindByRangeHost(site, subnet, deviceRange, host)
	if err == nil {
		updates := map[string]interface{}{
			"display_name":  site + " " + host,
			"enc_username":  encUser,
			"enc_password":  encPass,
			"vendor":        "pending",
			"access_method": "pending",
			"last_status":   "pending",
			"last_error":    "",
		}
		if err := h.repo.UpdateDevice(existing.ID, updates); err != nil {
			return 0, err
		}
		return existing.ID, nil
	}
	if err != nil && err != gorm.ErrRecordNotFound {
		return 0, err
	}

	d := &models.NocDataDevice{
		DisplayName:  site + " " + host,
		Site:         site,
		Subnet:       subnet,
		DeviceRange:  deviceRange,
		Host:         host,
		Vendor:       "pending",
		AccessMethod: "pending",
		EncUsername:  encUser,
		EncPassword:  encPass,
		LastStatus:   "pending",
	}
	if err := h.repo.Create(d); err != nil {
		return 0, err
	}
	return d.ID, nil
}

func (h *NocDataHandler) DeleteDevice(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.HardDelete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NocDataHandler) RunAll(c *gin.Context) {
	if h.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "collector runner is unavailable"})
		return
	}
	go h.runner.RunAllNow()
	c.JSON(http.StatusAccepted, gin.H{
		"ok":      true,
		"queued":  true,
		"message": "Full NOC Data run started. Failed devices will be retried one by one after the full run completes.",
	})
}

func (h *NocDataHandler) RetryRange(c *gin.Context) {
	var req struct {
		Site   string `json:"site" binding:"required"`
		Subnet string `json:"subnet" binding:"required"`
		Range  string `json:"range" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if h.runner == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "collector runner is unavailable"})
		return
	}

	list, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	site := strings.TrimSpace(req.Site)
	subnet := strings.TrimSpace(req.Subnet)
	deviceRange := strings.TrimSpace(req.Range)
	encUser, err := crypto.Encrypt(h.key, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	encPass, err := crypto.Encrypt(h.key, "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ids := make([]uint, 0)
	for i := range list {
		d := &list[i]
		formattedRange := formatNocDataRange(d.Subnet, d.DeviceRange)
		if strings.TrimSpace(d.Site) != site {
			continue
		}
		if strings.TrimSpace(d.Subnet) != subnet {
			continue
		}
		if strings.TrimSpace(d.DeviceRange) != deviceRange && formattedRange != deviceRange {
			continue
		}
		ids = append(ids, d.ID)
	}

	if len(ids) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no devices found for this range"})
		return
	}

	for _, id := range ids {
		if err := h.repo.UpdateDevice(id, map[string]interface{}{
			"vendor":        "pending",
			"access_method": "pending",
			"enc_username":  encUser,
			"enc_password":  encPass,
			"last_status":   "pending",
			"last_error":    "",
		}); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	go func(deviceIDs []uint) {
		var wg sync.WaitGroup
		for _, id := range deviceIDs {
			wg.Add(1)
			go func(deviceID uint) {
				defer wg.Done()
				h.runner.CollectDeviceNow(deviceID)
			}(id)
		}
		wg.Wait()
	}(append([]uint(nil), ids...))

	c.JSON(http.StatusAccepted, gin.H{
		"ok":           true,
		"queued":       true,
		"device_count": len(ids),
		"message":      "Range retry started.",
	})
}

func (h *NocDataHandler) ListCredentials(c *gin.Context) {
	list, err := h.repo.ListCredentials(c.Query("vendor"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocDataCredentialDTO, 0, len(list))
	for _, item := range list {
		user, decErr := crypto.Decrypt(h.key, item.EncUsername)
		if decErr != nil {
			log.Printf("[noc-data] decrypt username credential id=%d: %v", item.ID, decErr)
			continue
		}
		out = append(out, nocDataCredentialDTO{
			ID:           item.ID,
			VendorFamily: item.VendorFamily,
			Username:     strings.TrimSpace(user),
		})
	}
	c.JSON(http.StatusOK, gin.H{"credentials": out})
}

func (h *NocDataHandler) CreateCredential(c *gin.Context) {
	var req struct {
		VendorFamily string `json:"vendor_family" binding:"required"`
		Username     string `json:"username" binding:"required"`
		Password     string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	family := normalizeCredentialVendorFamily(req.VendorFamily)
	if family == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vendor_family must be cisco, huawei, or mikrotik"})
		return
	}

	encUser, err := crypto.Encrypt(h.key, strings.TrimSpace(req.Username))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt username"})
		return
	}
	encPass, err := crypto.Encrypt(h.key, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password"})
		return
	}

	item := &models.NocDataCredential{
		VendorFamily: family,
		EncUsername:  encUser,
		EncPassword:  encPass,
		Enabled:      true,
	}
	if err := h.repo.CreateCredential(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"credential": nocDataCredentialDTO{
		ID:           item.ID,
		VendorFamily: family,
		Username:     strings.TrimSpace(req.Username),
	}})
}

func (h *NocDataHandler) DeleteCredential(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteCredential(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NocDataHandler) ListExclusions(c *gin.Context) {
	list, err := h.repo.ListExclusions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocDataExclusionDTO, 0, len(list))
	for _, item := range list {
		out = append(out, nocDataExclusionDTO{
			ID:     item.ID,
			Subnet: item.Subnet,
			Target: item.Target,
		})
	}
	c.JSON(http.StatusOK, gin.H{"exclusions": out})
}

func (h *NocDataHandler) CreateExclusion(c *gin.Context) {
	var req struct {
		Subnet string `json:"subnet" binding:"required"`
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	target := normalizeExclusionTarget(req.Target)
	if _, err := nocdata.CountIPv4Range(strings.TrimSpace(req.Subnet), target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := &models.NocDataExclusion{
		Subnet: strings.TrimSpace(req.Subnet),
		Target: target,
	}
	if err := h.repo.CreateExclusion(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"exclusion": nocDataExclusionDTO{
		ID:     item.ID,
		Subnet: item.Subnet,
		Target: item.Target,
	}})
}

func (h *NocDataHandler) DeleteExclusion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteExclusion(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NocDataHandler) ExportCSV(c *gin.Context) {
	list, err := h.repo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="noc-data.csv"`)
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"site", "subnet", "range", "ip", "vendor", "method", "profile", "status", "hostname", "model", "version", "serial", "uptime", "IF_UP", "IF_DOWN", "default_router", "layer_mode", "user_count", "users", "ssh", "telnet", "snmp", "ntp", "aaa", "syslog", "error"})
	for i := range list {
		d := &list[i]
		profile := ""
		if len(d.EncUsername) > 0 {
			if user, err := crypto.Decrypt(h.key, d.EncUsername); err == nil {
				profile = strings.TrimSpace(user)
			}
		}
		_ = w.Write([]string{
			d.Site,
			d.Subnet,
			d.DeviceRange,
			d.Host,
			d.Vendor,
			d.AccessMethod,
			profile,
			d.LastStatus,
			d.Hostname,
			d.DeviceModel,
			d.Version,
			d.Serial,
			d.Uptime,
			strconv.Itoa(d.IFUp),
			strconv.Itoa(d.IFDown),
			boolString(d.DefaultRouter),
			d.LayerMode,
			strconv.Itoa(d.UserCount),
			d.Users,
			boolString(d.SSHEnabled),
			boolString(d.TelnetEnabled),
			boolString(d.SNMPEnabled),
			boolString(d.NTPEnabled),
			boolString(d.AAAEnabled),
			boolString(d.SyslogEnabled),
			d.LastError,
		})
	}
	w.Flush()
}

func boolString(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func formatNocDataRange(subnet, rawRange string) string {
	if strings.TrimSpace(subnet) == "" || strings.TrimSpace(rawRange) == "" {
		return rawRange
	}
	formatted, err := nocdata.FormatIPv4Range(subnet, rawRange)
	if err != nil {
		return rawRange
	}
	return formatted
}

func normalizeCredentialVendorFamily(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "cisco", "cisco_ios", "cisco_nexus":
		return "cisco"
	case "huawei":
		return "huawei"
	case "mikrotik":
		return "mikrotik"
	default:
		return ""
	}
}

func normalizeExclusionTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "-") {
		return target
	}
	return target + "-" + target
}

func isNocDataHostExcluded(host string, exclusions []models.NocDataExclusion) (bool, error) {
	for _, item := range exclusions {
		match, err := nocdata.HostMatchesIPv4Spec(item.Subnet, item.Target, host)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

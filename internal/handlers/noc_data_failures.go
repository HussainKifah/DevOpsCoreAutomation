package handlers

import (
	"net/http"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/gin-gonic/gin"
)

func (h *NocDataHandler) RunFailedDevicesOneByOne(c *gin.Context) {
	h.runFailedDevicesOneByOneWithID(c, 0)
}

func (h *NocDataHandler) RunFailedDevice(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	h.runFailedDevicesOneByOneWithID(c, id)
}

func (h *NocDataHandler) runFailedDevicesOneByOneWithID(c *gin.Context, forcedID uint) {
	var req struct {
		ID uint `json:"id"`
	}
	if forcedID == 0 {
		if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	} else {
		req.ID = forcedID
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

	targets := make([]uint, 0)
	for i := range list {
		device := &list[i]
		if req.ID != 0 && device.ID != req.ID {
			continue
		}
		if strings.ToLower(strings.TrimSpace(device.LastStatus)) != "fail" {
			continue
		}
		targets = append(targets, device.ID)
	}

	if len(targets) == 0 {
		if req.ID != 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "failed device not found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "no failed devices found"})
		return
	}

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

	for _, id := range targets {
		if err := h.resetNocDataDeviceForRecovery(id, encUser, encPass); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	go func(ids []uint) {
		for _, id := range ids {
			h.runner.RecoverFailedDeviceNow(id)
		}
	}(append([]uint(nil), targets...))

	message := "Failed IP recovery started one by one with fresh vendor/login detection."
	if req.ID != 0 {
		message = "Failed IP recovery started for this device."
	}

	c.JSON(http.StatusAccepted, gin.H{
		"ok":           true,
		"queued":       true,
		"device_count": len(targets),
		"message":      message,
	})
}

func (h *NocDataHandler) resetNocDataDeviceForRecovery(id uint, encUser, encPass []byte) error {
	return h.repo.UpdateDevice(id, map[string]interface{}{
		"vendor":            "pending",
		"access_method":     "pending",
		"enc_username":      encUser,
		"enc_password":      encPass,
		"last_status":       "pending",
		"last_error":        "",
		"hostname":          "",
		"device_model":      "",
		"version":           "",
		"serial":            "",
		"uptime":            "",
		"if_up":             0,
		"if_down":           0,
		"default_router":    false,
		"layer_mode":        "",
		"user_count":        0,
		"users":             "",
		"ssh_enabled":       false,
		"telnet_enabled":    false,
		"snmp_enabled":      false,
		"ntp_enabled":       false,
		"aaa_enabled":       false,
		"syslog_enabled":    false,
		"last_collected_at": nil,
	})
}

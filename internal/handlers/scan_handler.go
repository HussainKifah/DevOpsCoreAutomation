package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Scanner interface {
	RunHealthScan() bool
	RunPowerScan() bool
	RunPortScan() bool
	RunInventoryScan() bool
}

type ScanHandler struct {
	scanner Scanner
}

func NewScanHandler(s Scanner) *ScanHandler {
	return &ScanHandler{scanner: s}
}

func (h *ScanHandler) RunHealth(c *gin.Context) {
	if !h.scanner.RunHealthScan() {
		c.JSON(http.StatusConflict, gin.H{"error": "another scan is already running"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "health scan started"})
}

func (h *ScanHandler) RunPower(c *gin.Context) {
	if !h.scanner.RunPowerScan() {
		c.JSON(http.StatusConflict, gin.H{"error": "another scan is already running"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "power scan started"})
}

func (h *ScanHandler) RunPorts(c *gin.Context) {
	if !h.scanner.RunPortScan() {
		c.JSON(http.StatusConflict, gin.H{"error": "another scan is already running"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "port scan started"})
}

func (h *ScanHandler) RunInventory(c *gin.Context) {
	if !h.scanner.RunInventoryScan() {
		c.JSON(http.StatusConflict, gin.H{"error": "another scan is already running"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "inventory scan started"})
}

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Scanner interface {
	RunHealthScan()
	RunPowerScan()
	RunPortScan()
	RunInventoryScan()
	RunHuaweiHealthScan()
	RunHuaweiPowerScan()
	RunHuaweiPortScan()
	RunHuaweiBackup()
	RunHuaweiInventoryScan()
}

type ScanHandler struct {
	scanner Scanner
}

func NewScanHandler(s Scanner) *ScanHandler {
	return &ScanHandler{scanner: s}
}

func (h *ScanHandler) RunHealth(c *gin.Context) {
	h.scanner.RunHealthScan()
	h.scanner.RunHuaweiHealthScan()
	c.JSON(http.StatusOK, gin.H{"status": "health scan started (nokia + huawei)"})
}

func (h *ScanHandler) RunPower(c *gin.Context) {
	h.scanner.RunPowerScan()
	h.scanner.RunHuaweiPowerScan()
	c.JSON(http.StatusOK, gin.H{"status": "power scan started (nokia + huawei)"})
}

func (h *ScanHandler) RunPorts(c *gin.Context) {
	h.scanner.RunPortScan()
	h.scanner.RunHuaweiPortScan()
	c.JSON(http.StatusOK, gin.H{"status": "port scan started (nokia + huawei)"})
}

func (h *ScanHandler) RunInventory(c *gin.Context) {
	h.scanner.RunInventoryScan()
	h.scanner.RunHuaweiInventoryScan()
	c.JSON(http.StatusOK, gin.H{"status": "inventory scan started (nokia + huawei)"})
}

func (h *ScanHandler) RunBackup(c *gin.Context) {
	h.scanner.RunHuaweiBackup()
	c.JSON(http.StatusOK, gin.H{"status": "huawei backup started"})
}

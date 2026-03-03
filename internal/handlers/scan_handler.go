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
}

type ScanHandler struct {
	scanner Scanner
}

func NewScanHandler(s Scanner) *ScanHandler {
	return &ScanHandler{scanner: s}
}

func (h *ScanHandler) RunHealth(c *gin.Context) {
	h.scanner.RunHealthScan()
	c.JSON(http.StatusOK, gin.H{"status": "health scan started"})
}

func (h *ScanHandler) RunPower(c *gin.Context) {
	h.scanner.RunPowerScan()
	c.JSON(http.StatusOK, gin.H{"status": "power scan started"})
}

func (h *ScanHandler) RunPorts(c *gin.Context) {
	h.scanner.RunPortScan()
	c.JSON(http.StatusOK, gin.H{"status": "port scan started"})
}

func (h *ScanHandler) RunInventory(c *gin.Context) {
	h.scanner.RunInventoryScan()
	c.JSON(http.StatusOK, gin.H{"status": "inventory scan started"})
}

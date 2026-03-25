package handlers

import (
	"net/http"
	"strconv"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type PowerHandler struct {
	PowerRepo repository.PowerRepository
}

func NewPowerHandler(powerRepo repository.PowerRepository) *PowerHandler {
	return &PowerHandler{PowerRepo: powerRepo}
}

func (h *PowerHandler) GetAll(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	device := c.Query("device")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "ont_rx")
	sortOrder := c.DefaultQuery("sort_order", "asc")
	if sortOrder != "desc" {
		sortOrder = "asc"
	}

	if page < 1 {
		page = 1
	}
	if c.Query("export") == "true" {
		perPage = 0
	} else if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	data, err := h.PowerRepo.GetPaginated(page, perPage, vendor, device, search, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetWeak(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	threshold := -24.0
	if q := c.Query("threshold"); q != "" {
		if v, err := strconv.ParseFloat(q, 64); err == nil {
			threshold = v
		}
	}
	data, err := h.PowerRepo.GetWeak(threshold, vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetSummary(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	threshold := -24.0
	if q := c.Query("threshold"); q != "" {
		if v, err := strconv.ParseFloat(q, 64); err == nil {
			threshold = v
		}
	}
	data, err := h.PowerRepo.GetSummary(threshold, vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetDevices(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	data, err := h.PowerRepo.GetDevices(vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

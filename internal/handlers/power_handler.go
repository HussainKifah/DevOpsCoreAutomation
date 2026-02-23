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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	device := c.Query("device")
	search := c.Query("search")

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 200 {
		perPage = 50
	}

	data, err := h.PowerRepo.GetPaginated(page, perPage, device, search)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetWeak(c *gin.Context) {
	threshold := -24.0
	if q := c.Query("threshold"); q != "" {
		if v, err := strconv.ParseFloat(q, 64); err == nil {
			threshold = v
		}
	}
	data, err := h.PowerRepo.GetWeak(threshold)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetSummary(c *gin.Context) {
	threshold := -24.0
	if q := c.Query("threshold"); q != "" {
		if v, err := strconv.ParseFloat(q, 64); err == nil {
			threshold = v
		}
	}
	data, err := h.PowerRepo.GetSummary(threshold)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PowerHandler) GetDevices(c *gin.Context) {
	data, err := h.PowerRepo.GetDevices()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

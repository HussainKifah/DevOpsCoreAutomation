package handlers

import (
	"net/http"
	"strconv"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type InventoryHandler struct {
	repo repository.InventoryRepository
}

func NewInventoryHandler(repo repository.InventoryRepository) *InventoryHandler {
	return &InventoryHandler{repo: repo}
}

func (h *InventoryHandler) GetLatestSummary(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	summary, err := h.repo.GetLatestSummary(vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch latest inventory summary"})
		return
	}
	c.JSON(http.StatusOK, summary)
}

func (h *InventoryHandler) GetLatestOltInventories(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	inventories, err := h.repo.GetLatestOltInventories(vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch olt inventories"})
		return
	}
	c.JSON(http.StatusOK, inventories)
}

func (h *InventoryHandler) GetOltInventoryHistory(c *gin.Context) {
	host := c.Param("host")
	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host parameter is required"})
		return
	}

	limitStr := c.Query("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	inventories, err := h.repo.GetOltInventoryHistory(host, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch olt inventory history"})
		return
	}
	c.JSON(http.StatusOK, inventories)
}

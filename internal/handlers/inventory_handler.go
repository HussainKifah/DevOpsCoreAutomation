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

func (h *InventoryHandler) GetOntInterfaces(c *gin.Context) {
	vendor := c.DefaultQuery("vendor", "nokia")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	device := c.Query("device")
	search := c.Query("search")
	sortBy := c.DefaultQuery("sort_by", "ont_idx")
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

	rows, err := h.repo.GetOntInterfacesPaginated(page, perPage, vendor, device, search, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch ONT interfaces"})
		return
	}
	c.JSON(http.StatusOK, rows)
}

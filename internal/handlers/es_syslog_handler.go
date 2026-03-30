package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type EsSyslogHandler struct {
	repo *repository.EsSyslogRepository
}

func NewEsSyslogHandler(repo *repository.EsSyslogRepository) *EsSyslogHandler {
	return &EsSyslogHandler{repo: repo}
}

// ListAlerts GET /api/ip/syslog/alerts
func (h *EsSyslogHandler) ListAlerts(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}

	enabled, err := h.repo.ListFilters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if len(enabled) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"alerts": []models.EsSyslogAlert{},
			"pagination": gin.H{
				"page": page, "limit": limit, "total": int64(0), "total_pages": int64(1),
			},
		})
		return
	}
	offset := (page - 1) * limit
	list, total, err := h.repo.ListAlerts(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pages := (total + int64(limit) - 1) / int64(limit)
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"alerts": list,
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": pages,
		},
	})
}

// ListFilters GET /api/ip/syslog/filters
func (h *EsSyslogHandler) ListFilters(c *gin.Context) {
	list, err := h.repo.ListAllFiltersForAdmin()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"filters": list})
}

type esSyslogFilterCreate struct {
	Label     string `json:"label"`
	QueryText string `json:"query_text" binding:"required"`
	SortOrder int    `json:"sort_order"`
	Enabled   *bool  `json:"enabled"`
}

// CreateFilter POST /api/ip/syslog/filters
func (h *EsSyslogHandler) CreateFilter(c *gin.Context) {
	var req esSyslogFilterCreate
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	q := strings.TrimSpace(req.QueryText)
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query_text required"})
		return
	}
	f := &models.EsSyslogFilter{
		Label:     strings.TrimSpace(req.Label),
		QueryText: q,
		SortOrder: req.SortOrder,
		Enabled:   true,
	}
	if req.Enabled != nil {
		f.Enabled = *req.Enabled
	}
	if err := h.repo.CreateFilter(f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"filter": f})
}

type esSyslogFilterPatch struct {
	Label     *string `json:"label"`
	QueryText *string `json:"query_text"`
	SortOrder *int    `json:"sort_order"`
	Enabled   *bool   `json:"enabled"`
}

// UpdateFilter PUT /api/ip/syslog/filters/:id
func (h *EsSyslogHandler) UpdateFilter(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	f, err := h.repo.GetFilter(uint(id64))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	var req esSyslogFilterPatch
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Label != nil {
		f.Label = strings.TrimSpace(*req.Label)
	}
	if req.QueryText != nil {
		q := strings.TrimSpace(*req.QueryText)
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query_text cannot be empty"})
			return
		}
		f.QueryText = q
	}
	if req.SortOrder != nil {
		f.SortOrder = *req.SortOrder
	}
	if req.Enabled != nil {
		f.Enabled = *req.Enabled
	}
	if err := h.repo.UpdateFilter(f); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"filter": f})
}

// DeleteFilter DELETE /api/ip/syslog/filters/:id
func (h *EsSyslogHandler) DeleteFilter(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteFilter(uint(id64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

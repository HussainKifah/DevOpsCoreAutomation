package handlers

import (
	"net/http"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type PortHistoryHandler struct {
	Repo repository.PortHistoryRepository
}

func NewPortHistoryHandler(r repository.PortHistoryRepository) *PortHistoryHandler {
	return &PortHistoryHandler{Repo: r}
}

func (h *PortHistoryHandler) GetHistory(c *gin.Context) {
	host := c.Query("host")
	if host == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "host parameter required"})
		return
	}

	rangeStr := c.DefaultQuery("range", "24h")
	from, to := parseTimeRange(rangeStr)

	data, err := h.Repo.GetByHostAndRange(host, from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *PortHistoryHandler) GetDownCounts(c *gin.Context) {
	rangeStr := c.DefaultQuery("range", "7d")
	from, to := parseTimeRange(rangeStr)

	data, err := h.Repo.GetDownCountByRange(from, to)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, data)
}

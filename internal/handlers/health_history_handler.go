package handlers

import (
	"net/http"
	"time"

	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type HealthHistoryHandler struct {
	Repo repository.HealthHistoryRepository
}

func NewHealthHistoryHandler(r repository.HealthHistoryRepository) *HealthHistoryHandler {
	return &HealthHistoryHandler{Repo: r}
}

func (h *HealthHistoryHandler) GetHistory(c *gin.Context) {
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

func parseTimeRange(r string) (time.Time, time.Time) {
	now := time.Now()
	switch r {
	case "7d":
		return now.AddDate(0, 0, -7), now
	case "30d":
		return now.AddDate(0, -1, 0), now
	default:
		return now.Add(-24 * time.Hour), now
	}
}

package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type BetterStackHandler struct {
	repo *repository.BetterStackRepository
	cfg  *config.Config
}

func NewBetterStackHandler(repo *repository.BetterStackRepository, cfg *config.Config) *BetterStackHandler {
	return &BetterStackHandler{repo: repo, cfg: cfg}
}

func (h *BetterStackHandler) ListIncidents(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 30
	}
	state := strings.ToLower(strings.TrimSpace(c.DefaultQuery("state", "unresolved")))
	if state != "resolved" && state != "unresolved" {
		state = "unresolved"
	}

	channelID := ""
	if h.cfg != nil {
		channelID = strings.TrimSpace(h.cfg.BetterStackSlackChannelID)
	}
	offset := (page - 1) * limit
	list, total, err := h.repo.ListSlackIncidents(channelID, state, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	pages := (total + int64(limit) - 1) / int64(limit)
	if pages == 0 {
		pages = 1
	}
	c.JSON(http.StatusOK, gin.H{
		"incidents": betterStackIncidentDTOs(list),
		"pagination": gin.H{
			"page":        page,
			"limit":       limit,
			"total":       total,
			"total_pages": pages,
		},
	})
}

func betterStackIncidentDTOs(list []models.BetterStackSlackIncident) []gin.H {
	out := make([]gin.H, 0, len(list))
	for i := range list {
		row := list[i]
		out = append(out, gin.H{
			"id":                       row.ID,
			"better_stack_incident_id": row.BetterStackIncidentID,
			"channel_id":               row.ChannelID,
			"message_ts":               row.MessageTS,
			"name":                     row.Name,
			"cause":                    row.Cause,
			"url":                      row.URL,
			"origin_url":               row.OriginURL,
			"status":                   row.Status,
			"team_name":                row.TeamName,
			"resolved_by":              row.ResolvedBy,
			"started_at_utc":           timeOrNil(row.StartedAtUTC),
			"acknowledged_at_utc":      row.AcknowledgedAtUTC,
			"resolved_at_utc":          row.ResolvedAtUTC,
			"last_seen_at_utc":         timeOrNil(row.LastSeenAtUTC),
			"next_reminder_at":         timeOrNil(row.NextReminderAt),
		})
	}
	return out
}

func timeOrNil(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

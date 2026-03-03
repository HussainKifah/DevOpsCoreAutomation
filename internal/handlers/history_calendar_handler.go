package handlers

import (
	"net/http"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type HistoryCalendarHandler struct {
	HealthRepo repository.HealthHistoryRepository
	PortRepo   repository.PortHistoryRepository
}

func NewHistoryCalendarHandler(hr repository.HealthHistoryRepository, pr repository.PortHistoryRepository) *HistoryCalendarHandler {
	return &HistoryCalendarHandler{HealthRepo: hr, PortRepo: pr}
}

func (h *HistoryCalendarHandler) GetCalendar(c *gin.Context) {
	monthStr := c.DefaultQuery("month", time.Now().Format("2006-01"))
	t, err := time.Parse("2006-01", monthStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month format, use YYYY-MM"})
		return
	}
	from := t
	to := t.AddDate(0, 1, 0)

	var (
		healthDays []repository.HealthCalendarDay
		portDays   []repository.PortCalendarDay
		hErr, pErr error
		wg         sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		healthDays, hErr = h.HealthRepo.GetCalendarDays(from, to)
	}()
	go func() {
		defer wg.Done()
		portDays, pErr = h.PortRepo.GetCalendarDays(from, to)
	}()
	wg.Wait()

	if hErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": hErr.Error()})
		return
	}
	if pErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"month":       monthStr,
		"health_days": healthDays,
		"port_days":   portDays,
	})
}

func (h *HistoryCalendarHandler) GetDayDetail(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date parameter required (YYYY-MM-DD)"})
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format"})
		return
	}

	var (
		healthEntries []models.HealthSnapshot
		portEntries   []models.PortSnapshot
		hErr, pErr    error
		wg            sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		healthEntries, hErr = h.HealthRepo.GetSnapshotsForDate(date)
	}()
	go func() {
		defer wg.Done()
		portEntries, pErr = h.PortRepo.GetSnapshotsForDate(date)
	}()
	wg.Wait()

	if hErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": hErr.Error()})
		return
	}
	if pErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": pErr.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"date":   dateStr,
		"health": healthEntries,
		"ports":  portEntries,
	})
}

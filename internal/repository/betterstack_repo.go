package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BetterStackRepository struct {
	db *gorm.DB
}

func NewBetterStackRepository(db *gorm.DB) *BetterStackRepository {
	return &BetterStackRepository{db: db}
}

func (r *BetterStackRepository) CreateSlackIncidentIfNew(inc *models.BetterStackSlackIncident) (bool, error) {
	tx := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(inc)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *BetterStackRepository) FindByBetterStackIncident(channelID, incidentID string) (*models.BetterStackSlackIncident, error) {
	var inc models.BetterStackSlackIncident
	err := r.db.Where("channel_id = ? AND better_stack_incident_id = ?", channelID, incidentID).First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *BetterStackRepository) ListOpenSlackIncidents() ([]models.BetterStackSlackIncident, error) {
	var list []models.BetterStackSlackIncident
	err := r.db.Where("resolved_at_utc IS NULL").Order("started_at_utc ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *BetterStackRepository) ListOpenSlackIncidentsDueReminder(until time.Time) ([]models.BetterStackSlackIncident, error) {
	var list []models.BetterStackSlackIncident
	err := r.db.Where("resolved_at_utc IS NULL AND next_reminder_at <= ?", until).
		Order("next_reminder_at ASC").Limit(50).Find(&list).Error
	return list, err
}

func (r *BetterStackRepository) ListSlackIncidents(channelID, state string, limit, offset int) ([]models.BetterStackSlackIncident, int64, error) {
	var list []models.BetterStackSlackIncident
	var total int64
	q := r.db.Model(&models.BetterStackSlackIncident{})
	if channelID != "" {
		q = q.Where("channel_id = ?", channelID)
	}
	switch state {
	case "resolved":
		q = q.Where("resolved_at_utc IS NOT NULL")
	case "unresolved":
		q = q.Where("resolved_at_utc IS NULL")
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := q.Order("COALESCE(resolved_at_utc, started_at_utc) DESC, id DESC").
		Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

func (r *BetterStackRepository) UpdateSlackIncidentSnapshot(id uint, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	return r.db.Model(&models.BetterStackSlackIncident{}).Where("id = ?", id).Updates(fields).Error
}

func (r *BetterStackRepository) BumpSlackIncidentReminder(id uint, next time.Time) error {
	return r.db.Model(&models.BetterStackSlackIncident{}).Where("id = ? AND resolved_at_utc IS NULL").
		Update("next_reminder_at", next).Error
}

func (r *BetterStackRepository) MarkSlackIncidentResolved(id uint, resolvedBy string, at time.Time) error {
	far := at.AddDate(50, 0, 0)
	return r.db.Model(&models.BetterStackSlackIncident{}).Where("id = ? AND resolved_at_utc IS NULL").
		Updates(map[string]interface{}{
			"resolved_at_utc":  at,
			"resolved_by":      resolvedBy,
			"status":           "Resolved",
			"next_reminder_at": far,
		}).Error
}

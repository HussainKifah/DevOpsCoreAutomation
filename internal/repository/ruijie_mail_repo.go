package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RuijieMailRepository struct {
	db *gorm.DB
}

func NewRuijieMailRepository(db *gorm.DB) *RuijieMailRepository {
	return &RuijieMailRepository{db: db}
}

func (r *RuijieMailRepository) InsertAlertIfNew(a *models.RuijieMailAlert) (bool, error) {
	tx := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(a)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *RuijieMailRepository) CreateSlackIncident(inc *models.RuijieSlackIncident) error {
	return r.db.Create(inc).Error
}

func (r *RuijieMailRepository) GetSlackIncidentByMessage(channelID, messageTS string) (*models.RuijieSlackIncident, error) {
	var inc models.RuijieSlackIncident
	err := r.db.Where("channel_id = ? AND message_ts = ?", channelID, messageTS).First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *RuijieMailRepository) FindOpenSlackIncidentByChannelFingerprint(channelID, fingerprint string) (*models.RuijieSlackIncident, error) {
	if fingerprint == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var inc models.RuijieSlackIncident
	err := r.db.Where("channel_id = ? AND dedup_fingerprint = ? AND resolved_at IS NULL", channelID, fingerprint).
		Order("id DESC").First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

func (r *RuijieMailRepository) ListOpenSlackIncidentsDueReminder(until time.Time) ([]models.RuijieSlackIncident, error) {
	var list []models.RuijieSlackIncident
	err := r.db.Where("resolved_at IS NULL AND next_reminder_at <= ?", until).
		Order("next_reminder_at ASC").Limit(50).Find(&list).Error
	return list, err
}

func (r *RuijieMailRepository) MarkSlackIncidentResolved(id uint, resolvedBy string, at time.Time) error {
	far := at.AddDate(50, 0, 0)
	return r.db.Model(&models.RuijieSlackIncident{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"resolved_at":      at,
			"resolved_by":      resolvedBy,
			"next_reminder_at": far,
		}).Error
}

func (r *RuijieMailRepository) BumpSlackIncidentReminder(id uint, next time.Time) error {
	return r.db.Model(&models.RuijieSlackIncident{}).Where("id = ?", id).
		Update("next_reminder_at", next).Error
}

func (r *RuijieMailRepository) LinkAlertToSlackIncident(alertID uint, incidentID uint) error {
	if alertID == 0 {
		return nil
	}
	return r.db.Model(&models.RuijieMailAlert{}).Where("id = ?", alertID).
		Update("slack_incident_id", incidentID).Error
}

func (r *RuijieMailRepository) AlertsForSlackIncident(incidentID uint) ([]models.RuijieMailAlert, error) {
	var list []models.RuijieMailAlert
	err := r.db.Where("slack_incident_id = ?", incidentID).
		Order("received_at_utc ASC, id ASC").Find(&list).Error
	return list, err
}

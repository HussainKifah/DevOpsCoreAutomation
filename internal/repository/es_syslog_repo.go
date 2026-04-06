package repository

import (
	"errors"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type EsSyslogRepository struct {
	db *gorm.DB
}

func NewEsSyslogRepository(db *gorm.DB) *EsSyslogRepository {
	return &EsSyslogRepository{db: db}
}

func (r *EsSyslogRepository) ListFilters() ([]models.EsSyslogFilter, error) {
	var list []models.EsSyslogFilter
	err := r.db.Where("enabled = ?", true).Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *EsSyslogRepository) ListAllFiltersForAdmin() ([]models.EsSyslogFilter, error) {
	var list []models.EsSyslogFilter
	err := r.db.Order("sort_order ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *EsSyslogRepository) CreateFilter(f *models.EsSyslogFilter) error {
	return r.db.Create(f).Error
}

func (r *EsSyslogRepository) GetFilter(id uint) (*models.EsSyslogFilter, error) {
	var f models.EsSyslogFilter
	err := r.db.First(&f, id).Error
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *EsSyslogRepository) UpdateFilter(f *models.EsSyslogFilter) error {
	return r.db.Save(f).Error
}

func (r *EsSyslogRepository) DeleteFilter(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("filter_id = ?", id).Delete(&models.EsSyslogAlert{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&models.EsSyslogFilter{}, id).Error
	})
}

// InsertAlertIfNew inserts one row; duplicate es_index+es_doc_id is ignored.
// inserted is true only when a new row was written (RowsAffected > 0).
func (r *EsSyslogRepository) InsertAlertIfNew(a *models.EsSyslogAlert) (inserted bool, err error) {
	tx := r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(a)
	if tx.Error != nil {
		return false, tx.Error
	}
	return tx.RowsAffected > 0, nil
}

func (r *EsSyslogRepository) CreateSlackIncident(inc *models.EsSyslogSlackIncident) error {
	return r.db.Create(inc).Error
}

func (r *EsSyslogRepository) GetSlackIncidentByMessage(channelID, messageTS string) (*models.EsSyslogSlackIncident, error) {
	var inc models.EsSyslogSlackIncident
	err := r.db.Where("channel_id = ? AND message_ts = ?", channelID, messageTS).First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// FindOpenSlackIncidentByChannelFingerprint returns the latest unresolved incident for this channel and logical alarm fingerprint.
func (r *EsSyslogRepository) FindOpenSlackIncidentByChannelFingerprint(channelID, fingerprint string) (*models.EsSyslogSlackIncident, error) {
	if fingerprint == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var inc models.EsSyslogSlackIncident
	err := r.db.Where("channel_id = ? AND dedup_fingerprint = ? AND resolved_at IS NULL", channelID, fingerprint).
		Order("id DESC").First(&inc).Error
	if err != nil {
		return nil, err
	}
	return &inc, nil
}

// ListOpenSlackIncidentsDueReminder returns unresolved incidents with NextReminderAt <= until.
func (r *EsSyslogRepository) ListOpenSlackIncidentsDueReminder(until time.Time) ([]models.EsSyslogSlackIncident, error) {
	var list []models.EsSyslogSlackIncident
	err := r.db.Where("resolved_at IS NULL AND next_reminder_at <= ?", until).
		Order("next_reminder_at ASC").Limit(50).Find(&list).Error
	return list, err
}

func (r *EsSyslogRepository) MarkSlackIncidentResolved(id uint, resolvedBy string, at time.Time) error {
	// Stop reminders: push NextReminderAt far into the future.
	far := at.AddDate(50, 0, 0)
	return r.db.Model(&models.EsSyslogSlackIncident{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"resolved_at":      at,
			"resolved_by":      resolvedBy,
			"next_reminder_at": far,
		}).Error
}

func (r *EsSyslogRepository) BumpSlackIncidentReminder(id uint, next time.Time) error {
	return r.db.Model(&models.EsSyslogSlackIncident{}).Where("id = ?", id).
		Update("next_reminder_at", next).Error
}

func (r *EsSyslogRepository) AlertsForSlackIncident(incidentID uint) ([]models.EsSyslogAlert, error) {
	var list []models.EsSyslogAlert
	err := r.db.Where("slack_incident_id = ?", incidentID).
		Order("timestamp_utc ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *EsSyslogRepository) LinkAlertsToSlackIncident(alertIDs []uint, incidentID uint) error {
	if len(alertIDs) == 0 {
		return nil
	}
	return r.db.Model(&models.EsSyslogAlert{}).Where("id IN ?", alertIDs).
		Update("slack_incident_id", incidentID).Error
}

func (r *EsSyslogRepository) ListAlerts(limit, offset int) ([]models.EsSyslogAlert, int64, error) {
	var list []models.EsSyslogAlert
	var total int64
	filterIDs := r.db.Model(&models.EsSyslogFilter{}).Select("id")
	q := r.db.Model(&models.EsSyslogAlert{}).Where("filter_id IN (?)", filterIDs)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := r.db.Where("filter_id IN (?)", filterIDs).
		Order("timestamp_utc DESC, id DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, total, err
}

// AlertDedupExists is true if we already stored an alert with this fingerprint and log time within the lookback window.
func (r *EsSyslogRepository) AlertDedupExists(fingerprint string, sinceUTC time.Time) (bool, error) {
	if fingerprint == "" {
		return false, nil
	}
	var row models.EsSyslogAlert
	err := r.db.Where("dedup_fingerprint = ? AND timestamp_utc >= ?", fingerprint, sinceUTC).
		Select("id").Limit(1).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// DeleteAlertsOlderThan hard-deletes stored alerts (not filters).
func (r *EsSyslogRepository) DeleteAlertsOlderThan(cutoff time.Time) (int64, error) {
	tx := r.db.Unscoped().Where("created_at < ?", cutoff).Delete(&models.EsSyslogAlert{})
	return tx.RowsAffected, tx.Error
}

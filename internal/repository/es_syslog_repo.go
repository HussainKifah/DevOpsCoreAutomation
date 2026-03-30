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
func (r *EsSyslogRepository) InsertAlertIfNew(a *models.EsSyslogAlert) error {
	return r.db.Clauses(clause.OnConflict{DoNothing: true}).Create(a).Error
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

package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type HealthCalendarDay struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

type HealthHourEntry struct {
	Hour   int    `json:"hour"`
	Device string `json:"device"`
	Host   string `json:"host"`
}

type HealthHistoryRepository interface {
	Insert(snapshot *models.HealthSnapshot) error
	BulkInsert(snapshots []*models.HealthSnapshot) error
	GetByHostAndRange(host string, from, to time.Time, vendor string) ([]models.HealthSnapshot, error)
	DeleteOlderThan(cutoff time.Time) (int64, error)
	GetCalendarDays(from, to time.Time, vendor string) ([]HealthCalendarDay, error)
	GetSnapshotsForDate(date time.Time, vendor string) ([]models.HealthSnapshot, error)
	GetSnapshotsForRange(from, to time.Time, vendor string) ([]models.HealthSnapshot, error)
}

type healthHistoryRepository struct {
	DB *gorm.DB
}

func NewHealthHistoryRepository(db *gorm.DB) HealthHistoryRepository {
	return &healthHistoryRepository{DB: db}
}

func (r *healthHistoryRepository) Insert(s *models.HealthSnapshot) error {
	return r.DB.Create(s).Error
}

func (r *healthHistoryRepository) BulkInsert(snapshots []*models.HealthSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	return r.DB.CreateInBatches(snapshots, 50).Error
}

func (r *healthHistoryRepository) GetByHostAndRange(host string, from, to time.Time, vendor string) ([]models.HealthSnapshot, error) {
	var out []models.HealthSnapshot
	err := r.DB.
		Where("host = ? AND measured_at >= ? AND measured_at < ? AND vendor = ?", host, from, to, vendor).
		Order("measured_at").
		Find(&out).Error
	return out, err
}

func (r *healthHistoryRepository) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.DB.Where("measured_at < ?", cutoff).Delete(&models.HealthSnapshot{})
	return result.RowsAffected, result.Error
}

func (r *healthHistoryRepository) GetCalendarDays(from, to time.Time, vendor string) ([]HealthCalendarDay, error) {
	var out []HealthCalendarDay
	err := r.DB.Model(&models.HealthSnapshot{}).
		Select("TO_CHAR(measured_at, 'YYYY-MM-DD') as date, COUNT(*) as count").
		Where("measured_at >= ? AND measured_at < ? AND vendor = ?", from, to, vendor).
		Group("TO_CHAR(measured_at, 'YYYY-MM-DD')").
		Order("date").
		Find(&out).Error
	return out, err
}

func (r *healthHistoryRepository) GetSnapshotsForDate(date time.Time, vendor string) ([]models.HealthSnapshot, error) {
	nextDay := date.AddDate(0, 0, 1)
	return r.GetSnapshotsForRange(date, nextDay, vendor)
}

func (r *healthHistoryRepository) GetSnapshotsForRange(from, to time.Time, vendor string) ([]models.HealthSnapshot, error) {
	var out []models.HealthSnapshot
	err := r.DB.
		Where("measured_at >= ? AND measured_at < ? AND vendor = ?", from, to, vendor).
		Order("measured_at, host").
		Find(&out).Error
	return out, err
}

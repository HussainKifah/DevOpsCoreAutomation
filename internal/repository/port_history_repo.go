package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type PortCalendarDay struct {
	Date      string `json:"date"`
	DownCount int64  `json:"down_count"`
}

type PortHourEntry struct {
	Hour   int    `json:"hour"`
	Device string `json:"device"`
	Host   string `json:"host"`
	Count  int64  `json:"count"`
}

type PortHistoryRepository interface {
	BulkInsert(records []models.PortSnapshot) error
	GetByHostAndRange(host string, from, to time.Time, vendor string) ([]models.PortSnapshot, error)
	GetDownCountByRange(from, to time.Time, vendor string) ([]PortDownCount, error)
	DeleteOlderThan(cutoff time.Time) (int64, error)
	GetCalendarDays(from, to time.Time, vendor string) ([]PortCalendarDay, error)
	GetSnapshotsForDate(date time.Time, vendor string) ([]models.PortSnapshot, error)
}

type PortDownCount struct {
	Date      string `json:"date"`
	Host      string `json:"host"`
	Device    string `json:"device"`
	DownCount int64  `json:"down_count"`
}

type portHistoryRepository struct {
	DB *gorm.DB
}

func NewPortHistoryRepository(db *gorm.DB) PortHistoryRepository {
	return &portHistoryRepository{DB: db}
}

func (r *portHistoryRepository) BulkInsert(records []models.PortSnapshot) error {
	if len(records) == 0 {
		return nil
	}
	return r.DB.CreateInBatches(records, 100).Error
}

func (r *portHistoryRepository) GetByHostAndRange(host string, from, to time.Time, vendor string) ([]models.PortSnapshot, error) {
	var out []models.PortSnapshot
	err := r.DB.
		Where("host = ? AND measured_at >= ? AND measured_at < ? AND vendor = ?", host, from, to, vendor).
		Order("measured_at, port").
		Find(&out).Error
	return out, err
}

func (r *portHistoryRepository) GetDownCountByRange(from, to time.Time, vendor string) ([]PortDownCount, error) {
	var out []PortDownCount
	err := r.DB.Model(&models.PortSnapshot{}).
		Select("TO_CHAR(measured_at, 'YYYY-MM-DD') as date, host, device, COUNT(DISTINCT port) as down_count").
		Where("measured_at >= ? AND measured_at < ? AND vendor = ?", from, to, vendor).
		Group("TO_CHAR(measured_at, 'YYYY-MM-DD'), host, device").
		Order("date, host").
		Find(&out).Error
	return out, err
}

func (r *portHistoryRepository) DeleteOlderThan(cutoff time.Time) (int64, error) {
	result := r.DB.Where("measured_at < ?", cutoff).Delete(&models.PortSnapshot{})
	return result.RowsAffected, result.Error
}

func (r *portHistoryRepository) GetCalendarDays(from, to time.Time, vendor string) ([]PortCalendarDay, error) {
	var out []PortCalendarDay
	err := r.DB.Model(&models.PortSnapshot{}).
		Select("TO_CHAR(measured_at, 'YYYY-MM-DD') as date, COUNT(*) as down_count").
		Where("measured_at >= ? AND measured_at < ? AND vendor = ?", from, to, vendor).
		Group("TO_CHAR(measured_at, 'YYYY-MM-DD')").
		Order("date").
		Find(&out).Error
	return out, err
}

func (r *portHistoryRepository) GetSnapshotsForDate(date time.Time, vendor string) ([]models.PortSnapshot, error) {
	nextDay := date.AddDate(0, 0, 1)
	var out []models.PortSnapshot
	err := r.DB.
		Where("measured_at >= ? AND measured_at < ? AND vendor = ?", date, nextDay, vendor).
		Order("measured_at, host, port").
		Find(&out).Error
	return out, err
}

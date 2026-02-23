package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type DeviceInfo struct {
	Device string `json:"device"`
	Site   string `json:"site"`
	Host   string `json:"host"`
}

type DevicePowerSummary struct {
	Device    string `json:"device"`
	Site      string `json:"site"`
	Total     int64  `json:"total"`
	WeakCount int64  `json:"weak_count"`
}

type PowerReadingWithDesc struct {
	ID         uint      `json:"ID"`
	Device     string    `json:"device"`
	Site       string    `json:"site"`
	Host       string    `json:"host"`
	OntIdx     string    `json:"ont_idx"`
	OltRx      float64   `json:"olt_rx"`
	MeasuredAt time.Time `json:"measured_at"`
	Desc1      string    `json:"desc1"`
	Desc2      string    `json:"desc2"`
}

type PaginatedReadings struct {
	Data       []PowerReadingWithDesc `json:"data"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PerPage    int                    `json:"per_page"`
	TotalPages int                    `json:"total_pages"`
}

type PowerRepository interface {
	BulkInsert(device, site, host string, readings []models.PowerReading) error
	DeleteByHost(host string) error
	GetAll() ([]models.PowerReading, error)
	GetPaginated(page, perPage int, device, search string) (*PaginatedReadings, error)
	GetByHost(host string) ([]models.PowerReading, error)
	GetWeak(threshold float64) ([]models.PowerReading, error)
	GetDevices() ([]DeviceInfo, error)
	GetSummary(threshold float64) ([]DevicePowerSummary, error)
}
type powerRepository struct {
	DB *gorm.DB
}

func NewPowerRepository(db *gorm.DB) PowerRepository {
	return &powerRepository{DB: db}
}

func (r *powerRepository) BulkInsert(device, site, host string, readings []models.PowerReading) error {
	now := time.Now()
	for i := range readings {
		readings[i].Device = device
		readings[i].Site = site
		readings[i].Host = host
		readings[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(readings, 100).Error
}

func (r *powerRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PowerReading{}).Error
}

func (r *powerRepository) GetAll() ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetPaginated(page, perPage int, device, search string) (*PaginatedReadings, error) {
	join := "LEFT JOIN ont_descriptions ON power_readings.ont_idx = ont_descriptions.ont_idx AND power_readings.host = ont_descriptions.host"

	countQ := r.DB.Table("power_readings").Joins(join)
	dataQ := r.DB.Table("power_readings").
		Select("power_readings.id, power_readings.device, power_readings.site, power_readings.host, power_readings.ont_idx, power_readings.olt_rx, power_readings.measured_at, COALESCE(ont_descriptions.desc1, '') as desc1, COALESCE(ont_descriptions.desc2, '') as desc2").
		Joins(join)

	if device != "" {
		countQ = countQ.Where("power_readings.device = ?", device)
		dataQ = dataQ.Where("power_readings.device = ?", device)
	}
	if search != "" {
		pattern := "%" + search + "%"
		cond := "power_readings.ont_idx ILIKE ? OR ont_descriptions.desc1 ILIKE ? OR ont_descriptions.desc2 ILIKE ?"
		countQ = countQ.Where(cond, pattern, pattern, pattern)
		dataQ = dataQ.Where(cond, pattern, pattern, pattern)
	}

	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, err
	}

	totalPages := int((total + int64(perPage) - 1) / int64(perPage))
	if page > totalPages && totalPages > 0 {
		page = totalPages
	}

	var data []PowerReadingWithDesc
	err := dataQ.Order("power_readings.olt_rx ASC").Offset((page - 1) * perPage).Limit(perPage).Scan(&data).Error
	if err != nil {
		return nil, err
	}

	return &PaginatedReadings{
		Data:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

func (r *powerRepository) GetByHost(host string) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("host = ?", host).Order("ont_idx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetWeak(threshold float64) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("olt_rx < ?", threshold).Order("olt_rx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetDevices() ([]DeviceInfo, error) {
	var out []DeviceInfo
	err := r.DB.Model(&models.PowerReading{}).
		Select("DISTINCT device, site, host").
		Order("site, device").
		Find(&out).Error
	return out, err
}

func (r *powerRepository) GetSummary(threshold float64) ([]DevicePowerSummary, error) {
	var out []DevicePowerSummary
	err := r.DB.Model(&models.PowerReading{}).
		Select("device, site, COUNT(*) as total, SUM(CASE WHEN olt_rx < ? THEN 1 ELSE 0 END) as weak_count", threshold).
		Group("device, site").
		Order("site, device").
		Find(&out).Error
	return out, err
}

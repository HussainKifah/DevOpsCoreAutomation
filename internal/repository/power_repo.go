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

type PowerRepository interface {
	BulkInsert(device, site, host string, readings []models.PowerReading) error
	DeleteByHost(host string) error
	GetAll() ([]models.PowerReading, error)
	GetByHost(host string) ([]models.PowerReading, error)
	GetWeak(threshold float64) ([]models.PowerReading, error)
	GetDevices() ([]DeviceInfo, error)
}
type powerRepository struct {
	DB *gorm.DB
}

func NewPowerRepository(db *gorm.DB) PowerRepository {
	return &powerRepository{DB: db}
}

// insert the data from the device to the database
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

// delete the data by the host of the olt
func (r *powerRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PowerReading{}).Error
}

func (r *powerRepository) GetAll() ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Order("host, ont_idx").Find(&out).Error
	return out, err
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

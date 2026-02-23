package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type PowerRepo struct {
	DB *gorm.DB
}

func NewPowerRepo(db *gorm.DB) *PowerRepo {
	return &PowerRepo{DB: db}
}

func (r *PowerRepo) BulkInsert(device, site, host string, readings []models.PowerReading) error {
	now := time.Now()
	for i := range readings {
		readings[i].Device = device
		readings[i].Site = site
		readings[i].Host = host
		readings[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(readings, 100).Error
}

func (r *PowerRepo) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PowerReading{}).Error
}

func (r *PowerRepo) GetAll() ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *PowerRepo) GetByHost(host string) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("host = ?", host).Order("ont_idx").Find(&out).Error
	return out, err
}

func (r *PowerRepo) GetWeak(threshold float64) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("olt_rx < ?", threshold).Order("olt_rx").Find(&out).Error
	return out, err
}

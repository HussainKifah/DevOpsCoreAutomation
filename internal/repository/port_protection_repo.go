package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type PortProtectionRepository interface {
	BulkInsert(device, site, host string, records []models.PortProtectionRecord) error
	DeleteByHost(host string) error
	GetAll() ([]models.PortProtectionRecord, error)
	GetByHost(host string) ([]models.PortProtectionRecord, error)
	GetDown() ([]models.PortProtectionRecord, error)
}

type portProtectionRepository struct {
	DB *gorm.DB
}

func NewPortProtectionRepo(db *gorm.DB) PortProtectionRepository {
	return &portProtectionRepository{DB: db}
}

func (r *portProtectionRepository) BulkInsert(device, site, host string, records []models.PortProtectionRecord) error {
	now := time.Now()
	for i := range records {
		records[i].Device = device
		records[i].Site = site
		records[i].Host = host
		records[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(records, 100).Error
}

func (r *portProtectionRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PortProtectionRecord{}).Error
}

func (r *portProtectionRepository) GetAll() ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	err := r.DB.Order("host, port").Find(&out).Error
	return out, err
}

func (r *portProtectionRepository) GetByHost(host string) ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	err := r.DB.Where("host = ?", host).Order("port").Find(&out).Error
	return out, err
}

func (r *portProtectionRepository) GetDown() ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	err := r.DB.Where("port_state LIKE ? OR paired_state LIKE ?", "%down%", "%down%").
		Order("host, port").Find(&out).Error
	return out, err
}

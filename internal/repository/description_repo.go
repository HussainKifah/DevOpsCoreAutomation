package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type DescriptionRepository interface {
	BulkInsert(device, site, host string, descs []models.OntDescription) error
	DeleteByHost(host string) error
	GetAll() ([]models.OntDescription, error)
	GetByHost(host string) ([]models.OntDescription, error)
}

type descriptionRepository struct {
	DB *gorm.DB
}

func NewDescriptionRepository(db *gorm.DB) DescriptionRepository {
	return &descriptionRepository{DB: db}
}

func (r *descriptionRepository) BulkInsert(device, site, host string, descs []models.OntDescription) error {
	now := time.Now()
	for i := range descs {
		descs[i].Device = device
		descs[i].Site = site
		descs[i].Host = host
		descs[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(descs, 100).Error
}

func (r *descriptionRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.OntDescription{}).Error
}

func (r *descriptionRepository) GetAll() ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *descriptionRepository) GetByHost(host string) ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Where("host = ?", host).Order("ont_idx").Find(&out).Error
	return out, err
}

package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type DescriptionRepo struct {
	DB *gorm.DB
}

func NewDescriptionRepo(db *gorm.DB) *DescriptionRepo {
	return &DescriptionRepo{DB: db}
}

func (r *DescriptionRepo) BulkInsert(device, site, host string, descs []models.OntDescription) error {
	now := time.Now()
	for i := range descs {
		descs[i].Device = device
		descs[i].Site = site
		descs[i].Host = host
		descs[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(descs, 100).Error
}

func (r *DescriptionRepo) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.OntDescription{}).Error
}

func (r *DescriptionRepo) GetAll() ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *DescriptionRepo) GetByHost(host string) ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Where("host = ?", host).Order("ont_idx").Find(&out).Error
	return out, err
}

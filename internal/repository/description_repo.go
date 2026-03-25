package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type DescBatch struct {
	Device  string
	Site    string
	Host    string
	Records []models.OntDescription
}

type DescriptionRepository interface {
	BulkInsert(device, site, host string, descs []models.OntDescription) error
	ReplaceAll(batches []DescBatch) error
	DeleteByHost(host string) error
	GetAll(vendor string) ([]models.OntDescription, error)
	GetByHost(host, vendor string) ([]models.OntDescription, error)
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

func (r *descriptionRepository) ReplaceAll(batches []DescBatch) error {
	if len(batches) == 0 {
		return nil
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, b := range batches {
			vendor := "nokia"
			if len(b.Records) > 0 && b.Records[0].Vendor != "" {
				vendor = b.Records[0].Vendor
			}
			if err := tx.Where("host = ? AND vendor = ?", b.Host, vendor).Delete(&models.OntDescription{}).Error; err != nil {
				return err
			}
			if len(b.Records) == 0 {
				continue
			}
			for i := range b.Records {
				b.Records[i].Device = b.Device
				b.Records[i].Site = b.Site
				b.Records[i].Host = b.Host
				b.Records[i].MeasuredAt = now
			}
			if err := tx.CreateInBatches(b.Records, 100).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *descriptionRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.OntDescription{}).Error
}

func (r *descriptionRepository) GetAll(vendor string) ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Where("vendor = ?", vendor).Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *descriptionRepository) GetByHost(host, vendor string) ([]models.OntDescription, error) {
	var out []models.OntDescription
	err := r.DB.Where("host = ? AND vendor = ?", host, vendor).Order("ont_idx").Find(&out).Error
	return out, err
}

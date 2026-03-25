package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type PortBatch struct {
	Device  string
	Site    string
	Host    string
	Records []models.PortProtectionRecord
}

type PortProtectionRepository interface {
	BulkInsert(device, site, host string, records []models.PortProtectionRecord) error
	ReplaceAll(batches []PortBatch) error
	DeleteByHost(host string) error
	GetAll(vendor string) ([]models.PortProtectionRecord, error)
	GetByHost(host, vendor string) ([]models.PortProtectionRecord, error)
	GetDown(vendor string) ([]models.PortProtectionRecord, error)
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

func (r *portProtectionRepository) ReplaceAll(batches []PortBatch) error {
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
			if err := tx.Where("host = ? AND vendor = ?", b.Host, vendor).Delete(&models.PortProtectionRecord{}).Error; err != nil {
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

func (r *portProtectionRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PortProtectionRecord{}).Error
}

func (r *portProtectionRepository) GetAll(vendor string) ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	err := r.DB.Where("vendor = ?", vendor).Order("host, port").Find(&out).Error
	return out, err
}

func (r *portProtectionRepository) GetByHost(host, vendor string) ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	err := r.DB.Where("host = ? AND vendor = ?", host, vendor).Order("port").Find(&out).Error
	return out, err
}

func (r *portProtectionRepository) GetDown(vendor string) ([]models.PortProtectionRecord, error) {
	var out []models.PortProtectionRecord
	if vendor == "huawei" {
		err := r.DB.Where("vendor = ?", vendor).Order("host, port").Find(&out).Error
		return out, err
	}
	err := r.DB.Where("(port_state LIKE ? OR paired_state LIKE ?) AND vendor = ?", "%down%", "%down%", vendor).
		Order("host, port").Find(&out).Error
	return out, err
}

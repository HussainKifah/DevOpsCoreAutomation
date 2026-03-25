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
	DeleteExceptHosts(hosts []string) error
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

func (r *portProtectionRepository) ReplaceAll(batches []PortBatch) error {
	if len(batches) == 0 {
		return nil
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, b := range batches {
			if err := tx.Unscoped().Where("host = ?", b.Host).Delete(&models.PortProtectionRecord{}).Error; err != nil {
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

func (r *portProtectionRepository) DeleteExceptHosts(hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	return r.DB.Unscoped().Where("host NOT IN ?", hosts).Delete(&models.PortProtectionRecord{}).Error
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

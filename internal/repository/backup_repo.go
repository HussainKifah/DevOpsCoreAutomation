package repository

import (
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type BackupRepository interface {
	Create(backup *models.OltBackups) error
	GetAll() ([]models.OltBackups, error)
	GetBySite(site string) ([]models.OltBackups, error)
	GetByID(id uint) (*models.OltBackups, error)
}

type backupRepository struct {
	DB *gorm.DB
}

func NewBackupRepository(db *gorm.DB) BackupRepository {
	return &backupRepository{DB: db}
}

func (r *backupRepository) Create(backup *models.OltBackups) error {
	return r.DB.Create(backup).Error
}

func (r *backupRepository) GetAll() ([]models.OltBackups, error) {
	var out []models.OltBackups
	err := r.DB.Order("created_at DESC").Find(&out).Error
	return out, err
}

func (r *backupRepository) GetBySite(site string) ([]models.OltBackups, error) {
	var out []models.OltBackups
	err := r.DB.Where("site = ?", site).Order("created_at DESC").Find(&out).Error
	return out, err
}

func (r *backupRepository) GetByID(id uint) (*models.OltBackups, error) {
	var b models.OltBackups
	err := r.DB.First(&b, id).Error
	if err != nil {
		return nil, err
	}
	return &b, nil
}

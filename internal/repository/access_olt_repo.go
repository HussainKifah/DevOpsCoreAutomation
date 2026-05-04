package repository

import (
	"strings"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type AccessOltRepository interface {
	ListOlts() ([]models.AccessOlt, error)
	CreateOlt(olt *models.AccessOlt) error
	DeleteOlt(id uint) error
	ListCredentials(vendorFamily string) ([]models.AccessOltCredential, error)
	CreateCredential(c *models.AccessOltCredential) error
	DeleteCredential(id uint) error
}

type accessOltRepo struct {
	db *gorm.DB
}

func NewAccessOltRepository(db *gorm.DB) AccessOltRepository {
	return &accessOltRepo{db: db}
}

func (r *accessOltRepo) ListOlts() ([]models.AccessOlt, error) {
	var list []models.AccessOlt
	err := r.db.Order("site ASC, name ASC, ip ASC").Find(&list).Error
	return list, err
}

func (r *accessOltRepo) CreateOlt(olt *models.AccessOlt) error {
	return r.db.Create(olt).Error
}

func (r *accessOltRepo) DeleteOlt(id uint) error {
	return r.db.Delete(&models.AccessOlt{}, id).Error
}

func (r *accessOltRepo) ListCredentials(vendorFamily string) ([]models.AccessOltCredential, error) {
	var list []models.AccessOltCredential
	tx := r.db.Model(&models.AccessOltCredential{}).Where("enabled = ?", true)
	if strings.TrimSpace(vendorFamily) != "" {
		tx = tx.Where("vendor_family = ?", strings.ToLower(strings.TrimSpace(vendorFamily)))
	}
	err := tx.Order("vendor_family ASC, created_at ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *accessOltRepo) CreateCredential(c *models.AccessOltCredential) error {
	return r.db.Create(c).Error
}

func (r *accessOltRepo) DeleteCredential(id uint) error {
	return r.db.Delete(&models.AccessOltCredential{}, id).Error
}

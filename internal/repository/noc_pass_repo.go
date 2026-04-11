package repository

import (
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type NocPassRepository interface {
	ListEnabled() ([]models.NocPassDevice, error)
	Search(q string) ([]models.NocPassDevice, error)
	GetByID(id uint) (*models.NocPassDevice, error)
	Create(d *models.NocPassDevice) error
	Update(d *models.NocPassDevice) error
	Delete(id uint) error
	ListKeepUsers() ([]models.NocPassKeepUser, error)
	CreateKeepUser(user *models.NocPassKeepUser) error
	DeleteKeepUser(id uint) error
	UpdateAfterApply(id uint, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error
}

type nocPassRepo struct {
	db *gorm.DB
}

func NewNocPassRepository(db *gorm.DB) NocPassRepository {
	return &nocPassRepo{db: db}
}

func (r *nocPassRepo) ListEnabled() ([]models.NocPassDevice, error) {
	var list []models.NocPassDevice
	err := r.db.Where("enabled = ?", true).Order("display_name ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) Search(q string) ([]models.NocPassDevice, error) {
	var list []models.NocPassDevice
	qq := strings.TrimSpace(q)
	tx := r.db.Model(&models.NocPassDevice{}).Where("enabled = ?", true)
	if qq != "" {
		pat := "%" + strings.ToLower(qq) + "%"
		tx = tx.Where("LOWER(display_name) LIKE ? OR LOWER(host) LIKE ?", pat, pat)
	}
	err := tx.Order("display_name ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) GetByID(id uint) (*models.NocPassDevice, error) {
	var d models.NocPassDevice
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *nocPassRepo) Create(d *models.NocPassDevice) error {
	return r.db.Create(d).Error
}

func (r *nocPassRepo) Update(d *models.NocPassDevice) error {
	return r.db.Save(d).Error
}

func (r *nocPassRepo) Delete(id uint) error {
	return r.db.Delete(&models.NocPassDevice{}, id).Error
}

func (r *nocPassRepo) ListKeepUsers() ([]models.NocPassKeepUser, error) {
	var list []models.NocPassKeepUser
	err := r.db.Order("username ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) CreateKeepUser(user *models.NocPassKeepUser) error {
	return r.db.Create(user).Error
}

func (r *nocPassRepo) DeleteKeepUser(id uint) error {
	return r.db.Delete(&models.NocPassKeepUser{}, id).Error
}

func (r *nocPassRepo) UpdateAfterApply(id uint, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_apply_ok":    ok,
		"last_apply_error": errMsg,
		"last_applied_at":  now,
	}
	if ok && len(encNocPass) > 0 && rotatedAt != nil {
		updates["enc_noc_password"] = encNocPass
		updates["password_rotated_at"] = rotatedAt
	}
	return r.db.Model(&models.NocPassDevice{}).Where("id = ?", id).Updates(updates).Error
}

package repository

import (
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HealthRepo struct {
	DB *gorm.DB
}

func NewHealthRepo(db *gorm.DB) *HealthRepo {
	return &HealthRepo{DB: db}
}

// Upsert inserts or updates health data keyed by host.
func (r *HealthRepo) Upsert(h *models.OltHealth) error {
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "host"}},
		DoUpdates: clause.AssignmentColumns([]string{"device", "site", "uptime", "cpu_loads", "temperatures", "measured_at"}),
	}).Create(h).Error
}

func (r *HealthRepo) GetAll() ([]models.OltHealth, error) {
	var out []models.OltHealth
	err := r.DB.Order("host").Find(&out).Error
	return out, err
}

func (r *HealthRepo) GetByHost(host string) (*models.OltHealth, error) {
	var h models.OltHealth
	err := r.DB.Where("host = ?", host).First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

package repository

import (
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type HealthRepository interface {
	Upsert(h *models.OltHealth) error
	BulkUpsert(records []*models.OltHealth) error
	GetAll() ([]models.OltHealth, error)
	GetByHost(host string) (*models.OltHealth, error)
}

type healthRepository struct {
	DB *gorm.DB
}

func NewHealthRepository(db *gorm.DB) HealthRepository {
	return &healthRepository{DB: db}
}

func (r *healthRepository) Upsert(h *models.OltHealth) error {
	return r.DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "host"}},
		DoUpdates: clause.AssignmentColumns([]string{"device", "site", "uptime", "cpu_loads", "temperatures", "measured_at"}),
	}).Create(h).Error
}

func (r *healthRepository) BulkUpsert(records []*models.OltHealth) error {
	if len(records) == 0 {
		return nil
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		for _, h := range records {
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "host"}},
				DoUpdates: clause.AssignmentColumns([]string{"device", "site", "uptime", "cpu_loads", "temperatures", "measured_at"}),
			}).Create(h).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *healthRepository) GetAll() ([]models.OltHealth, error) {
	var out []models.OltHealth
	err := r.DB.Order("host").Find(&out).Error
	return out, err
}

func (r *healthRepository) GetByHost(host string) (*models.OltHealth, error) {
	var h models.OltHealth
	err := r.DB.Where("host = ?", host).First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

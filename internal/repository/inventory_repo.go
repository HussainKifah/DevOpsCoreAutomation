package repository

import (
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type InventoryRepository interface {
	SaveSummary(summary *models.InventorySummary) error
	SaveOltInventory(inventories []models.OltInventory) error
	ReplaceOntInventoryByHost(host string, items []models.OntInventoryItem) error
	GetLatestSummary() (*models.InventorySummary, error)
	GetLatestOltInventories() ([]models.OltInventory, error)
	GetOltInventoryHistory(host string, limit int) ([]models.OltInventory, error)
}

type InventoryRepo struct {
	db *gorm.DB
}

func NewInventoryRepo(db *gorm.DB) *InventoryRepo {
	return &InventoryRepo{db: db}
}

func (r *InventoryRepo) SaveSummary(summary *models.InventorySummary) error {
	return r.db.Create(summary).Error
}

func (r *InventoryRepo) SaveOltInventory(inventories []models.OltInventory) error {
	if len(inventories) == 0 {
		return nil
	}
	return r.db.Create(&inventories).Error
}

func (r *InventoryRepo) ReplaceOntInventoryByHost(host string, items []models.OntInventoryItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("host = ?", host).Delete(&models.OntInventoryItem{}).Error; err != nil {
			return err
		}
		var valid []models.OntInventoryItem
		for _, it := range items {
			if it.OntIdx != "" {
				valid = append(valid, models.OntInventoryItem{Host: host, OntIdx: it.OntIdx, EquipID: it.EquipID, SerialNo: it.SerialNo})
			}
		}
		if len(valid) == 0 {
			return nil
		}
		return tx.CreateInBatches(valid, 200).Error
	})
}

func (r *InventoryRepo) GetLatestSummary() (*models.InventorySummary, error) {
	var summary models.InventorySummary
	err := r.db.Order("measured_at desc").First(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *InventoryRepo) GetLatestOltInventories() ([]models.OltInventory, error) {
	var inventories []models.OltInventory
	// Subquery to get the latest measured_at per host
	subQuery := r.db.Model(&models.OltInventory{}).Select("host, MAX(measured_at) as max_measured_at").Group("host")
	err := r.db.Joins("INNER JOIN (?) as latest ON olt_inventories.host = latest.host AND olt_inventories.measured_at = latest.max_measured_at", subQuery).Find(&inventories).Error
	return inventories, err
}

func (r *InventoryRepo) GetOltInventoryHistory(host string, limit int) ([]models.OltInventory, error) {
	var inventories []models.OltInventory
	err := r.db.Where("host = ?", host).Order("measured_at desc").Limit(limit).Find(&inventories).Error
	return inventories, err
}

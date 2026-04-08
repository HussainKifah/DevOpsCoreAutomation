package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type OntInterfaceRow struct {
	ID             uint      `json:"ID"`
	Device         string    `json:"device"`
	Site           string    `json:"site"`
	Host           string    `json:"host"`
	OntIdx         string    `json:"ont_idx"`
	EqptVerNum     string    `json:"eqpt_ver_num"`
	SwVerAct       string    `json:"sw_ver_act"`
	ActualNumSlots string    `json:"actual_num_slots"`
	VersionNumber  string    `json:"version_number"`
	SerNum         string    `json:"sernum"`
	YpSerialNo     string    `json:"yp_serial_no"`
	CfgFile1VerAct string    `json:"cfgfile1_ver_act"`
	CfgFile2VerAct string    `json:"cfgfile2_ver_act"`
	MeasuredAt     time.Time `json:"measured_at"`
}

type PaginatedOntInterfaces struct {
	Data       []OntInterfaceRow `json:"data"`
	Total      int64             `json:"total"`
	Page       int               `json:"page"`
	PerPage    int               `json:"per_page"`
	TotalPages int               `json:"total_pages"`
}

type InventoryRepository interface {
	SaveSummary(summary *models.InventorySummary) error
	SaveOltInventory(inventories []models.OltInventory) error
	DeleteInventorySnapshot(vendor string) error
	DeleteOntInterfaces(vendor string) error
	ReplaceOntInventoryByHost(host, vendor string, items []models.OntInventoryItem) error
	ReplaceOntInterfacesByHost(device, site, host, vendor string, items []models.OntInterface) error
	GetLatestSummary(vendor string) (*models.InventorySummary, error)
	GetLatestOltInventories(vendor string) ([]models.OltInventory, error)
	GetOltInventoryHistory(host string, limit int) ([]models.OltInventory, error)
	GetOntInterfacesPaginated(page, perPage int, vendor, device, search, sortBy, sortOrder string) (*PaginatedOntInterfaces, error)
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

func (r *InventoryRepo) DeleteInventorySnapshot(vendor string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("vendor = ?", vendor).Delete(&models.InventorySummary{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("vendor = ?", vendor).Delete(&models.OltInventory{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("vendor = ?", vendor).Delete(&models.OntInventoryItem{}).Error
	})
}

func (r *InventoryRepo) DeleteOntInterfaces(vendor string) error {
	return r.db.Unscoped().Where("vendor = ?", vendor).Delete(&models.OntInterface{}).Error
}

func (r *InventoryRepo) ReplaceOntInventoryByHost(host, vendor string, items []models.OntInventoryItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("host = ? AND vendor = ?", host, vendor).Delete(&models.OntInventoryItem{}).Error; err != nil {
			return err
		}
		var valid []models.OntInventoryItem
		seen := make(map[string]int)
		for _, it := range items {
			if it.OntIdx == "" {
				continue
			}
			row := models.OntInventoryItem{Host: host, OntIdx: it.OntIdx, Vendor: vendor, EquipID: it.EquipID, SerialNo: it.SerialNo}
			if idx, ok := seen[it.OntIdx]; ok {
				valid[idx] = row
				continue
			}
			seen[it.OntIdx] = len(valid)
			valid = append(valid, row)
		}
		if len(valid) == 0 {
			return nil
		}
		return tx.CreateInBatches(valid, 200).Error
	})
}

func (r *InventoryRepo) ReplaceOntInterfacesByHost(device, site, host, vendor string, items []models.OntInterface) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("host = ? AND vendor = ?", host, vendor).Delete(&models.OntInterface{}).Error; err != nil {
			return err
		}
		var valid []models.OntInterface
		seen := make(map[string]int)
		now := time.Now()
		for _, it := range items {
			if it.OntIdx == "" {
				continue
			}
			it.Device = device
			it.Site = site
			it.Host = host
			it.Vendor = vendor
			it.MeasuredAt = now
			if idx, ok := seen[it.OntIdx]; ok {
				valid[idx] = it
				continue
			}
			seen[it.OntIdx] = len(valid)
			valid = append(valid, it)
		}
		if len(valid) == 0 {
			return nil
		}
		return tx.CreateInBatches(valid, 200).Error
	})
}

func (r *InventoryRepo) GetLatestSummary(vendor string) (*models.InventorySummary, error) {
	var summary models.InventorySummary
	err := r.db.Where("vendor = ?", vendor).Order("measured_at desc").First(&summary).Error
	if err != nil {
		return nil, err
	}
	return &summary, nil
}

func (r *InventoryRepo) GetLatestOltInventories(vendor string) ([]models.OltInventory, error) {
	var inventories []models.OltInventory
	subQuery := r.db.Model(&models.OltInventory{}).
		Select("host, MAX(measured_at) as max_measured_at").
		Where("vendor = ?", vendor).
		Group("host")
	err := r.db.Where("olt_inventories.vendor = ?", vendor).
		Joins("INNER JOIN (?) as latest ON olt_inventories.host = latest.host AND olt_inventories.measured_at = latest.max_measured_at", subQuery).
		Find(&inventories).Error
	return inventories, err
}

func (r *InventoryRepo) GetOltInventoryHistory(host string, limit int) ([]models.OltInventory, error) {
	var inventories []models.OltInventory
	err := r.db.Where("host = ?", host).Order("measured_at desc").Limit(limit).Find(&inventories).Error
	return inventories, err
}

func (r *InventoryRepo) GetOntInterfacesPaginated(page, perPage int, vendor, device, search, sortBy, sortOrder string) (*PaginatedOntInterfaces, error) {
	q := r.db.Model(&models.OntInterface{}).Where("vendor = ?", vendor)
	if device != "" {
		q = q.Where("device = ?", device)
	}
	if search != "" {
		pattern := "%" + search + "%"
		q = q.Where(
			"ont_idx ILIKE ? OR eqpt_ver_num ILIKE ? OR sw_ver_act ILIKE ? OR actual_num_slots ILIKE ? OR version_number ILIKE ? OR ser_num ILIKE ? OR yp_serial_no ILIKE ? OR cfg_file1_ver_act ILIKE ? OR cfg_file2_ver_act ILIKE ? OR device ILIKE ? OR site ILIKE ? OR host ILIKE ?",
			pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern, pattern,
		)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}

	totalPages := 1
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
		if totalPages == 0 {
			totalPages = 1
		}
		if page > totalPages {
			page = totalPages
		}
	} else {
		page = 1
	}

	orderCol := "ont_idx"
	switch sortBy {
	case "eqpt_ver_num", "sw_ver_act", "actual_num_slots", "version_number", "ser_num", "yp_serial_no", "cfg_file1_ver_act", "cfg_file2_ver_act", "device", "site", "host", "measured_at":
		orderCol = sortBy
	case "ont_idx":
		orderCol = "ont_idx"
	}
	dir := "ASC"
	if sortOrder == "desc" {
		dir = "DESC"
	}

	dataQ := q.Order(orderCol + " " + dir)
	if perPage > 0 {
		dataQ = dataQ.Offset((page - 1) * perPage).Limit(perPage)
	}

	var data []OntInterfaceRow
	if err := dataQ.Find(&data).Error; err != nil {
		return nil, err
	}

	return &PaginatedOntInterfaces{
		Data:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

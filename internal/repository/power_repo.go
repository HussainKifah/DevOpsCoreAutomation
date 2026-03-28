package repository

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type DeviceInfo struct {
	Device string `json:"device"`
	Site   string `json:"site"`
	Host   string `json:"host"`
}

type DevicePowerSummary struct {
	Device    string `json:"device"`
	Site      string `json:"site"`
	Host      string `json:"host"`
	Total     int64  `json:"total"`
	WeakCount int64  `json:"weak_count"`
}

type PowerReadingWithDesc struct {
	ID         uint      `json:"ID"`
	Device     string    `json:"device"`
	Site       string    `json:"site"`
	Host       string    `json:"host"`
	OntIdx     string    `json:"ont_idx"`
	OltRx      float64   `json:"olt_rx"`
	OntRx      float64   `json:"ont_rx"`
	MeasuredAt time.Time `json:"measured_at"`
	Desc1      string    `json:"desc1"`
	Desc2      string    `json:"desc2"`
	EquipID    string    `json:"equip_id,omitempty"`
	SerialNo   string    `json:"serial_no,omitempty"`
}

type PaginatedReadings struct {
	Data       []PowerReadingWithDesc `json:"data"`
	Total      int64                  `json:"total"`
	Page       int                    `json:"page"`
	PerPage    int                    `json:"per_page"`
	TotalPages int                    `json:"total_pages"`
}

type PowerBatch struct {
	Device  string
	Site    string
	Host    string
	Records []models.PowerReading
}

type PowerRepository interface {
	BulkInsert(device, site, host string, readings []models.PowerReading) error
	ReplaceAll(batches []PowerBatch) error
	DeleteByHost(host string) error
	DeleteExceptHosts(hosts []string) error
	GetAll(vendor string) ([]models.PowerReading, error)
	GetPaginated(page, perPage int, vendor, device, search, sortBy, sortOrder string) (*PaginatedReadings, error)
	GetByHost(host, vendor string) ([]models.PowerReading, error)
	GetWeak(threshold float64, vendor string) ([]models.PowerReading, error)
	GetDevices(vendor string) ([]DeviceInfo, error)
	GetSummary(threshold float64, vendor string) ([]DevicePowerSummary, error)
	GetOntIndicesByHost(host string) ([]string, error)
}
type powerRepository struct {
	DB *gorm.DB
}

func NewPowerRepository(db *gorm.DB) PowerRepository {
	return &powerRepository{DB: db}
}

func (r *powerRepository) BulkInsert(device, site, host string, readings []models.PowerReading) error {
	now := time.Now()
	for i := range readings {
		readings[i].Device = device
		readings[i].Site = site
		readings[i].Host = host
		readings[i].MeasuredAt = now
	}
	return r.DB.CreateInBatches(readings, 100).Error
}

func (r *powerRepository) ReplaceAll(batches []PowerBatch) error {
	if len(batches) == 0 {
		return nil
	}
	return r.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		for _, b := range batches {
			if err := tx.Unscoped().Where("host = ?", b.Host).Delete(&models.PowerReading{}).Error; err != nil {
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

func (r *powerRepository) DeleteByHost(host string) error {
	return r.DB.Where("host = ?", host).Delete(&models.PowerReading{}).Error
}

func (r *powerRepository) DeleteExceptHosts(hosts []string) error {
	if len(hosts) == 0 {
		return nil
	}
	return r.DB.Unscoped().Where("host NOT IN ?", hosts).Delete(&models.PowerReading{}).Error
}

func (r *powerRepository) GetAll(vendor string) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("vendor = ?", vendor).Order("host, ont_idx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetPaginated(page, perPage int, vendor, device, search, sortBy, sortOrder string) (*PaginatedReadings, error) {
	descJoin := "LEFT JOIN ont_descriptions ON power_readings.ont_idx = ont_descriptions.ont_idx AND power_readings.host = ont_descriptions.host AND ont_descriptions.deleted_at IS NULL"
	invJoin := "LEFT JOIN ont_inventory_items ON power_readings.ont_idx = ont_inventory_items.ont_idx AND power_readings.host = ont_inventory_items.host AND ont_inventory_items.deleted_at IS NULL"

	countQ := r.DB.Model(&models.PowerReading{}).Joins(descJoin).Where("power_readings.vendor = ?", vendor)
	if device != "" {
		countQ = countQ.Where("power_readings.device = ?", device)
	}
	if search != "" {
		pattern := "%" + search + "%"
		countQ = countQ.Where("power_readings.ont_idx ILIKE ? OR ont_descriptions.desc1 ILIKE ? OR ont_descriptions.desc2 ILIKE ?", pattern, pattern, pattern)
	}

	var total int64
	if err := countQ.Count(&total).Error; err != nil {
		return nil, err
	}

	var totalPages int
	if perPage > 0 {
		totalPages = int((total + int64(perPage) - 1) / int64(perPage))
		if page > totalPages && totalPages > 0 {
			page = totalPages
		}
	} else {
		totalPages = 1
		page = 1
	}

	dataQ := r.DB.Model(&models.PowerReading{}).
		Select("power_readings.id, power_readings.device, power_readings.site, power_readings.host, power_readings.ont_idx, power_readings.olt_rx, power_readings.ont_rx, power_readings.measured_at, COALESCE(ont_descriptions.desc1, '') as desc1, COALESCE(ont_descriptions.desc2, '') as desc2, COALESCE(ont_inventory_items.equip_id, '') as equip_id, COALESCE(ont_inventory_items.serial_no, '') as serial_no").
		Joins(descJoin).Joins(invJoin).
		Where("power_readings.vendor = ?", vendor)
	if device != "" {
		dataQ = dataQ.Where("power_readings.device = ?", device)
	}
	if search != "" {
		pattern := "%" + search + "%"
		dataQ = dataQ.Where("power_readings.ont_idx ILIKE ? OR ont_descriptions.desc1 ILIKE ? OR ont_descriptions.desc2 ILIKE ?", pattern, pattern, pattern)
	}

	if perPage > 0 {
		dataQ = dataQ.Offset((page - 1) * perPage).Limit(perPage)
	}

	// Whitelist sort columns for SQL safety
	orderCol := "power_readings.ont_rx"
	if sortBy != "" {
		switch sortBy {
		case "ont_idx":
			orderCol = "power_readings.ont_idx"
		case "olt_rx":
			orderCol = "power_readings.olt_rx"
		case "ont_rx":
			orderCol = "power_readings.ont_rx"
		case "device":
			orderCol = "power_readings.device"
		case "site":
			orderCol = "power_readings.site"
		case "host":
			orderCol = "power_readings.host"
		case "measured_at":
			orderCol = "power_readings.measured_at"
		case "desc1":
			orderCol = "ont_descriptions.desc1"
		case "desc2":
			orderCol = "ont_descriptions.desc2"
		case "equip_id":
			orderCol = "ont_inventory_items.equip_id"
		case "serial_no":
			orderCol = "ont_inventory_items.serial_no"
		}
	}
	dir := "ASC"
	if sortOrder == "desc" {
		dir = "DESC"
	}
	orderClause := orderCol + " " + dir

	var data []PowerReadingWithDesc
	err := dataQ.Order(orderClause).Scan(&data).Error
	if err != nil {
		return nil, err
	}

	return &PaginatedReadings{
		Data:       data,
		Total:      total,
		Page:       page,
		PerPage:    perPage,
		TotalPages: totalPages,
	}, nil
}

func (r *powerRepository) GetByHost(host, vendor string) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("host = ? AND vendor = ?", host, vendor).Order("ont_idx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetWeak(threshold float64, vendor string) ([]models.PowerReading, error) {
	var out []models.PowerReading
	err := r.DB.Where("ont_rx < ? AND vendor = ?", threshold, vendor).Order("ont_rx").Find(&out).Error
	return out, err
}

func (r *powerRepository) GetDevices(vendor string) ([]DeviceInfo, error) {
	var out []DeviceInfo
	err := r.DB.Model(&models.PowerReading{}).
		Select("DISTINCT device, site, host").
		Where("vendor = ?", vendor).
		Order("site, device").
		Find(&out).Error
	return out, err
}

func (r *powerRepository) GetSummary(threshold float64, vendor string) ([]DevicePowerSummary, error) {
	var out []DevicePowerSummary
	err := r.DB.Model(&models.PowerReading{}).
		Select("device, site, host, COUNT(*) as total, SUM(CASE WHEN ont_rx < ? THEN 1 ELSE 0 END) as weak_count", threshold).
		Where("vendor = ?", vendor).
		Group("device, site, host").
		Order("site, device").
		Find(&out).Error
	return out, err
}

func (r *powerRepository) GetOntIndicesByHost(host string) ([]string, error) {
	var rows []struct {
		OntIdx string
	}
	err := r.DB.Model(&models.PowerReading{}).
		Select("ont_idx").
		Where("host = ?", host).
		Order("ont_idx").
		Distinct("ont_idx").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.OntIdx)
	}
	return out, nil
}

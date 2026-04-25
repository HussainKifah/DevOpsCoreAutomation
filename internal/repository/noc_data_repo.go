package repository

import (
	"fmt"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type NocDataRepository interface {
	List(q string) ([]models.NocDataDevice, error)
	ListAll() ([]models.NocDataDevice, error)
	GetByID(id uint) (*models.NocDataDevice, error)
	FindByRangeHost(site, subnet, deviceRange, host string) (*models.NocDataDevice, error)
	FindByIdentity(hostname, model, version, serial string) ([]models.NocDataDevice, error)
	ListCredentials(vendorFamily string) ([]models.NocDataCredential, error)
	CreateCredential(c *models.NocDataCredential) error
	DeleteCredential(id uint) error
	ListExclusions() ([]models.NocDataExclusion, error)
	CreateExclusion(e *models.NocDataExclusion) error
	DeleteExclusion(id uint) error
	Create(d *models.NocDataDevice) error
	UpdateDevice(id uint, updates map[string]interface{}) error
	Delete(id uint) error
	HardDelete(id uint) error
	UpdateSnapshot(id uint, updates map[string]interface{}) error
	CreateHistorySnapshot(runAt time.Time, devices []models.NocDataDevice) error
	ListHistoryDates(limit int) ([]time.Time, error)
	ListHistoryByDate(day time.Time) ([]models.NocDataHistory, error)
}

type nocDataRepo struct {
	db *gorm.DB
}

func NewNocDataRepository(db *gorm.DB) NocDataRepository {
	return &nocDataRepo{db: db}
}

func (r *nocDataRepo) List(q string) ([]models.NocDataDevice, error) {
	var list []models.NocDataDevice
	tx := r.db.Model(&models.NocDataDevice{})
	qq := strings.TrimSpace(q)
	if qq != "" {
		pat := "%" + strings.ToLower(qq) + "%"
		tx = tx.Where("LOWER(display_name) LIKE ? OR LOWER(site) LIKE ? OR LOWER(subnet) LIKE ? OR LOWER(device_range) LIKE ? OR LOWER(host) LIKE ?", pat, pat, pat, pat, pat)
	}
	err := tx.Order("site ASC, subnet ASC, device_range ASC, host ASC").Find(&list).Error
	return list, err
}

func (r *nocDataRepo) ListAll() ([]models.NocDataDevice, error) {
	return r.List("")
}

func (r *nocDataRepo) GetByID(id uint) (*models.NocDataDevice, error) {
	var d models.NocDataDevice
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *nocDataRepo) FindByRangeHost(site, subnet, deviceRange, host string) (*models.NocDataDevice, error) {
	var d models.NocDataDevice
	err := r.db.
		Where("site = ? AND subnet = ? AND device_range = ? AND host = ?", site, subnet, deviceRange, host).
		Order("id ASC").
		First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *nocDataRepo) Create(d *models.NocDataDevice) error {
	return r.db.Create(d).Error
}

func (r *nocDataRepo) FindByIdentity(hostname, model, version, serial string) ([]models.NocDataDevice, error) {
	var list []models.NocDataDevice
	err := r.db.
		Where("LOWER(hostname) = ? AND LOWER(device_model) = ? AND LOWER(version) = ? AND LOWER(serial) = ?",
			strings.ToLower(strings.TrimSpace(hostname)),
			strings.ToLower(strings.TrimSpace(model)),
			strings.ToLower(strings.TrimSpace(version)),
			strings.ToLower(strings.TrimSpace(serial)),
		).
		Order("id ASC").
		Find(&list).Error
	return list, err
}

func (r *nocDataRepo) ListCredentials(vendorFamily string) ([]models.NocDataCredential, error) {
	var list []models.NocDataCredential
	tx := r.db.Model(&models.NocDataCredential{}).Where("enabled = ?", true)
	if strings.TrimSpace(vendorFamily) != "" {
		tx = tx.Where("vendor_family = ?", strings.ToLower(strings.TrimSpace(vendorFamily)))
	}
	err := tx.Order("vendor_family ASC, created_at ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocDataRepo) CreateCredential(c *models.NocDataCredential) error {
	return r.db.Create(c).Error
}

func (r *nocDataRepo) DeleteCredential(id uint) error {
	return r.db.Delete(&models.NocDataCredential{}, id).Error
}

func (r *nocDataRepo) ListExclusions() ([]models.NocDataExclusion, error) {
	var list []models.NocDataExclusion
	err := r.db.Order("subnet ASC, target ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocDataRepo) CreateExclusion(e *models.NocDataExclusion) error {
	return r.db.Create(e).Error
}

func (r *nocDataRepo) DeleteExclusion(id uint) error {
	return r.db.Delete(&models.NocDataExclusion{}, id).Error
}

func (r *nocDataRepo) UpdateDevice(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.NocDataDevice{}).Where("id = ?", id).Updates(updates).Error
}

func (r *nocDataRepo) Delete(id uint) error {
	return r.db.Delete(&models.NocDataDevice{}, id).Error
}

func (r *nocDataRepo) HardDelete(id uint) error {
	return r.db.Unscoped().Delete(&models.NocDataDevice{}, id).Error
}

func (r *nocDataRepo) UpdateSnapshot(id uint, updates map[string]interface{}) error {
	sanitizeNocDataSnapshotUpdates(updates)
	updates["last_collected_at"] = time.Now()
	return r.db.Model(&models.NocDataDevice{}).Where("id = ?", id).Updates(updates).Error
}

func (r *nocDataRepo) CreateHistorySnapshot(runAt time.Time, devices []models.NocDataDevice) error {
	if len(devices) == 0 {
		return nil
	}
	rows := make([]models.NocDataHistory, 0, len(devices))
	for i := range devices {
		d := devices[i]
		rows = append(rows, models.NocDataHistory{
			RunAt:           runAt,
			DeviceID:        d.ID,
			DisplayName:     d.DisplayName,
			Site:            d.Site,
			Subnet:          d.Subnet,
			DeviceRange:     d.DeviceRange,
			Host:            d.Host,
			Vendor:          d.Vendor,
			AccessMethod:    d.AccessMethod,
			LastStatus:      d.LastStatus,
			LastError:       d.LastError,
			Hostname:        d.Hostname,
			DeviceModel:     d.DeviceModel,
			Version:         d.Version,
			Serial:          d.Serial,
			Uptime:          d.Uptime,
			IFUp:            d.IFUp,
			IFDown:          d.IFDown,
			DefaultRouter:   d.DefaultRouter,
			LayerMode:       d.LayerMode,
			UserCount:       d.UserCount,
			Users:           d.Users,
			SSHEnabled:      d.SSHEnabled,
			TelnetEnabled:   d.TelnetEnabled,
			SNMPEnabled:     d.SNMPEnabled,
			NTPEnabled:      d.NTPEnabled,
			AAAEnabled:      d.AAAEnabled,
			SyslogEnabled:   d.SyslogEnabled,
			LastCollectedAt: d.LastCollectedAt,
		})
	}
	return r.db.CreateInBatches(rows, 500).Error
}

func (r *nocDataRepo) ListHistoryDates(limit int) ([]time.Time, error) {
	if limit <= 0 || limit > 365 {
		limit = 60
	}
	var rows []struct {
		Day time.Time
	}
	err := r.db.Model(&models.NocDataHistory{}).
		Select("DATE(run_at) AS day").
		Group("DATE(run_at)").
		Order("day DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]time.Time, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Day)
	}
	return out, nil
}

func (r *nocDataRepo) ListHistoryByDate(day time.Time) ([]models.NocDataHistory, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	var run struct {
		RunAt time.Time
	}
	if err := r.db.Model(&models.NocDataHistory{}).
		Select("run_at").
		Where("run_at >= ? AND run_at < ?", start, end).
		Group("run_at").
		Order("run_at DESC").
		Limit(1).
		Scan(&run).Error; err != nil {
		return nil, err
	}
	if run.RunAt.IsZero() {
		return []models.NocDataHistory{}, nil
	}
	var out []models.NocDataHistory
	err := r.db.Where("run_at = ?", run.RunAt).
		Order("site ASC, subnet ASC, device_range ASC, host ASC").
		Find(&out).Error
	return out, err
}

const nocDataLastErrorMaxLen = 1024

func sanitizeNocDataSnapshotUpdates(updates map[string]interface{}) {
	if updates == nil {
		return
	}
	raw, ok := updates["last_error"]
	if !ok || raw == nil {
		return
	}

	text := strings.TrimSpace(toNocDataString(raw))
	if text == "" {
		updates["last_error"] = ""
		return
	}

	text = dedupeNocDataErrorSegments(text)
	if len(text) > nocDataLastErrorMaxLen {
		suffix := "... [truncated]"
		if len(suffix) >= nocDataLastErrorMaxLen {
			text = suffix[:nocDataLastErrorMaxLen]
		} else {
			text = text[:nocDataLastErrorMaxLen-len(suffix)] + suffix
		}
	}
	updates["last_error"] = text
}

func dedupeNocDataErrorSegments(text string) string {
	parts := strings.Split(text, "; ")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return strings.TrimSpace(text)
	}
	return strings.Join(out, "; ")
}

func toNocDataString(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	default:
		return fmt.Sprint(value)
	}
}

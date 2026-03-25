package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

type OltHealth struct {
	gorm.Model
	Device       string    `gorm:"index;not null" json:"device"`
	Site         string    `gorm:"index;not null" json:"site"`
	Host         string    `gorm:"uniqueIndex:idx_health_host_vendor;not null" json:"host"`
	Vendor       string    `gorm:"uniqueIndex:idx_health_host_vendor;index;not null;default:'nokia'" json:"vendor"`
	Uptime       string    `json:"uptime"`
	CpuLoads     JSONSlice `gorm:"type:jsonb" json:"cpu_loads"`
	Temperatures JSONSlice `gorm:"type:jsonb" json:"temperatures"`
	MeasuredAt   time.Time `gorm:"autoUpdateTime" json:"measured_at"`
}

// JSONSlice stores arbitrary JSON arrays in a JSONB column.
type JSONSlice []any

func (j JSONSlice) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	return json.Marshal(j)
}

func (j *JSONSlice) Scan(src any) error {
	if src == nil {
		*j = nil
		return nil
	}
	b, ok := src.([]byte)
	if !ok {
		return errors.New("JSONSlice.Scan: expected []byte")
	}
	return json.Unmarshal(b, j)
}

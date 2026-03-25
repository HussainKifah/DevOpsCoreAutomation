package models

import (
	"time"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"gorm.io/gorm"
)

type InventorySummary struct {
	gorm.Model
	Vendor       string                   `gorm:"index;not null;default:'nokia'" json:"vendor"`
	Count        []extractor.EquipIDCount `gorm:"type:jsonb;serializer:json" json:"counts"`
	VendorCounts []extractor.VendorCount  `gorm:"type:jsonb;serializer:json" json:"vendor_counts"`
	SwVerCounts  []extractor.SwVerCount   `gorm:"type:jsonb;serializer:json" json:"sw_ver_counts"`
	Total        int                      `json:"total"`
	MeasuredAt   time.Time                `gorm:"autoCreateTime;index" json:"measured_at"`
}

type OltInventory struct {
	gorm.Model
	Host         string                   `gorm:"index;not null" json:"host"`
	Device       string                   `gorm:"index;not null" json:"device"`
	Site         string                   `gorm:"index;not null" json:"site"`
	Vendor       string                   `gorm:"index;not null;default:'nokia'" json:"vendor"`
	Counts       []extractor.EquipIDCount `gorm:"type:jsonb;serializer:json" json:"counts"`
	VendorCounts []extractor.VendorCount  `gorm:"type:jsonb;serializer:json" json:"vendor_counts"`
	SwVerCounts  []extractor.SwVerCount   `gorm:"type:jsonb;serializer:json" json:"sw_ver_counts"`
	Total        int                      `json:"total"`
	MeasuredAt   time.Time                `gorm:"autoCreateTime;index" json:"measured_at"`
}

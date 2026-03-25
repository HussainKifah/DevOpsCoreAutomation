package models

import (
	"time"

	"gorm.io/gorm"
)

type PowerReading struct {
	gorm.Model
	Device     string    `gorm:"index;not null" json:"device"`
	Site       string    `gorm:"index;not null" json:"site"`
	Host       string    `gorm:"index;not null" json:"host"`
	Vendor     string    `gorm:"index;not null;default:'nokia'" json:"vendor"`
	OntIdx     string    `gorm:"not null" json:"ont_idx"`
	OltRx      float64   `json:"olt_rx"`
	OntRx      float64   `json:"ont_rx"`
	MeasuredAt time.Time `gorm:"autoCreateTime" json:"measured_at"`
}

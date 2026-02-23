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
	OntIdx     string    `gorm:"not null" json:"ont_idx"`
	OltRx      float64   `json:"olt_rx"`
	MeasuredAt time.Time `gorm:"autoCreateTime" json:"measured_at"`
}

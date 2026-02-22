package models

import (
	"time"

	"gorm.io/gorm"
)

type PortProtectionRecord struct {
	gorm.Model
	Device      string    `gorm:"index;not null" json:"device"`
	Site        string    `gorm:"index;not null" json:"site"`
	Host        string    `gorm:"index;not null" json:"host"`
	Port        string    `gorm:"not null" json:"port"`
	PortState   string    `json:"port_state"`
	PairedState string    `json:"paired_state"`
	SwoReason   string    `json:"swo_reason"`
	NumSwo      int       `json:"num_swo"`
	MeasuredAt  time.Time `gorm:"autoCreateTime" json:"measured_at"`
}

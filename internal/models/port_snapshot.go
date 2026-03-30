package models

import "time"

type PortSnapshot struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Device      string    `gorm:"not null" json:"device"`
	Site        string    `gorm:"not null" json:"site"`
	Host        string    `gorm:"not null" json:"host"`
	Vendor      string    `gorm:"index;not null;default:'nokia'" json:"vendor"`
	Port        string    `gorm:"not null" json:"port"`
	PairedPort  string    `gorm:"size:128" json:"paired_port"`
	PortState   string    `json:"port_state"`
	PairedState string    `json:"paired_state"`
	SwoReason   string    `json:"swo_reason"`
	NumSwo      int       `json:"num_swo"`
	MeasuredAt  time.Time `gorm:"not null" json:"measured_at"`
}

package models

import "time"

type PortSnapshot struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Device      string    `gorm:"not null" json:"device"`
	Site        string    `gorm:"not null" json:"site"`
	Host        string    `gorm:"not null" json:"host"`
	Port        string    `gorm:"not null" json:"port"`
	PortState   string    `json:"port_state"`
	PairedState string    `json:"paired_state"`
	SwoReason   string    `json:"swo_reason"`
	NumSwo      int       `json:"num_swo"`
	MeasuredAt  time.Time `gorm:"not null" json:"measured_at"`
}

package models

import (
	"time"

	"gorm.io/gorm"
)

type OntDescription struct {
	gorm.Model
	Device     string    `gorm:"index;not null" json:"device"`
	Site       string    `gorm:"index;not null" json:"site"`
	Host       string    `gorm:"index;not null" json:"host"`
	OntIdx     string    `gorm:"not null" json:"ont_idx"`
	Desc1      string    `json:"desc1"`
	Desc2      string    `json:"desc2"`
	MeasuredAt time.Time `gorm:"autoCreateTime" json:"measured_at"`
}

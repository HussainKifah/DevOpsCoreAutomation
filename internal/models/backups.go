package models

import "gorm.io/gorm"

type OltBackups struct {
	gorm.Model
	Device   string `gorm:"index;not null" json:"device"`
	Site     string `gorm:"index;not null" json:"site"`
	Host     string `gorm:"index;not null" json:"host"`
	FilePath string `gorm:"not null" json:"file_path"`
}

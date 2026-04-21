package models

import "gorm.io/gorm"

type NocDataExclusion struct {
	gorm.Model
	Subnet string `gorm:"size:64;not null;default:''"`
	Target string `gorm:"size:255;not null"`
}

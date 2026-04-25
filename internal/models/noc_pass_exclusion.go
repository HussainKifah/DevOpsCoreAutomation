package models

import "gorm.io/gorm"

// NocPassExclusion stores IPs or IP ranges that should be skipped by NOC PASS rotation.
type NocPassExclusion struct {
	gorm.Model
	Subnet string `gorm:"size:64;not null;default:''"`
	Target string `gorm:"size:255;not null"`
}

package models

import (
	"time"

	"gorm.io/gorm"
)

// NocPassPolicy stores one NOC PASS policy and its current password state.
type NocPassPolicy struct {
	gorm.Model
	Name                     string `gorm:"size:128;default:'NOC PASS Policy'"`
	Enabled                  bool   `gorm:"default:false"`
	TargetType               string `gorm:"size:32;not null;default:'all_networks';index"`
	TargetValue              string `gorm:"size:255;not null;default:'all';index"`
	TargetLabel              string `gorm:"size:255;not null;default:'All Networks'"`
	PasswordMode             string `gorm:"size:16;not null;default:'random'"`
	EncManualPassword        []byte
	EncActivePassword        []byte
	EncManualFiberxPassword  []byte
	EncManualSupportPassword []byte
	EncActiveFiberxPassword  []byte
	EncActiveSupportPassword []byte
	ActivePasswordDate       string `gorm:"size:10;index"`
	LastRunAt                *time.Time
	LastStatus               string `gorm:"size:20"`
	LastMessage              string `gorm:"type:text"`
}

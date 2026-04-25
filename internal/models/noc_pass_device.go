package models

import (
	"time"

	"gorm.io/gorm"
)

// NocPassDevice stores per-host NOC PASS execution state keyed by host.
// Membership and credentials now come from NOC Data at runtime; legacy admin credential fields are kept for compatibility.
type NocPassDevice struct {
	gorm.Model
	DisplayName       string `gorm:"size:128;not null"`
	Host              string `gorm:"size:255;not null;index"`
	Vendor            string `gorm:"size:32;not null"`
	EncAdminUser      []byte
	EncAdminPass      []byte
	EncNocPassword    []byte
	PasswordRotatedAt *time.Time
	LastApplyOK       bool
	LastApplyError    string `gorm:"size:1024"`
	LastAppliedAt     *time.Time
	Enabled           bool `gorm:"default:true"`
}

package models

import (
	"time"

	"gorm.io/gorm"
)

// NocPassDevice is a network device managed for rotating NOC local-user credentials.
// Vendor: cisco_ios | cisco_nexus | mikrotik
// Accounts are always fiberx (priv 15 / full) and readOnly (priv 13 / read-only); one shared password rotates every 24h.
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

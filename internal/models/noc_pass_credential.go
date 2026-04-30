package models

import (
	"time"

	"gorm.io/gorm"
)

// NocPassCredential stores the latest confirmed password for one local user on one device.
type NocPassCredential struct {
	gorm.Model
	Host              string `gorm:"size:255;not null;uniqueIndex:ux_noc_pass_credential_host_user,priority:1;index"`
	Username          string `gorm:"size:64;not null"`
	CanonicalUsername string `gorm:"size:64;not null;uniqueIndex:ux_noc_pass_credential_host_user,priority:2;index"`
	Source            string `gorm:"size:32;not null;index"`
	EncPassword       []byte `gorm:"not null"`
	SavedUserID       *uint  `gorm:"index"`
	LastApplyOK       bool
	LastApplyError    string `gorm:"size:1024"`
	LastAppliedAt     *time.Time
}

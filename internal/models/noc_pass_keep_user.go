package models

import "gorm.io/gorm"

// NocPassKeepUser stores a globally protected username that the NOC password rotator must not delete.
type NocPassKeepUser struct {
	gorm.Model
	Username string `gorm:"size:64;not null;uniqueIndex"`
}

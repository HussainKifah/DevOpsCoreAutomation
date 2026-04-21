package models

import "gorm.io/gorm"

type NocDataCredential struct {
	gorm.Model
	VendorFamily string `gorm:"size:32;not null;index"`
	EncUsername  []byte `gorm:"not null"`
	EncPassword  []byte `gorm:"not null"`
	Enabled      bool   `gorm:"not null;default:true"`
}

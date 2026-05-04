package models

import "gorm.io/gorm"

type AccessOlt struct {
	gorm.Model
	IP        string   `gorm:"size:255;not null;uniqueIndex" json:"ip"`
	Name      string   `gorm:"size:128;not null" json:"name"`
	Site      string   `gorm:"size:128;not null;index" json:"site"`
	OltType   string   `gorm:"size:32;not null;index" json:"olt_type"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type AccessOltCredential struct {
	gorm.Model
	VendorFamily string `gorm:"size:32;not null;index" json:"vendor_family"`
	EncUsername  []byte `gorm:"not null"`
	EncPassword  []byte `gorm:"not null"`
	Enabled      bool   `gorm:"not null;default:true"`
}

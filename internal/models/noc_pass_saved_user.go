package models

import "gorm.io/gorm"

// NocPassSavedUser stores a managed user that should be preserved by the NOC PASS rotator
// and applied only to the selected NOC Data targets.
type NocPassSavedUser struct {
	gorm.Model
	Username          string `gorm:"size:64;not null"`
	CanonicalUsername string `gorm:"size:64;index"`
	Privilege         string `gorm:"size:16;not null;default:'full'"`
	EncPassword       []byte `gorm:"not null"`
	NetworkTypesJSON  string `gorm:"type:text"`
	ProvincesJSON     string `gorm:"type:text"`
	VendorsJSON       string `gorm:"type:text"`
	ModelsJSON        string `gorm:"type:text"`
	DevicesJSON       string `gorm:"type:text"`
}

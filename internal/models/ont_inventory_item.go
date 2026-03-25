package models

import (
	"gorm.io/gorm"
)

// OntInventoryItem stores per-ONT model (equip-id) and serial from inventory scan.
// Used to join with power readings for the devices tab.
type OntInventoryItem struct {
	gorm.Model
	Host     string `gorm:"index:idx_ont_inv_host_ont,unique;not null" json:"host"`
	OntIdx   string `gorm:"index:idx_ont_inv_host_ont,unique;not null" json:"ont_idx"`
	Vendor   string `gorm:"index:idx_ont_inv_host_ont,unique;not null;default:'nokia'" json:"vendor"`
	EquipID  string `gorm:"index" json:"equip_id"`
	SerialNo string `json:"serial_no,omitempty"`
}

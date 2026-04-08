package models

import (
	"time"

	"gorm.io/gorm"
)

type OntInterface struct {
	gorm.Model
	Device         string    `gorm:"index;not null" json:"device"`
	Site           string    `gorm:"index;not null" json:"site"`
	Host           string    `gorm:"index:idx_ont_interface_host_ont,unique;not null" json:"host"`
	Vendor         string    `gorm:"index:idx_ont_interface_host_ont,unique;not null;default:'nokia'" json:"vendor"`
	OntIdx         string    `gorm:"index:idx_ont_interface_host_ont,unique;not null" json:"ont_idx"`
	EqptVerNum     string    `gorm:"index" json:"eqpt_ver_num"`
	SwVerAct       string    `gorm:"index" json:"sw_ver_act"`
	ActualNumSlots string    `json:"actual_num_slots"`
	VersionNumber  string    `json:"version_number"`
	SerNum         string    `gorm:"index" json:"sernum"`
	YpSerialNo     string    `gorm:"index" json:"yp_serial_no"`
	CfgFile1VerAct string    `json:"cfgfile1_ver_act"`
	CfgFile2VerAct string    `json:"cfgfile2_ver_act"`
	MeasuredAt     time.Time `gorm:"autoCreateTime;index" json:"measured_at"`
}

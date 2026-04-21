package models

import (
	"time"

	"gorm.io/gorm"
)

// NocDataDevice stores the source device plus its latest collected operational snapshot.
type NocDataDevice struct {
	gorm.Model
	DisplayName     string `gorm:"size:128;not null"`
	Site            string `gorm:"size:128;not null;index"`
	Subnet          string `gorm:"size:64;not null;default:''"`
	DeviceRange     string `gorm:"size:128;not null"`
	Host            string `gorm:"size:255;not null;index"`
	Vendor          string `gorm:"size:32;not null"`
	AccessMethod    string `gorm:"size:16;not null;default:'pending'"`
	EncUsername     []byte `gorm:"not null"`
	EncPassword     []byte `gorm:"not null"`
	LastStatus      string `gorm:"size:16"`
	LastError       string `gorm:"size:1024"`
	Hostname        string `gorm:"size:255"`
	DeviceModel     string `gorm:"size:255"`
	Version         string `gorm:"size:255"`
	Serial          string `gorm:"size:255"`
	Uptime          string `gorm:"size:255"`
	IFUp            int
	IFDown          int
	DefaultRouter   bool
	LayerMode       string `gorm:"size:64"`
	UserCount       int
	Users           string `gorm:"type:text"`
	SSHEnabled      bool
	TelnetEnabled   bool
	SNMPEnabled     bool
	NTPEnabled      bool
	AAAEnabled      bool
	SyslogEnabled   bool
	LastCollectedAt *time.Time
}

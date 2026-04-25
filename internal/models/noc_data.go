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

// NocDataHistory stores one table snapshot row for each NOC Data device after a full collector run.
type NocDataHistory struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	RunAt           time.Time  `gorm:"not null;index" json:"run_at"`
	DeviceID        uint       `gorm:"index" json:"device_id"`
	DisplayName     string     `gorm:"size:128;not null" json:"display_name"`
	Site            string     `gorm:"size:128;not null;index" json:"site"`
	Subnet          string     `gorm:"size:64;not null;default:''" json:"subnet"`
	DeviceRange     string     `gorm:"size:128;not null" json:"range"`
	Host            string     `gorm:"size:255;not null;index" json:"ip"`
	Vendor          string     `gorm:"size:32;not null" json:"vendor"`
	AccessMethod    string     `gorm:"size:16;not null;default:'pending'" json:"method"`
	LastStatus      string     `gorm:"size:16" json:"status"`
	LastError       string     `gorm:"size:1024" json:"error"`
	Hostname        string     `gorm:"size:255" json:"hostname"`
	DeviceModel     string     `gorm:"size:255" json:"model"`
	Version         string     `gorm:"size:255" json:"version"`
	Serial          string     `gorm:"size:255" json:"serial"`
	Uptime          string     `gorm:"size:255" json:"uptime"`
	IFUp            int        `json:"if_up"`
	IFDown          int        `json:"if_down"`
	DefaultRouter   bool       `json:"default_router"`
	LayerMode       string     `gorm:"size:64" json:"layer_mode"`
	UserCount       int        `json:"user_count"`
	Users           string     `gorm:"type:text" json:"users"`
	SSHEnabled      bool       `json:"ssh"`
	TelnetEnabled   bool       `json:"telnet"`
	SNMPEnabled     bool       `json:"snmp"`
	NTPEnabled      bool       `json:"ntp"`
	AAAEnabled      bool       `json:"aaa"`
	SyslogEnabled   bool       `json:"syslog"`
	LastCollectedAt *time.Time `json:"last_collected_at"`
	CreatedAt       time.Time  `gorm:"not null;index" json:"created_at"`
}

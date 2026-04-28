package models

import (
	"time"

	"gorm.io/gorm"
)

type WorkflowDevice struct {
	gorm.Model
	Scope       string `gorm:"not null;default:ip;size:20;index" json:"scope"`
	Name        string `gorm:"not null;size:100" json:"name"`
	Host        string `gorm:"not null;size:64" json:"host"`
	Vendor      string `gorm:"not null;size:20" json:"vendor"`                          // nokia | cisco | mikrotik
	NetworkType string `gorm:"size:20;not null;default:wifi;index" json:"network_type"` // ftth | wifi
	Province    string `gorm:"size:120;not null;default:'';index" json:"province"`
	DeviceType  string `gorm:"size:80;not null;default:'';index" json:"device_type"`
	EncUsername []byte `gorm:"not null" json:"-"` // AES-GCM ciphertext
	EncPassword []byte `gorm:"not null" json:"-"` // AES-GCM ciphertext
	CreatedByID uint   `gorm:"index" json:"created_by_id"`
}

type WorkflowJob struct {
	gorm.Model
	Scope       string         `gorm:"not null;default:ip;size:20;index" json:"scope"`
	DeviceID    *uint          `gorm:"index" json:"device_id"`
	Device      WorkflowDevice `gorm:"foreignKey:DeviceID" json:"device,omitempty"`
	TargetType  string         `gorm:"size:32;index" json:"target_type"`
	TargetValue string         `gorm:"size:255;index" json:"target_value"`
	TargetLabel string         `gorm:"size:255" json:"target_label"`
	JobType     string         `gorm:"not null;size:20" json:"job_type"` // backup | command
	Command     string         `gorm:"type:text" json:"command"`
	Schedule    string         `gorm:"not null;size:50" json:"schedule"` // "24h" or "0 21 * * *"
	Enabled     bool           `gorm:"default:true" json:"enabled"`
	LastRunAt   *time.Time     `json:"last_run_at"`
	LastStatus  string         `gorm:"size:20" json:"last_status"` // ok | error | pending
	LastMessage string         `gorm:"type:text" json:"last_message"`
	CreatedByID uint           `gorm:"index" json:"created_by_id"`
}

type WorkflowRun struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Scope      string     `gorm:"not null;default:ip;size:20;index" json:"scope"`
	JobID      uint       `gorm:"not null;index" json:"job_id"`
	DeviceName string     `gorm:"size:100" json:"device_name"`
	Host       string     `gorm:"size:64;index" json:"host"`
	Vendor     string     `gorm:"size:20" json:"vendor"`
	StartedAt  time.Time  `gorm:"not null;index" json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	Status     string     `gorm:"not null;size:20" json:"status"` // ok | error | pending
	Output     string     `gorm:"type:text" json:"output"`
	ErrorMsg   string     `gorm:"type:text" json:"error_msg"`
}

type WorkflowLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Scope      string    `gorm:"not null;default:ip;size:20;index" json:"scope"`
	JobID      *uint     `gorm:"index" json:"job_id,omitempty"` // nil for system events
	RunID      *uint     `gorm:"index" json:"run_id,omitempty"` // nil for non-run events
	DeviceName string    `gorm:"size:100" json:"device_name"`
	Host       string    `gorm:"size:64;index" json:"host"`
	Vendor     string    `gorm:"size:20" json:"vendor"`
	JobType    string    `gorm:"size:20;index" json:"job_type"` // backup | command | system
	Command    string    `gorm:"type:text" json:"command"`
	Level      string    `gorm:"not null;size:10;index" json:"level"` // info | success | warning | error
	Event      string    `gorm:"not null;size:50;index" json:"event"` // job_started | job_success | job_failed | job_skipped | scheduler_reload | system
	Message    string    `gorm:"type:text" json:"message"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `gorm:"not null;index" json:"created_at"`
}

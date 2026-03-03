package models

import "time"

type HealthSnapshot struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Device       string    `gorm:"not null" json:"device"`
	Site         string    `gorm:"not null" json:"site"`
	Host         string    `gorm:"not null" json:"host"`
	Uptime       string    `json:"uptime"`
	CpuLoads     JSONSlice `gorm:"type:jsonb" json:"cpu_loads"`
	Temperatures JSONSlice `gorm:"type:jsonb" json:"temperatures"`
	MeasuredAt   time.Time `gorm:"not null" json:"measured_at"`
}

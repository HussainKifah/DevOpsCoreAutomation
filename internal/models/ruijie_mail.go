package models

import "time"

type RuijieMailAlert struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	GraphMessageID string    `gorm:"size:256;not null;uniqueIndex"`
	ReceivedAtUTC  time.Time `gorm:"not null;index"`
	Subject        string    `gorm:"size:512"`
	From           string    `gorm:"size:512"`
	BodyText       string    `gorm:"type:text"`
	AlarmSource    string    `gorm:"size:256;index"`
	AlarmType      string    `gorm:"size:256;index"`
	AlarmLevel     string    `gorm:"size:128"`

	DedupFingerprint string `gorm:"size:64;index"`
	SlackIncidentID  *uint  `gorm:"index"`
}

type RuijieSlackIncident struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ChannelID string `gorm:"size:32;uniqueIndex:ux_ruijie_slack_incident_ch_ts,priority:1"`
	MessageTS string `gorm:"size:32;uniqueIndex:ux_ruijie_slack_incident_ch_ts,priority:2"`

	AlarmSource      string `gorm:"size:256;index"`
	AlarmType        string `gorm:"size:256;index"`
	AlarmLevel       string `gorm:"size:128"`
	DedupFingerprint string `gorm:"size:64;index"`

	ResolvedAt *time.Time
	ResolvedBy string     `gorm:"size:256"`
	SnoozedAt  *time.Time `gorm:"index"`
	SnoozedBy  string     `gorm:"size:256"`

	NextReminderAt time.Time `gorm:"index"`
}

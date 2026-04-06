package models

import "time"

// EsSyslogSlackIncident is one Slack parent message (possibly bundling several alerts for one device).
type EsSyslogSlackIncident struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	DeviceKey         string `gorm:"size:512;index"`
	DedupFingerprint  string `gorm:"size:64;index"`
	ChannelID         string `gorm:"size:32;uniqueIndex:ux_slack_incident_ch_ts,priority:1"`
	MessageTS         string `gorm:"size:32;uniqueIndex:ux_slack_incident_ch_ts,priority:2"`
	ResolvedAt *time.Time
	ResolvedBy string `gorm:"size:256"` // Slack @display or user id

	// NextReminderAt is when to post the next thread reminder (open incidents only).
	NextReminderAt time.Time `gorm:"index"`
}

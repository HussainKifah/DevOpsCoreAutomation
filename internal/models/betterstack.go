package models

import "time"

type BetterStackSlackIncident struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	BetterStackIncidentID string `gorm:"size:64;not null;uniqueIndex:ux_betterstack_incident_channel,priority:1"`
	ChannelID             string `gorm:"size:32;not null;uniqueIndex:ux_betterstack_incident_channel,priority:2;uniqueIndex:ux_betterstack_slack_ch_ts,priority:1"`
	MessageTS             string `gorm:"size:32;not null;uniqueIndex:ux_betterstack_slack_ch_ts,priority:2"`

	Name       string `gorm:"size:512"`
	Cause      string `gorm:"size:1024"`
	URL        string `gorm:"size:2048"`
	OriginURL  string `gorm:"size:2048"`
	Status     string `gorm:"size:128;index"`
	TeamName   string `gorm:"size:256"`
	ResolvedBy string `gorm:"size:256"`

	StartedAtUTC      time.Time `gorm:"index"`
	AcknowledgedAtUTC *time.Time
	ResolvedAtUTC     *time.Time `gorm:"index"`
	LastSeenAtUTC     time.Time  `gorm:"index"`
	NextReminderAt    time.Time  `gorm:"index"`
}

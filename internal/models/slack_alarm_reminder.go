package models

import "time"

// SlackAlarmReminder is a generic Slack thread reminder (not tied to syslog incidents).
type SlackAlarmReminder struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ChannelID string `gorm:"size:32;index"`
	// ThreadTS is the parent message timestamp; replies use chat.postMessage with thread_ts.
	ThreadTS string `gorm:"size:32;index"`
	Note     string `gorm:"size:512"`

	// TeamMention is optional mrkdwn (e.g. <!subteam^S...|group>); empty uses server default from config.
	TeamMention string `gorm:"size:256"`

	ReminderIntervalNanos int64
	NextReminderAt        time.Time `gorm:"index"`
	ResolvedAt            *time.Time
}

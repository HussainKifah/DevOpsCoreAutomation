package models

import "time"

// SlackTicketReminder tracks one Slack ticket message until a checkmark reaction resolves it.
// MessageTS is always the ticket line’s own timestamp (for DB identity and UpdateMessage).
// ThreadRootTS is the parent thread ts when the ticket was posted as a thread reply; empty means top-level (use MessageTS for thread APIs).
type SlackTicketReminder struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	ChannelID    string `gorm:"size:32;uniqueIndex:ux_slack_ticket_ch_ts,priority:1"`
	MessageTS    string `gorm:"size:32;uniqueIndex:ux_slack_ticket_ch_ts,priority:2"`
	ThreadRootTS string `gorm:"size:32"`

	MessageText string `gorm:"type:text"`
	TicketTitle string `gorm:"size:255"`
	Province    string `gorm:"size:255"`
	TicketType  string `gorm:"size:255"`
	ToTeam      string `gorm:"size:255"`
	Sender      string `gorm:"size:255"`

	LastReplyTS       string `gorm:"size:32"`
	LastReplyUserID   string `gorm:"size:64"`
	LastReplyUserName string `gorm:"size:255"`
	LastReplyText     string `gorm:"type:text"`
	LastReminderAt    *time.Time
	NextReminderAt    time.Time `gorm:"index"`

	ResolvedAt *time.Time
	ResolvedBy string `gorm:"size:255"`
}

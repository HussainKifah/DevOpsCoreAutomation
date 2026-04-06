package models

import (
	"time"
)

// EsSyslogFilter defines a match on the log message field; poller runs one ES search per enabled filter per tick.
type EsSyslogFilter struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Label     string `gorm:"size:200" json:"label"`
	QueryText string `gorm:"size:2000;not null" json:"query_text"` // terms matched against `message` (match query)
	SortOrder int    `gorm:"default:0;index" json:"sort_order"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
}

// EsSyslogAlert is a deduplicated hit from Elasticsearch (unique es_index + es_doc_id). Hard-deleted after retention.
type EsSyslogAlert struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	EsIndex      string    `gorm:"size:200;not null;uniqueIndex:ux_es_syslog_doc,priority:1" json:"es_index"`
	EsDocID      string    `gorm:"size:200;not null;uniqueIndex:ux_es_syslog_doc,priority:2" json:"es_doc_id"`
	TimestampUTC time.Time `gorm:"not null;index" json:"timestamp_utc"`
	Host         string    `gorm:"size:100" json:"host"`
	DeviceName   string    `gorm:"size:200" json:"device_name"`
	Message      string    `gorm:"type:text" json:"message"`
	FilterID     uint      `gorm:"index" json:"filter_id"`
	FilterLabel  string    `gorm:"size:200" json:"filter_label"`
	// SHA256 hex (64); same host+device+normalized-message within dedup window → skip insert
	DedupFingerprint string `gorm:"size:64;index" json:"-"`
	// Slack: optional link to EsSyslogSlackIncident when posted to Slack
	SlackIncidentID *uint `gorm:"index" json:"-"`
}

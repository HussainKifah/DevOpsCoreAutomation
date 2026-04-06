package config

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	DBSSLMode  string

	ServerPort string
	JWTSecret  string
	TLSCertFile string
	TLSKeyFile string

	OLTUser string // Nokia SSH credentials
	OLTPass string

	HuaweiOLTUser string // Huawei SSH credentials (falls back to OLTUser if empty)
	HuaweiOLTPass string

	// Elasticsearch (IP syslog alerts). Empty URL disables polling.
	ElasticsearchURL           string
	ElasticsearchUser        string
	ElasticsearchPassword    string
	ElasticsearchSkipTLSVerify bool
	ElasticsearchIndexPattern string // e.g. logstash-*
	EsSyslogPollInterval      time.Duration
	EsSyslogRetentionDays     int // hard-delete alerts older than this (default 30)
	EsSyslogDedupWindow       time.Duration // suppress same host+device+normalized message within this window (default 1h)

	// Slack syslog alerts (optional). Requires SLACK_BOT_TOKEN + SLACK_CHANNEL_ID and Events URL for "fixed"/"done".
	SlackSyslogEnabled       bool
	SlackBotToken            string
	SlackChannelID           string
	SlackSigningSecret       string
	SlackReminderInterval    time.Duration // default 12h
	SlackSyslogBatchWindow   time.Duration // coalesce alerts per device before post; default 45s
	// SlackSyslogDisplayOffset shifts stored UTC timestamps shown in Slack (e.g. 3h for UTC+3).
	SlackSyslogDisplayOffset time.Duration
	// SlackSyslogTeamMention is raw mrkdwn for pings, e.g. <!subteam^S01234|ip-core> (from Slack user group).
	SlackSyslogTeamMention string

	// SlackAlarmsReminderEnabled runs generic thread reminders (SlackAlarmReminder table); uses SLACK_BOT_TOKEN.
	SlackAlarmsReminderEnabled bool
	// SlackAlarmsDefaultTeamMention is used when a reminder row has no TeamMention set.
	SlackAlarmsDefaultTeamMention string
	// SlackAlarmsTickInterval is how often the worker polls for due reminders (default 2m).
	SlackAlarmsTickInterval time.Duration

	PowerScanInterval  time.Duration
	HealthScanInterval time.Duration
	DescScanInterval   time.Duration
	PortScanInterval   time.Duration
	BackupInterval     time.Duration
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		log.Println("INFO: no .env file found, using environment variables")
	}

	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "hussain"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "devopscore"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		ServerPort: getEnv("PORT", "8080"),
		JWTSecret:  getEnv("JWT_SECRET", ""),
		TLSCertFile: getEnv("TLS_CERT", ""),
		TLSKeyFile: getEnv("TLS_KEY", ""),

		OLTUser: getEnv("OLT_SSH_USER", ""),
		OLTPass: getEnv("OLT_SSH_PASS", ""),

		HuaweiOLTUser: getEnv("HW_SSH_USER", getEnv("OLT_SSH_USER", "")),
		HuaweiOLTPass: getEnv("HW_SSH_PASS", getEnv("OLT_SSH_PASS", "")),

		ElasticsearchURL:            getEnv("ELASTICSEARCH_URL", ""),
		ElasticsearchUser:         getEnv("ELASTICSEARCH_USER", ""),
		ElasticsearchPassword:     getEnv("ELASTICSEARCH_PASSWORD", ""),
		ElasticsearchSkipTLSVerify: getEnv("ELASTICSEARCH_SKIP_TLS_VERIFY", "") == "true" || getEnv("ELASTICSEARCH_SKIP_TLS_VERIFY", "") == "1",
		ElasticsearchIndexPattern: getEnv("ELASTICSEARCH_INDEX_PATTERN", "logstash-*"),
		EsSyslogPollInterval:      parseDuration(getEnv("ES_SYSLOG_POLL_INTERVAL", "1m")),
		EsSyslogRetentionDays:     parseRetentionDays("ES_SYSLOG_RETENTION_DAYS", 30),
		EsSyslogDedupWindow:       parseDuration(getEnv("ES_SYSLOG_DEDUP_WINDOW", "1h")),

		SlackSyslogEnabled:     getEnv("SLACK_SYSLOG_ENABLED", "") == "1" || getEnv("SLACK_SYSLOG_ENABLED", "") == "true",
		SlackBotToken:          getEnv("SLACK_BOT_TOKEN", ""),
		SlackChannelID:         strings.TrimSpace(getEnv("SLACK_CHANNEL_ID", "")),
		SlackSigningSecret:     getEnv("SLACK_SIGNING_SECRET", ""),
		SlackReminderInterval:  parseDurationWithFallback(getEnv("SLACK_REMINDER_INTERVAL", "6h"), 6*time.Hour),
		SlackSyslogBatchWindow: parseDurationWithFallback(getEnv("SLACK_SYSLOG_BATCH_WINDOW", "45s"), 45*time.Second),
		SlackSyslogDisplayOffset: parseDurationWithFallback(getEnv("SLACK_SYSLOG_DISPLAY_OFFSET", "3h"), 3*time.Hour),
		SlackSyslogTeamMention:   strings.TrimSpace(getEnv("SLACK_SYSLOG_TEAM_MENTION", "")),

		SlackAlarmsReminderEnabled:     getEnv("SLACK_ALARMS_REMINDER_ENABLED", "") == "1" || getEnv("SLACK_ALARMS_REMINDER_ENABLED", "") == "true",
		SlackAlarmsDefaultTeamMention: strings.TrimSpace(getEnv("SLACK_ALARMS_DEFAULT_TEAM_MENTION", "")),
		SlackAlarmsTickInterval:       parseDurationWithFallback(getEnv("SLACK_ALARMS_TICK_INTERVAL", "2m"), 2*time.Minute),

		PowerScanInterval:  parseDuration(getEnv("POWER_SCAN_INTERVAL", "6h")),
		HealthScanInterval: parseDuration(getEnv("HEALTH_SCAN_INTERVAL", "0.5h")),
		DescScanInterval:   parseDuration(getEnv("DESC_SCAN_INTERVAL", "6h")),
		PortScanInterval:   parseDuration(getEnv("PORT_SCAN_INTERVAL", "0.5h")),
		BackupInterval:     parseDuration(getEnv("BACKUP_INTERVAL", "24h")),
	}
}

func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" port=" + c.DBPort +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" sslmode=" + c.DBSSLMode
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Hour
	}
	return d
}

// SlackSyslogConfigured is true when posting to Slack should run (token + channel).
func (c *Config) SlackSyslogConfigured() bool {
	return c != nil && c.SlackSyslogEnabled && c.SlackBotToken != "" && c.SlackChannelID != ""
}

// SlackAlarmsReminderConfigured is true when the generic Slack reminder worker should run.
func (c *Config) SlackAlarmsReminderConfigured() bool {
	return c != nil && c.SlackAlarmsReminderEnabled && c.SlackBotToken != ""
}

func parseDurationWithFallback(s string, fallback time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < time.Second {
		return fallback
	}
	return d
}

func parseRetentionDays(key string, fallback int) int {
	v := strings.TrimSpace(getEnv(key, ""))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 3650 {
		return fallback
	}
	return n
}

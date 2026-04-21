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
	DBHost       string
	DBPort       string
	DBUser       string
	DBPassword   string
	DBName       string
	DBSSLMode    string
	GormLogLevel string

	ServerPort  string
	JWTSecret   string
	TLSCertFile string
	TLSKeyFile  string

	OLTUser string // Nokia SSH credentials
	OLTPass string

	HuaweiOLTUser string // Huawei SSH credentials (falls back to OLTUser if empty)
	HuaweiOLTPass string

	NocDataCiscoUser    string
	NocDataCiscoPass    string
	NocDataMikrotikUser string
	NocDataMikrotikPass string
	NocDataWorkers      int
	NocDataCommandGap   time.Duration
	NocDataHeavyCmdGap  time.Duration

	// Elasticsearch (IP syslog alerts). Empty URL disables polling.
	ElasticsearchURL           string
	ElasticsearchUser          string
	ElasticsearchPassword      string
	ElasticsearchSkipTLSVerify bool
	ElasticsearchIndexPattern  string // e.g. logstash-*
	EsSyslogPollInterval       time.Duration
	EsSyslogRetentionDays      int           // hard-delete alerts older than this (default 30)
	EsSyslogDedupWindow        time.Duration // suppress same host+device+normalized message within this window (default 1h)

	// Slack syslog alerts (optional). Requires SLACK_BOT_TOKEN + SLACK_CHANNEL_ID and Events URL for "fixed"/"done".
	SlackSyslogEnabled     bool
	SlackBotToken          string
	SlackChannelID         string
	SlackSigningSecret     string
	SlackReminderInterval  time.Duration // default 12h
	SlackSyslogBatchWindow time.Duration // coalesce alerts per device before post; default 45s
	// SlackSyslogDisplayOffset shifts stored UTC timestamps shown in Slack (e.g. 3h for UTC+3).
	SlackSyslogDisplayOffset time.Duration
	// SlackSyslogTeamMention is raw mrkdwn for pings, e.g. <!subteam^S01234|ip-core> (from Slack user group).
	SlackSyslogTeamMention string

	SlackTicketReminderEnabled    bool
	SlackTicketChannelID          string
	SlackTicketReminderInterval   time.Duration
	SlackTicketFirstReminderAfter time.Duration
	SlackTicketTickInterval       time.Duration
	SlackTicketIPTeamMention      string
	SlackTicketDisplayOffset      time.Duration
	SlackTicketSourceBotID        string

	RuijieMailEnabled           bool
	RuijieMailTenantID          string
	RuijieMailClientID          string
	RuijieMailClientSecret      string
	RuijieMailUserID            string
	RuijieMailFolderID          string
	RuijieMailSubject           string
	RuijieMailPollInterval      time.Duration
	RuijieMailLookback          time.Duration
	RuijieSlackChannelID        string
	RuijieSlackTeamMention      string
	RuijieSlackReminderInterval time.Duration
	RuijieSlackDisplayOffset    time.Duration

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
		DBHost:       getEnv("DB_HOST", "localhost"),
		DBPort:       getEnv("DB_PORT", "5432"),
		DBUser:       getEnv("DB_USER", "hussain"),
		DBPassword:   getEnv("DB_PASSWORD", ""),
		DBName:       getEnv("DB_NAME", "devopscore"),
		DBSSLMode:    getEnv("DB_SSLMODE", "disable"),
		GormLogLevel: strings.TrimSpace(getEnv("GORM_LOG_LEVEL", "warn")),

		ServerPort:  getEnv("PORT", "8080"),
		JWTSecret:   getEnv("JWT_SECRET", ""),
		TLSCertFile: getEnv("TLS_CERT", ""),
		TLSKeyFile:  getEnv("TLS_KEY", ""),

		OLTUser: getEnv("OLT_SSH_USER", ""),
		OLTPass: getEnv("OLT_SSH_PASS", ""),

		HuaweiOLTUser: getEnv("HW_SSH_USER", getEnv("OLT_SSH_USER", "")),
		HuaweiOLTPass: getEnv("HW_SSH_PASS", getEnv("OLT_SSH_PASS", "")),

		NocDataCiscoUser:    strings.TrimSpace(getEnv("NOC_DATA_CISCO_USER", "")),
		NocDataCiscoPass:    getEnv("NOC_DATA_CISCO_PASS", ""),
		NocDataMikrotikUser: strings.TrimSpace(getEnv("NOC_DATA_MIKROTIK_USER", "")),
		NocDataMikrotikPass: getEnv("NOC_DATA_MIKROTIK_PASS", ""),
		NocDataWorkers:      parseBoundedInt("NOC_DATA_WORKERS", 2, 1, 32),
		NocDataCommandGap:   parseDurationBounded(getEnv("NOC_DATA_CMD_GAP", "250ms"), 250*time.Millisecond, 0, 30*time.Second),
		NocDataHeavyCmdGap:  parseDurationBounded(getEnv("NOC_DATA_HEAVY_CMD_GAP", "750ms"), 750*time.Millisecond, 0, 30*time.Second),

		ElasticsearchURL:           getEnv("ELASTICSEARCH_URL", ""),
		ElasticsearchUser:          getEnv("ELASTICSEARCH_USER", ""),
		ElasticsearchPassword:      getEnv("ELASTICSEARCH_PASSWORD", ""),
		ElasticsearchSkipTLSVerify: getEnv("ELASTICSEARCH_SKIP_TLS_VERIFY", "") == "true" || getEnv("ELASTICSEARCH_SKIP_TLS_VERIFY", "") == "1",
		ElasticsearchIndexPattern:  getEnv("ELASTICSEARCH_INDEX_PATTERN", "logstash-*"),
		EsSyslogPollInterval:       parseDuration(getEnv("ES_SYSLOG_POLL_INTERVAL", "1m")),
		EsSyslogRetentionDays:      parseRetentionDays("ES_SYSLOG_RETENTION_DAYS", 30),
		EsSyslogDedupWindow:        parseDuration(getEnv("ES_SYSLOG_DEDUP_WINDOW", "1h")),

		SlackSyslogEnabled:       getEnv("SLACK_SYSLOG_ENABLED", "") == "1" || getEnv("SLACK_SYSLOG_ENABLED", "") == "true",
		SlackBotToken:            getEnv("SLACK_BOT_TOKEN", ""),
		SlackChannelID:           strings.TrimSpace(getEnv("SLACK_CHANNEL_ID", "")),
		SlackSigningSecret:       getEnv("SLACK_SIGNING_SECRET", ""),
		SlackReminderInterval:    parseDurationWithFallback(getEnv("SLACK_REMINDER_INTERVAL", "6h"), 6*time.Hour),
		SlackSyslogBatchWindow:   parseDurationWithFallback(getEnv("SLACK_SYSLOG_BATCH_WINDOW", "45s"), 45*time.Second),
		SlackSyslogDisplayOffset: parseDurationWithFallback(getEnv("SLACK_SYSLOG_DISPLAY_OFFSET", "3h"), 3*time.Hour),
		SlackSyslogTeamMention:   strings.TrimSpace(getEnv("SLACK_SYSLOG_TEAM_MENTION", "")),

		SlackTicketReminderEnabled:    getEnv("SLACK_TICKET_REMINDER_ENABLED", "") == "1" || getEnv("SLACK_TICKET_REMINDER_ENABLED", "") == "true",
		SlackTicketChannelID:          strings.TrimSpace(getEnv("SLACK_TICKET_CHANNEL_ID", getEnv("SLACK_CHANNEL_ID", ""))),
		SlackTicketReminderInterval:   parseDurationWithFallback(getEnv("SLACK_TICKET_REMINDER_INTERVAL", "6h"), 6*time.Hour),
		SlackTicketFirstReminderAfter: parseDurationWithFallback(getEnv("SLACK_TICKET_FIRST_REMINDER_AFTER", "6h"), 6*time.Hour),
		SlackTicketTickInterval:       parseDurationWithFallback(getEnv("SLACK_TICKET_TICK_INTERVAL", "2m"), 2*time.Minute),
		SlackTicketIPTeamMention:      strings.TrimSpace(getEnv("SLACK_TICKET_IP_TEAM_MENTION", "")),
		SlackTicketDisplayOffset:      parseDurationWithFallback(getEnv("SLACK_TICKET_DISPLAY_OFFSET", "3h"), 3*time.Hour),
		SlackTicketSourceBotID:        strings.TrimSpace(getEnv("SLACK_TICKET_SOURCE_BOT_ID", "")),

		RuijieMailEnabled:           getEnv("RUIJIE_MAIL_ENABLED", "") == "1" || getEnv("RUIJIE_MAIL_ENABLED", "") == "true",
		RuijieMailTenantID:          strings.TrimSpace(getEnv("RUIJIE_MAIL_TENANT_ID", "")),
		RuijieMailClientID:          strings.TrimSpace(getEnv("RUIJIE_MAIL_CLIENT_ID", "")),
		RuijieMailClientSecret:      strings.TrimSpace(getEnv("RUIJIE_MAIL_CLIENT_SECRET", "")),
		RuijieMailUserID:            strings.TrimSpace(getEnv("RUIJIE_MAIL_USER_ID", "")),
		RuijieMailFolderID:          strings.TrimSpace(getEnv("RUIJIE_MAIL_FOLDER_ID", "junkemail")),
		RuijieMailSubject:           strings.TrimSpace(getEnv("RUIJIE_MAIL_SUBJECT", "Ruijie Cloud Alarm Notification")),
		RuijieMailPollInterval:      parseDurationWithFallback(getEnv("RUIJIE_MAIL_POLL_INTERVAL", "1m"), time.Minute),
		RuijieMailLookback:          parseDurationWithFallback(getEnv("RUIJIE_MAIL_LOOKBACK", "10m"), 10*time.Minute),
		RuijieSlackChannelID:        strings.TrimSpace(getEnv("RUIJIE_SLACK_CHANNEL_ID", "")),
		RuijieSlackTeamMention:      strings.TrimSpace(getEnv("RUIJIE_SLACK_TEAM_MENTION", getEnv("SLACK_SYSLOG_TEAM_MENTION", ""))),
		RuijieSlackReminderInterval: parseDurationWithFallback(getEnv("RUIJIE_SLACK_REMINDER_INTERVAL", getEnv("SLACK_REMINDER_INTERVAL", "6h")), 6*time.Hour),
		RuijieSlackDisplayOffset:    parseDurationWithFallback(getEnv("RUIJIE_SLACK_DISPLAY_OFFSET", getEnv("SLACK_SYSLOG_DISPLAY_OFFSET", "3h")), 3*time.Hour),

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

func (c *Config) SlackTicketReminderConfigured() bool {
	return c != nil &&
		c.SlackTicketReminderEnabled &&
		c.SlackBotToken != "" &&
		c.SlackSigningSecret != "" &&
		c.SlackTicketChannelID != ""
}

func (c *Config) RuijieMailConfigured() bool {
	return c != nil &&
		c.RuijieMailEnabled &&
		c.RuijieMailTenantID != "" &&
		c.RuijieMailClientID != "" &&
		c.RuijieMailClientSecret != "" &&
		c.RuijieMailUserID != "" &&
		c.SlackBotToken != "" &&
		c.RuijieSlackChannelID != ""
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

func parseBoundedInt(key string, fallback, min, max int) int {
	v := strings.TrimSpace(getEnv(key, ""))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return fallback
	}
	return n
}

func parseDurationBounded(s string, fallback, min, max time.Duration) time.Duration {
	d, err := time.ParseDuration(strings.TrimSpace(s))
	if err != nil || d < min || d > max {
		return fallback
	}
	return d
}

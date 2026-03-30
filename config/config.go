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

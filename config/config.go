package config

import (
	"log"
	"os"
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

	OLTUser string
	OLTPass string

	PowerScanInterval  time.Duration
	HealthScanInterval time.Duration
	DescScanInterval   time.Duration
	PortScanInterval   time.Duration
	BackupInterval     time.Duration
}

func Load() *Config {
	// Try .env in current dir, then parent dirs (for running from cmd/api/)
	if err := godotenv.Load(); err != nil {
		if err2 := godotenv.Load("../../.env"); err2 != nil {
			log.Println("WARN: no .env file found, using environment variables")
		}
	}

	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnv("DB_PORT", "5432"),
		DBUser:     getEnv("DB_USER", "hussain"),
		DBPassword: getEnv("DB_PASSWORD", ""),
		DBName:     getEnv("DB_NAME", "devopscore"),
		DBSSLMode:  getEnv("DB_SSLMODE", "disable"),

		ServerPort: getEnv("PORT", "8080"),

		OLTUser: getEnv("OLT_SSH_USER", ""),
		OLTPass: getEnv("OLT_SSH_PASS", ""),

		PowerScanInterval:  parseDuration(getEnv("POWER_SCAN_INTERVAL", "6h")),
		HealthScanInterval: parseDuration(getEnv("HEALTH_SCAN_INTERVAL", "1h")),
		DescScanInterval:   parseDuration(getEnv("DESC_SCAN_INTERVAL", "6h")),
		PortScanInterval:   parseDuration(getEnv("PORT_SCAN_INTERVAL", "2h")),
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

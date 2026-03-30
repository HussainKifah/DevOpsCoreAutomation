package db

import (
	"log"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func Connect(cfg *config.Config) *gorm.DB {
	db, err := gorm.Open(postgres.Open(cfg.DSN()), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatalf("failed to get underlying sql.DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := db.AutoMigrate(
		&models.PowerReading{},
		&models.OntDescription{},
		&models.OltHealth{},
		&models.PortProtectionRecord{},
		&models.OltBackups{},
		&models.User{},
		&models.HealthSnapshot{},
		&models.PortSnapshot{},
		&models.InventorySummary{},
		&models.OltInventory{},
		&models.OntInventoryItem{},
		&models.WorkflowDevice{},
		&models.WorkflowJob{},
		&models.WorkflowRun{},
		&models.WorkflowLog{},
		&models.NocPassDevice{},
		&models.EsSyslogFilter{},
		&models.EsSyslogAlert{},
	); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	go func() {
		indexes := []string{
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_hs_measured ON health_snapshots (measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_hs_host_measured ON health_snapshots (host, measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ps_measured ON port_snapshots (measured_at)",
			"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ps_host_measured ON port_snapshots (host, measured_at)",
		}
		for _, ddl := range indexes {
			if err := db.Exec(ddl).Error; err != nil {
				log.Printf("index warning: %v", err)
			}
		}
		log.Println("database indexes ensured")
	}()

	log.Println("database connected and migrated")
	return db
}

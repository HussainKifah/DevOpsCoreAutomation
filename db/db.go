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
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetConnMaxIdleTime(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)

	if err := db.AutoMigrate(
		&models.PowerReading{},
		&models.OntDescription{},
		&models.OltHealth{},
		&models.PortProtectionRecord{},
		&models.OltBackups{},
		&models.User{},
	); err != nil {
		log.Fatalf("failed to migrate database: %v", err)
	}

	log.Println("database connected and migrated")
	return db
}

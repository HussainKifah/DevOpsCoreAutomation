package main

import (
	"log"
	"path/filepath"
	"runtime"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/db"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/router"
	"github.com/Flafl/DevOpsCore/internal/scheduler"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	database := db.Connect(cfg)

	powerRepo := repository.NewPowerRepository(database)
	descRepo := repository.NewDescriptionRepository(database)
	healthRepo := repository.NewHealthRepository(database)
	portRepo := repository.NewPortProtectionRepo(database)
	backupRepo := repository.NewBackupRepository(database)
	userRepo := repository.NewUserRepository(database)

	sched := scheduler.New(cfg, powerRepo, descRepo, healthRepo, portRepo, backupRepo)
	sched.Start()

	server := gin.Default()

	powerH := handlers.NewPowerHandler(powerRepo)
	descH := handlers.NewDescriptionHandler(descRepo)
	healthH := handlers.NewHealthHandler(healthRepo)
	portH := handlers.NewPortHandler(portRepo)
	backupH := handlers.NewBackupHandler(backupRepo)
	userH := handlers.NewUserHandler(userRepo)

	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	pageH := handlers.NewPageHandler(filepath.Join(projectRoot, "templates"))

	router.Setup(server, powerH, descH, healthH, portH, backupH, userH, pageH)

	log.Printf("server starting on :%s", cfg.ServerPort)
	if err := server.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

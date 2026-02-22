package main

import (
	"log"

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

	powerRepo := repository.NewPowerRepo(database)
	descRepo := repository.NewDescriptionRepo(database)
	healthRepo := repository.NewHealthRepo(database)
	portRepo := repository.NewPortProtectionRepo(database)

	sched := scheduler.New(cfg, powerRepo, descRepo, healthRepo, portRepo)
	sched.Start()

	server := gin.Default()

	powerH := handlers.NewPowerHandler(powerRepo)
	descH := handlers.NewDescriptionHandler(descRepo)
	healthH := handlers.NewHealthHandler(healthRepo)
	portH := handlers.NewPortHandler(portRepo)

	router.Setup(server, powerH, descH, healthH, portH)

	log.Printf("server starting on :%s", cfg.ServerPort)
	if err := server.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}

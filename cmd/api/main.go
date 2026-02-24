package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/db"
	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/router"
	"github.com/Flafl/DevOpsCore/internal/scheduler"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
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

	jwtManager := auth.NewJWTManager(auth.JWTconfig{
		SecretKey:            []byte(cfg.JWTSecret),
		AccessTokenDuration:  24 * time.Hour,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "devopscore",
	})

	hub := websocket.NewHub()
	go hub.Run()

	sched := scheduler.New(cfg, hub, powerRepo, descRepo, healthRepo, portRepo, backupRepo)
	sched.Start()

	server := gin.Default()

	_, thisFile, _, _ := runtime.Caller(0)
	projectRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	server.Static("/static", filepath.Join(projectRoot, "templates", "static"))

	powerH := handlers.NewPowerHandler(powerRepo)
	descH := handlers.NewDescriptionHandler(descRepo)
	healthH := handlers.NewHealthHandler(healthRepo)
	portH := handlers.NewPortHandler(portRepo)
	backupH := handlers.NewBackupHandler(backupRepo)
	userH := handlers.NewUserHandler(userRepo)
	authH := handlers.NewAuthHandler(userRepo, jwtManager)

	pageH := handlers.NewPageHandler(filepath.Join(projectRoot, "templates"))

	router.Setup(server, jwtManager, hub, powerH, descH, healthH, portH, backupH, userH, authH, pageH)

	// Graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: server,
	}
	go func() {
		log.Printf("server starting on :%s", cfg.ServerPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutdown signal received, stopping...")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server stopped cleanly")

}

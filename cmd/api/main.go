package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/db"
	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/router"
	"github.com/Flafl/DevOpsCore/internal/scheduler"
	"github.com/Flafl/DevOpsCore/internal/shell"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	database := db.Connect(cfg)

	powerRepo := repository.NewPowerRepository(database)
	descRepo := repository.NewDescriptionRepository(database)
	healthRepo := repository.NewHealthRepository(database)
	historyRepo := repository.NewHealthHistoryRepository(database)
	portRepo := repository.NewPortProtectionRepo(database)
	portHistRepo := repository.NewPortHistoryRepository(database)
	backupRepo := repository.NewBackupRepository(database)
	userRepo := repository.NewUserRepository(database)
	inventoryRepo := repository.NewInventoryRepo(database)
	workflowRepo := repository.NewWorkflowRepository(database)

	jwtManager := auth.NewJWTManager(auth.JWTconfig{
		SecretKey:            []byte(cfg.JWTSecret),
		AccessTokenDuration:  24 * time.Hour,
		RefreshTokenDuration: 7 * 24 * time.Hour,
		Issuer:               "devopscore",
	})

	hub := websocket.NewHub()
	go hub.Run()

	sshPool := shell.NewConnectionPool(cfg.OLTUser, cfg.OLTPass)

	sched := scheduler.New(cfg, hub, sshPool, powerRepo, descRepo, healthRepo, historyRepo, portRepo, portHistRepo, backupRepo, inventoryRepo, database)
	sched.Start()

	cryptoKey := []byte(cfg.JWTSecret)
	wfSched, err := scheduler.NewWorkflowScheduler(workflowRepo, cryptoKey)
	if err != nil {
		log.Fatalf("failed to create workflow scheduler: %v", err)
	}
	wfSched.Start()

	server := gin.Default()

	projectRoot := os.Getenv("APP_ROOT")
	if projectRoot == "" {
		exe, err := os.Executable()
		if err == nil {
			projectRoot = filepath.Dir(exe)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, "templates")); err != nil {
			projectRoot, _ = os.Getwd()
		}
	}
	server.Static("/static", filepath.Join(projectRoot, "templates", "static"))

	powerH := handlers.NewPowerHandler(powerRepo)
	descH := handlers.NewDescriptionHandler(descRepo)
	healthH := handlers.NewHealthHandler(healthRepo)
	healthHistoryH := handlers.NewHealthHistoryHandler(historyRepo)
	portH := handlers.NewPortHandler(portRepo)
	portHistoryH := handlers.NewPortHistoryHandler(portHistRepo)
	calendarH := handlers.NewHistoryCalendarHandler(historyRepo, portHistRepo)
	backupH := handlers.NewBackupHandler(backupRepo)
	userH := handlers.NewUserHandler(userRepo)
	authH := handlers.NewAuthHandler(userRepo, jwtManager)
	inventoryH := handlers.NewInventoryHandler(inventoryRepo)
	scanH := handlers.NewScanHandler(sched)
	workflowH := handlers.NewWorkflowHandler(workflowRepo, wfSched, cryptoKey)

	pageH := handlers.NewPageHandler(filepath.Join(projectRoot, "templates"), userRepo)

	router.Setup(server, jwtManager, hub, powerH, descH, healthH, healthHistoryH, portH, portHistoryH, calendarH, backupH, userH, authH, pageH, inventoryH, scanH, workflowH)

	// Graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.ServerPort,
		Handler: server,
	}
	go func() {
		addr := ":" + cfg.ServerPort
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			log.Printf("server starting HTTPS on %s", addr)
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server failed: %v", err)
			}
		} else {

			log.Printf("server starting HTTP on %s", addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("server failed: %v", err)

			}


		}
	}()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutdown signal received, stopping...")

	sched.FlushHealthBuffer()
	wfSched.Stop()
	sshPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server stopped cleanly")

}

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
	slackalarms "github.com/Flafl/DevOpsCore/internal/SlackRemindersAndAlarms"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/internal/syslog"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
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
	nocPassRepo := repository.NewNocPassRepository(database)

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

	nocPassRotator := scheduler.NewNocPassRotator(nocPassRepo, cryptoKey)
	nocPassRotator.Start()

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
	nocPassH := handlers.NewNocPassHandler(nocPassRepo, cryptoKey)
	esSyslogRepo := repository.NewEsSyslogRepository(database)
	esSyslogH := handlers.NewEsSyslogHandler(esSyslogRepo)

	var slackAPI *slack.Client
	var slackBatcher *syslog.SlackSyslogBatcher
	var slackReminder *syslog.SlackReminderWorker
	var slackAlarmsWorker *slackalarms.Worker
	var slackAlarmsH *slackalarms.Handler
	var slackEventsH *handlers.SlackEventsHandler
	if cfg.SlackSyslogConfigured() {
		slackAPI = slack.New(cfg.SlackBotToken)
		slackBatcher = syslog.NewSlackSyslogBatcher(cfg, esSyslogRepo, slackAPI)
		slackBatcher.Start()
		slackReminder = syslog.NewSlackReminderWorker(cfg, esSyslogRepo, slackAPI)
		slackReminder.Start()
		log.Printf("[slack-syslog] enabled channel=%s batch=%s reminder=%s", cfg.SlackChannelID, cfg.SlackSyslogBatchWindow, cfg.SlackReminderInterval)
	}
	if cfg.SlackBotToken != "" && cfg.SlackSigningSecret != "" {
		api := slackAPI
		if api == nil {
			api = slack.New(cfg.SlackBotToken)
		}
		slackEventsH = handlers.NewSlackEventsHandler(cfg, esSyslogRepo, api)
	}

	if cfg.SlackAlarmsReminderConfigured() {
		saStore := slackalarms.NewStore(database)
		slackAlarmsH = slackalarms.NewHandler(saStore)
		api := slackAPI
		if api == nil {
			api = slack.New(cfg.SlackBotToken)
		}
		slackAlarmsWorker = slackalarms.NewWorker(cfg, saStore, api)
		slackAlarmsWorker.Start()
		log.Printf("[slack-alarms] generic reminders enabled tick=%s", cfg.SlackAlarmsTickInterval)
	}

	pageH := handlers.NewPageHandler(filepath.Join(projectRoot, "templates"), userRepo, jwtManager)

	router.Setup(server, jwtManager, hub, powerH, descH, healthH, healthHistoryH, portH, portHistoryH, calendarH, backupH, userH, authH, pageH, inventoryH, scanH, workflowH, nocPassH, esSyslogH, slackEventsH, slackAlarmsH)

	esSyslogPoller := scheduler.NewEsSyslogPoller(cfg, esSyslogRepo, slackBatcher)
	esSyslogPoller.Start()

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
	nocPassRotator.Stop()
	esSyslogPoller.Stop()
	if slackReminder != nil {
		slackReminder.Stop()
	}
	if slackAlarmsWorker != nil {
		slackAlarmsWorker.Stop()
	}
	sshPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server stopped cleanly")

}

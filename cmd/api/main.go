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
	ruijie "github.com/Flafl/DevOpsCore/internal/Ruijie"
	slackreminders "github.com/Flafl/DevOpsCore/internal/SlackReminders"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/router"
	"github.com/Flafl/DevOpsCore/internal/scheduler"
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
	nocWorkflowRepo := repository.NewWorkflowRepositoryForScope(database, "noc")
	nocPassRepo := repository.NewNocPassRepository(database)
	nocDataRepo := repository.NewNocDataRepository(database)
	slackTicketRepo := repository.NewSlackTicketReminderRepository(database)
	ruijieMailRepo := repository.NewRuijieMailRepository(database)
	ipCapacityRepo := repository.NewIPCapacityRepository(database)

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
	nocWfSched, err := scheduler.NewWorkflowSchedulerForScope(nocWorkflowRepo, cryptoKey, "noc", nocDataRepo)
	if err != nil {
		log.Fatalf("failed to create NOC workflow scheduler: %v", err)
	}
	nocWfSched.Start()

	nocPassRotator := scheduler.NewNocPassRotator(nocPassRepo, nocDataRepo, cryptoKey)
	nocPassRotator.Start()
	nocDataCollector := scheduler.NewNocDataCollector(nocDataRepo, cryptoKey, cfg)
	nocDataCollector.Start()

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
	nocWorkflowH := handlers.NewNocWorkflowHandler(nocWorkflowRepo, nocWfSched, cryptoKey, nocDataRepo)
	nocPassH := handlers.NewNocPassHandler(nocPassRepo, nocDataRepo, cryptoKey)
	nocDataH := handlers.NewNocDataHandler(nocDataRepo, cryptoKey, nocDataCollector, cfg)
	esSyslogRepo := repository.NewEsSyslogRepository(database)
	esSyslogH := handlers.NewEsSyslogHandler(esSyslogRepo)
	ipCapacityH := handlers.NewIPCapacityHandler(ipCapacityRepo)

	var slackAPI *slack.Client
	var slackBatcher *syslog.SlackSyslogBatcher
	var slackReminder *syslog.SlackReminderWorker
	var slackTicketWorker *slackreminders.Worker
	var slackActivityLogWorker *scheduler.ActivityLogSlackWorker
	var ruijieMailPoller *ruijie.MailPoller
	var ruijieReminder *ruijie.ReminderWorker
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
		if slackAPI == nil {
			slackAPI = slack.New(cfg.SlackBotToken)
		}
		slackEventsH = handlers.NewSlackEventsHandler(cfg, esSyslogRepo, slackTicketRepo, ruijieMailRepo, slackAPI)
	}
	if cfg.SlackTicketReminderConfigured() {
		if slackAPI == nil {
			slackAPI = slack.New(cfg.SlackBotToken)
		}
		slackTicketWorker = slackreminders.NewWorker(cfg, slackTicketRepo, slackAPI)
		slackTicketWorker.Start()
		log.Printf("[slack-ticket-reminders] enabled channel=%s every=%s", cfg.SlackTicketChannelID, cfg.SlackTicketReminderInterval)
	}
	if cfg.SlackActivityLogConfigured() {
		if slackAPI == nil {
			slackAPI = slack.New(cfg.SlackBotToken)
		}
		slackActivityLogWorker = scheduler.NewActivityLogSlackWorker(cfg, workflowRepo, slackAPI)
		slackActivityLogWorker.Start()
	}
	if cfg.RuijieMailConfigured() {
		if slackAPI == nil {
			slackAPI = slack.New(cfg.SlackBotToken)
		}
		ruijieMailPoller = ruijie.NewMailPoller(cfg, ruijieMailRepo, slackAPI)
		ruijieMailPoller.Start()
		ruijieReminder = ruijie.NewReminderWorker(cfg, ruijieMailRepo, slackAPI)
		ruijieReminder.Start()
		log.Printf("[ruijie-mail] enabled channel=%s every=%s", cfg.RuijieSlackChannelID, cfg.RuijieSlackReminderInterval)
	}

	pageH := handlers.NewPageHandler(filepath.Join(projectRoot, "templates"), userRepo, jwtManager)

	router.Setup(server, jwtManager, hub, powerH, descH, healthH, healthHistoryH, portH, portHistoryH, calendarH, backupH, userH, authH, pageH, inventoryH, scanH, workflowH, nocWorkflowH, nocPassH, nocDataH, esSyslogH, ipCapacityH, slackEventsH)

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
	nocWfSched.Stop()
	nocPassRotator.Stop()
	nocDataCollector.Stop()
	esSyslogPoller.Stop()
	if slackReminder != nil {
		slackReminder.Stop()
	}
	if slackTicketWorker != nil {
		slackTicketWorker.Stop()
	}
	if slackActivityLogWorker != nil {
		slackActivityLogWorker.Stop()
	}
	if ruijieMailPoller != nil {
		ruijieMailPoller.Stop()
	}
	if ruijieReminder != nil {
		ruijieReminder.Stop()
	}
	sshPool.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("server forced to shutdown: %v", err)
	}

	log.Println("server stopped cleanly")

}

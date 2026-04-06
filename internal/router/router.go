package router

import (
	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/middleware"
	slackalarms "github.com/Flafl/DevOpsCore/internal/SlackRemindersAndAlarms"
	websocket "github.com/Flafl/DevOpsCore/internal/webSocket"
	"github.com/gin-gonic/gin"
)

func Setup(
	r *gin.Engine,
	jwtManager *auth.JWTManager,
	hub *websocket.Hub,
	powerH *handlers.PowerHandler,
	descH *handlers.DescriptionHandler,
	healthH *handlers.HealthHandler,
	healthHistoryH *handlers.HealthHistoryHandler,
	portH *handlers.PortHandler,
	portHistoryH *handlers.PortHistoryHandler,
	calendarH *handlers.HistoryCalendarHandler,
	backupH *handlers.BackupHandler,
	userH *handlers.UserhHandler,
	authH *handlers.AuthHandler,
	pageH *handlers.PageHandler,
	inventoryH *handlers.InventoryHandler,
	scanH *handlers.ScanHandler,
	workflowH *handlers.WorkflowHandler,
	nocPassH *handlers.NocPassHandler,
	esSyslogH *handlers.EsSyslogHandler,
	slackEventsH *handlers.SlackEventsHandler,
	slackAlarmsH *slackalarms.Handler,
) {
	// WebSocket endpoint (auth inside handler)
	r.GET("/ws", websocket.ServerWs(hub, jwtManager))

	// Public routes
	r.GET("/", func(c *gin.Context) { c.Redirect(302, "/login") })
	r.GET("/login", pageH.Login)

	if slackEventsH != nil {
		r.POST("/api/slack/events", slackEventsH.Handle)
	}

	// Auth API (public)
	r.POST("/api/auth/login", authH.Login)

	// Page routes viewable by excess, admin, and viewer
	pages := r.Group("/")
	pages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("excess", "admin", "viewer"))
	{
		pages.GET("/dashboard", pageH.Dashboard)
		pages.GET("/devices", pageH.Devices)
		pages.GET("/alerts", pageH.Alerts)
	}

	// Backups page (excess + admin only)
	backupPages := r.Group("/")
	backupPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("excess", "admin"))
	{
		backupPages.GET("/backups", pageH.Backups)
	}

	// Admin-only page routes
	adminPages := r.Group("/")
	adminPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("admin"))
	{
		adminPages.GET("/admin/users", pageH.AdminUsers)
	}

	// Auth API routes (any authenticated user)
	authAPI := r.Group("/api/auth")
	authAPI.Use(middleware.AuthMiddleware(jwtManager))
	{
		authAPI.POST("/logout", authH.Logout)
		authAPI.GET("/me", authH.Me)
		authAPI.POST("/refresh", authH.Refresh)
	}

	// Read-only API routes (excess, admin, viewer)
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("excess", "admin", "viewer"))
	{
		api.GET("/devices", powerH.GetDevices)

		power := api.Group("/power")
		{
			power.GET("/readings", powerH.GetAll)
			power.GET("/weak", powerH.GetWeak)
			power.GET("/summary", powerH.GetSummary)
		}

		desc := api.Group("/descriptions")
		{
			desc.GET("", descH.GetAll)
			desc.GET("/:host", descH.GetByHost)
		}

		health := api.Group("/health")
		{
			health.GET("", healthH.GetAll)
			health.GET("/history", healthHistoryH.GetHistory)
			health.GET("/:host", healthH.GetByHost)
		}

		ports := api.Group("/ports")
		{
			ports.GET("/down", portH.GetDown)
			ports.GET("/history", portHistoryH.GetHistory)
			ports.GET("/history/counts", portHistoryH.GetDownCounts)
			ports.GET("/:host", portH.GetByHost)
		}

		history := api.Group("/history")
		{
			history.GET("/calendar", calendarH.GetCalendar)
			history.GET("/day", calendarH.GetDayDetail)
		}

		inventory := api.Group("/inventory")
		{
			inventory.GET("/summary", inventoryH.GetLatestSummary)
			inventory.GET("/olts", inventoryH.GetLatestOltInventories)
			inventory.GET("/olts/:host", inventoryH.GetOltInventoryHistory)
		}
	}

	// Write API routes (excess + admin only: backups, scans, admin)
	writeAPI := r.Group("/api")
	writeAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("excess", "admin"))
	{
		backups := writeAPI.Group("/backups")
		{
			backups.GET("", backupH.GetAll)
			backups.GET("/:id/download", backupH.Download)
		}

		scan := writeAPI.Group("/scan")
		{
			scan.POST("/health", scanH.RunHealth)
			scan.POST("/power", scanH.RunPower)
			scan.POST("/ports", scanH.RunPorts)
			scan.POST("/inventory", scanH.RunInventory)
			scan.POST("/backup", scanH.RunBackup)
		}

		// Admin-only API routes
		users := writeAPI.Group("/admin/users")
		users.Use(middleware.RoleGuard("admin"))
		{
			users.GET("", userH.ListUsers)
			users.POST("", userH.Create)
			users.PUT("/:id", userH.UpdateUser)
			users.DELETE("/:id", userH.DeleteUser)
		}

		if slackAlarmsH != nil {
			alarms := writeAPI.Group("/admin/slack-alarms")
			alarms.Use(middleware.RoleGuard("admin"))
			{
				alarms.GET("/reminders", slackAlarmsH.ListOpen)
				alarms.POST("/reminders", slackAlarmsH.Create)
				alarms.DELETE("/reminders/:id", slackAlarmsH.Resolve)
			}
		}
	}

	// ──────────── IP Team pages (role: ip, admin) ────────────
	ipPages := r.Group("/")
	ipPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("ip", "admin"))
	{
		ipPages.GET("/workflows", pageH.Workflows)
		ipPages.GET("/ip-backups", pageH.IPBackups)
		ipPages.GET("/ip-cmd-output", pageH.IPCmdOutput)
		ipPages.GET("/ip-activity-log", pageH.IPActivityLog)
		ipPages.GET("/ip-syslog-alerts", pageH.IPSyslogAlerts)
	}

	// NOC Pass pages (role: noc, admin)
	nocPages := r.Group("/")
	nocPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("noc", "admin"))
	{
		nocPages.GET("/noc-pass", pageH.NocPass)
	}

	// NOC Pass API (role: noc, admin)
	nocAPI := r.Group("/api/noc-pass")
	nocAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("noc", "admin"))
	{
		nocAPI.GET("/devices", nocPassH.ListDevices)
		nocAPI.POST("/devices", nocPassH.CreateDevice)
		nocAPI.DELETE("/devices/:id", nocPassH.DeleteDevice)
		nocAPI.GET("/devices/:id/credential", nocPassH.Credential)
		nocAPI.POST("/devices/:id/rotate", nocPassH.RotateNow)
	}

	// IP Team API routes (role: ip, admin)
	wfAPI := r.Group("/api/workflows")
	wfAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("ip", "admin"))
	{
		wfAPI.GET("/devices", workflowH.ListDevices)
		wfAPI.POST("/devices", workflowH.CreateDevice)
		wfAPI.PUT("/devices/:id", workflowH.UpdateDevice)
		wfAPI.DELETE("/devices/:id", workflowH.DeleteDevice)

		wfAPI.GET("/jobs", workflowH.ListJobs)
		wfAPI.POST("/jobs", workflowH.CreateJob)
		wfAPI.PUT("/jobs/:id", workflowH.UpdateJob)
		wfAPI.DELETE("/jobs/:id", workflowH.DeleteJob)
		wfAPI.POST("/jobs/:id/run", workflowH.RunJobNow)
		wfAPI.GET("/jobs/:id/runs", workflowH.GetRuns)

		wfAPI.GET("/runs", workflowH.GetRunsByType)
		wfAPI.GET("/runs/:id/output", workflowH.GetRunOutput)

		wfAPI.GET("/logs", workflowH.GetLogs)
	}

	// IP Team — Elasticsearch syslog alerts (role: ip, admin)
	esAPI := r.Group("/api/ip/syslog")
	esAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("ip", "admin"))
	{
		esAPI.GET("/alerts", esSyslogH.ListAlerts)
		esAPI.GET("/filters", esSyslogH.ListFilters)
		esAPI.POST("/filters", esSyslogH.CreateFilter)
		esAPI.PUT("/filters/:id", esSyslogH.UpdateFilter)
		esAPI.DELETE("/filters/:id", esSyslogH.DeleteFilter)
	}
}

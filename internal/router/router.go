package router

import (
	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/Flafl/DevOpsCore/internal/middleware"
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
	accessOltH *handlers.AccessOltHandler,
	scanH *handlers.ScanHandler,
	workflowH *handlers.WorkflowHandler,
	nocWorkflowH *handlers.WorkflowHandler,
	nocPassH *handlers.NocPassHandler,
	nocDataH *handlers.NocDataHandler,
	esSyslogH *handlers.EsSyslogHandler,
	ipCapacityH *handlers.IPCapacityHandler,
	betterStackH *handlers.BetterStackHandler,
	slackEventsH *handlers.SlackEventsHandler,
	betterStackWebhookH *handlers.BetterStackWebhookHandler,
) {
	// WebSocket endpoint (auth inside handler)
	r.GET("/ws", websocket.ServerWs(hub, jwtManager))

	// Public routes
	r.GET("/", func(c *gin.Context) { c.Redirect(302, "/login") })
	r.GET("/login", pageH.Login)

	if slackEventsH != nil {
		r.POST("/api/slack/events", slackEventsH.Handle)
	}
	if betterStackWebhookH != nil {
		r.POST("/api/webhooks/betterstack", betterStackWebhookH.Handle)
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
		backupPages.GET("/olt-setup", pageH.OltSetup)
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
			inventory.GET("/onts", inventoryH.GetOntInterfaces)
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

		oltSetup := writeAPI.Group("/access/olt-setup")
		{
			oltSetup.GET("/olts", accessOltH.ListOlts)
			oltSetup.POST("/olts", accessOltH.CreateOlt)
			oltSetup.DELETE("/olts/:id", accessOltH.DeleteOlt)
			oltSetup.GET("/credentials", accessOltH.ListCredentials)
			oltSetup.POST("/credentials", accessOltH.CreateCredential)
			oltSetup.DELETE("/credentials/:id", accessOltH.DeleteCredential)
		}
	}

	// ──────────── IP Team pages (role: ip, admin) ────────────
	ipPages := r.Group("/")
	ipPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("ip", "admin"))
	{
		ipPages.GET("/workflows", pageH.Workflows)
		ipPages.GET("/ip-backups", pageH.IPBackups)
		ipPages.GET("/bng-sync-checker", pageH.BNGSyncChecker)
		ipPages.GET("/ip-cmd-output", pageH.IPCmdOutput)
		ipPages.GET("/ip-activity-log", pageH.IPActivityLog)
		ipPages.GET("/ip-alerts", pageH.IPAlerts)
		ipPages.GET("/ip-syslog-alerts", func(c *gin.Context) { c.Redirect(302, "/ip-alerts") })
		ipPages.GET("/ip-capacities", pageH.IPCapacities)
	}

	// NOC Pass pages (role: noc, admin)
	nocPages := r.Group("/")
	nocPages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("noc", "admin"))
	{
		nocPages.GET("/noc-setup", pageH.NocSetup)
		nocPages.GET("/noc-pass", pageH.NocPass)
		nocPages.GET("/noc-data", pageH.NocData)
		nocPages.GET("/noc-workflows", pageH.NocWorkflows)
		nocPages.GET("/noc-backups", pageH.NocBackups)
		nocPages.GET("/noc-cmd-output", pageH.NocCmdOutput)
		nocPages.GET("/noc-activity-log", pageH.NocActivityLog)
	}

	// NOC Pass API (role: noc, admin)
	nocAPI := r.Group("/api/noc-pass")
	nocAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("noc", "admin"))
	{
		nocAPI.GET("/all-devices", nocPassH.ListAllDevices)
		nocAPI.GET("/devices", nocPassH.ListDevices)
		nocAPI.GET("/policies", nocPassH.ListPolicies)
		nocAPI.POST("/policies", nocPassH.CreatePolicy)
		nocAPI.PUT("/policies/:id", nocPassH.UpdatePolicy)
		nocAPI.DELETE("/policies/:id", nocPassH.DeletePolicy)
		nocAPI.POST("/policies/:id/run", nocPassH.RunPolicyNow)
		nocAPI.POST("/devices", nocPassH.CreateDevice)
		nocAPI.DELETE("/devices/:id", nocPassH.DeleteDevice)
		nocAPI.GET("/devices/:id/credential", nocPassH.Credential)
		nocAPI.POST("/devices/:id/rotate", nocPassH.RotateNow)
		nocAPI.GET("/keep-users", nocPassH.ListKeepUsers)
		nocAPI.POST("/keep-users", nocPassH.CreateKeepUser)
		nocAPI.DELETE("/keep-users/:id", nocPassH.DeleteKeepUser)
		nocAPI.GET("/saved-users", nocPassH.ListSavedUsers)
		nocAPI.POST("/saved-users", nocPassH.CreateSavedUser)
		nocAPI.DELETE("/saved-users/:id", nocPassH.DeleteSavedUser)
		nocAPI.GET("/exclusions", nocPassH.ListExclusions)
		nocAPI.POST("/exclusions", nocPassH.CreateExclusion)
		nocAPI.DELETE("/exclusions/:id", nocPassH.DeleteExclusion)
	}

	nocDataAPI := r.Group("/api/noc-data")
	nocDataAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("noc", "admin"))
	{
		nocDataAPI.GET("/devices", nocDataH.ListDevices)
		nocDataAPI.GET("/history/dates", nocDataH.ListHistoryDates)
		nocDataAPI.GET("/history", nocDataH.ListHistory)
		nocDataAPI.POST("/devices", nocDataH.CreateDevice)
		nocDataAPI.DELETE("/devices/:id", nocDataH.DeleteDevice)
		nocDataAPI.POST("/failed/run", nocDataH.RunFailedDevicesOneByOne)
		nocDataAPI.POST("/failed/run/:id", nocDataH.RunFailedDevice)
		nocDataAPI.GET("/credentials", nocDataH.ListCredentials)
		nocDataAPI.POST("/credentials", nocDataH.CreateCredential)
		nocDataAPI.DELETE("/credentials/:id", nocDataH.DeleteCredential)
		nocDataAPI.GET("/exclusions", nocDataH.ListExclusions)
		nocDataAPI.POST("/exclusions", nocDataH.CreateExclusion)
		nocDataAPI.DELETE("/exclusions/:id", nocDataH.DeleteExclusion)
		nocDataAPI.POST("/run", nocDataH.RunAll)
		nocDataAPI.GET("/export.csv", nocDataH.ExportCSV)
	}

	// NOC Team workflow and backup API routes (role: noc, admin)
	nocWfAPI := r.Group("/api/noc-workflows")
	nocWfAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("noc", "admin"))
	{
		nocWfAPI.GET("/devices", nocWorkflowH.ListDevices)
		nocWfAPI.POST("/devices", nocWorkflowH.CreateDevice)
		nocWfAPI.PUT("/devices/:id", nocWorkflowH.UpdateDevice)
		nocWfAPI.DELETE("/devices/:id", nocWorkflowH.DeleteDevice)

		nocWfAPI.GET("/jobs", nocWorkflowH.ListJobs)
		nocWfAPI.POST("/jobs", nocWorkflowH.CreateJob)
		nocWfAPI.PUT("/jobs/:id", nocWorkflowH.UpdateJob)
		nocWfAPI.DELETE("/jobs/:id", nocWorkflowH.DeleteJob)
		nocWfAPI.POST("/jobs/:id/run", nocWorkflowH.RunJobNow)
		nocWfAPI.GET("/jobs/:id/runs", nocWorkflowH.GetRuns)

		nocWfAPI.GET("/runs", nocWorkflowH.GetRunsByType)
		nocWfAPI.GET("/backups/compare", nocWorkflowH.CompareBackups)
		nocWfAPI.GET("/runs/:id/output", nocWorkflowH.GetRunOutput)

		nocWfAPI.GET("/logs", nocWorkflowH.GetLogs)
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
		wfAPI.GET("/backups/compare", workflowH.CompareBackups)
		wfAPI.POST("/bng-sync-checker", workflowH.CheckBNGSync)
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

	betterStackAPI := r.Group("/api/ip/betterstack")
	betterStackAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("ip", "admin"))
	{
		betterStackAPI.GET("/incidents", betterStackH.ListIncidents)
	}

	capacityAPI := r.Group("/api/ip/capacities")
	capacityAPI.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("ip", "admin"))
	{
		capacityAPI.GET("/nodes", ipCapacityH.ListNodes)
		capacityAPI.POST("/nodes", ipCapacityH.CreateNode)
		capacityAPI.PUT("/nodes/:id", ipCapacityH.UpdateNode)
		capacityAPI.DELETE("/nodes/:id", ipCapacityH.DeleteNode)
		capacityAPI.GET("/actions", ipCapacityH.ListActions)
		capacityAPI.POST("/actions", ipCapacityH.CreateAction)
		capacityAPI.PUT("/actions/:id", ipCapacityH.UpdateAction)
		capacityAPI.DELETE("/actions/:id", ipCapacityH.DeleteAction)
		capacityAPI.POST("/import", ipCapacityH.ImportActions)
		capacityAPI.GET("/history/days", ipCapacityH.ListHistoryDays)
		capacityAPI.GET("/history/day", ipCapacityH.GetDayHistory)
		capacityAPI.GET("/history/all", ipCapacityH.GetAllHistory)
	}
}

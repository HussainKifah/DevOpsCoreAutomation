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
	scanH *handlers.ScanHandler,
) {
	// WebSocket endpoint (auth inside handler)
	r.GET("/ws", websocket.ServerWs(hub, jwtManager))

	// Public routes
	r.GET("/login", pageH.Login)

	// Auth API (public)
	r.POST("/api/auth/login", authH.Login)

	// Protected page routes (excess + admin only)
	pages := r.Group("/")
	pages.Use(middleware.PageAuthMiddleware(jwtManager), middleware.PageRoleGuard("excess", "admin"))
	{
		pages.GET("/", func(c *gin.Context) { c.Redirect(302, "/dashboard") })
		pages.GET("/dashboard", pageH.Dashboard)
		pages.GET("/devices", pageH.Devices)
		pages.GET("/alerts", pageH.Alerts)
		pages.GET("/backups", pageH.Backups)
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

	// Protected API routes (excess + admin only)
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware(jwtManager), middleware.RoleGuard("excess", "admin"))
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

		backups := api.Group("/backups")
		{
			backups.GET("", backupH.GetAll)
			backups.GET("/:id/download", backupH.Download)
		}

		inventory := api.Group("/inventory")
		{
			inventory.GET("/summary", inventoryH.GetLatestSummary)
			inventory.GET("/olts", inventoryH.GetLatestOltInventories)
			inventory.GET("/olts/:host", inventoryH.GetOltInventoryHistory)
		}

		scan := api.Group("/scan")
		{
			scan.POST("/health", scanH.RunHealth)
			scan.POST("/power", scanH.RunPower)
			scan.POST("/ports", scanH.RunPorts)
			scan.POST("/inventory", scanH.RunInventory)
		}

		// Admin-only API routes
		users := api.Group("/admin/users")
		users.Use(middleware.RoleGuard("admin"))
		{
			users.GET("", userH.ListUsers)
			users.POST("", userH.Create)
			users.PUT("/:id", userH.UpdateUser)
			users.DELETE("/:id", userH.DeleteUser)
		}
	}
}

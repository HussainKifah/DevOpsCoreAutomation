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
	portH *handlers.PortHandler,
	backupH *handlers.BackupHandler,
	userH *handlers.UserhHandler,
	authH *handlers.AuthHandler,
	pageH *handlers.PageHandler,
) {
	// WebSocket endpoint (auth inside handler)
	r.GET("/ws", websocket.ServerWs(hub, jwtManager))

	// Public routes
	r.GET("/login", pageH.Login)

	// Auth API (public)
	r.POST("/api/auth/login", authH.Login)

	// Protected page routes (all authenticated users)
	pages := r.Group("/")
	pages.Use(middleware.PageAuthMiddleware(jwtManager))
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

	// Protected API routes (all authenticated users)
	api := r.Group("/api")
	api.Use(middleware.AuthMiddleware(jwtManager))
	{
		api.POST("/auth/logout", authH.Logout)
		api.GET("/auth/me", authH.Me)
		api.POST("/auth/refresh", authH.Refresh)

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
			health.GET("/:host", healthH.GetByHost)
		}

		ports := api.Group("/ports")
		{
			ports.GET("/down", portH.GetDown)
			ports.GET("/:host", portH.GetByHost)
		}

		backups := api.Group("/backups")
		{
			backups.GET("", backupH.GetAll)
			backups.GET("/:id/download", backupH.Download)
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

package router

import (
	"github.com/Flafl/DevOpsCore/internal/handlers"
	"github.com/gin-gonic/gin"
)

func Setup(
	r *gin.Engine,
	powerH *handlers.PowerHandler,
	descH *handlers.DescriptionHandler,
	healthH *handlers.HealthHandler,
	portH *handlers.PortHandler,
	backupH *handlers.BackupHandler,
	userH *handlers.UserhHandler,
	pageH *handlers.PageHandler,
) {
	// Page routes
	r.GET("/", func(c *gin.Context) { c.Redirect(302, "/dashboard") })
	r.GET("/dashboard", pageH.Dashboard)
	r.GET("/devices", pageH.Devices)
	r.GET("/alerts", pageH.Alerts)
	r.GET("/backups", pageH.Backups)
	r.GET("/admin/users", pageH.AdminUsers)

	// API routes
	api := r.Group("/api")

	api.GET("/devices", powerH.GetDevices)

	power := api.Group("/power")
	{
		power.GET("/readings", powerH.GetAll)
		power.GET("/weak", powerH.GetWeak)
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

	users := api.Group("/admin/users")
	{
		users.GET("", userH.ListUsers)
		users.POST("", userH.Create)
		users.PUT("/:id", userH.UpdateUser)
		users.DELETE("/:id", userH.DeleteUser)
	}
}

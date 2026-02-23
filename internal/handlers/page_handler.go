package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type PageHandler struct {
	templates map[string]*template.Template
}

func NewPageHandler(templateDir string) *PageHandler {
	base := filepath.Join(templateDir, "layout", "base.html")
	pages := map[string]string{
		"dashboard":   filepath.Join(templateDir, "excess", "dashboard.html"),
		"devices":     filepath.Join(templateDir, "excess", "devices.html"),
		"alerts":      filepath.Join(templateDir, "excess", "alerts.html"),
		"backups":     filepath.Join(templateDir, "excess", "backups.html"),
		"admin-users": filepath.Join(templateDir, "admin", "users.html"),
	}

	tmpl := make(map[string]*template.Template, len(pages))
	for name, page := range pages {
		t, err := template.ParseFiles(base, page)
		if err != nil {
			log.Fatalf("failed to parse template %s: %v", name, err)
		}
		tmpl[name] = t
	}
	return &PageHandler{templates: tmpl}
}

func (h *PageHandler) render(c *gin.Context, name string, data gin.H) {
	t, ok := h.templates[name]
	if !ok {
		c.String(http.StatusInternalServerError, "template %s not found", name)
		return
	}
	if data == nil {
		data = gin.H{}
	}
	data["Page"] = name
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(c.Writer, "base", data); err != nil {
		log.Printf("template render error: %v", err)
	}
}

func (h *PageHandler) Dashboard(c *gin.Context) { h.render(c, "dashboard", nil) }
func (h *PageHandler) Devices(c *gin.Context)   { h.render(c, "devices", nil) }
func (h *PageHandler) Alerts(c *gin.Context)     { h.render(c, "alerts", nil) }
func (h *PageHandler) Backups(c *gin.Context)    { h.render(c, "backups", nil) }
func (h *PageHandler) AdminUsers(c *gin.Context) { h.render(c, "admin-users", nil) }

package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"

	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type PageHandler struct {
	templates    map[string]*template.Template
	loginTmpl    *template.Template
	userRepo     repository.UserRepository
	jwtManager   *auth.JWTManager
}

func NewPageHandler(templateDir string, userRepo repository.UserRepository, jwtManager *auth.JWTManager) *PageHandler {
	base := filepath.Join(templateDir, "layout", "base.html")
	pages := map[string]string{
		"dashboard":   filepath.Join(templateDir, "excess", "dashboard.html"),
		"devices":     filepath.Join(templateDir, "excess", "devices.html"),
		"alerts":      filepath.Join(templateDir, "excess", "alerts.html"),
		"backups":     filepath.Join(templateDir, "excess", "backups.html"),
		"admin-users":     filepath.Join(templateDir, "admin", "users.html"),
		"workflows":       filepath.Join(templateDir, "ip", "workflows.html"),
		"ip-backups":      filepath.Join(templateDir, "ip", "backups.html"),
		"ip-cmd-output":   filepath.Join(templateDir, "ip", "cmd_output.html"),
		"ip-activity-log": filepath.Join(templateDir, "ip", "activity_log.html"),
		"ip-syslog-alerts": filepath.Join(templateDir, "ip", "syslog_alerts.html"),
		"noc-pass":        filepath.Join(templateDir, "noc", "noc_pass.html"),
	}

	tmpl := make(map[string]*template.Template, len(pages))
	for name, page := range pages {
		t, err := template.ParseFiles(base, page)
		if err != nil {
			log.Fatalf("failed to parse template %s: %v", name, err)
		}
		tmpl[name] = t
	}

	loginTmpl, err := template.ParseFiles(filepath.Join(templateDir, "auth", "login.html"))
	if err != nil {
		log.Fatalf("failed to parse login template: %v", err)
	}

	return &PageHandler{templates: tmpl, loginTmpl: loginTmpl, userRepo: userRepo, jwtManager: jwtManager}
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

	if claims, exists := c.Get("user"); exists {
		if uc, ok := claims.(*auth.Claims); ok {
			data["UserEmail"] = uc.Email
			data["UserRole"] = uc.Role
			data["UserID"] = uc.UserID
			// Prefer full name for display; fallback to email if full name is empty
			if user, err := h.userRepo.GetByID(uc.UserID); err == nil && user.Fullname != "" {
				data["UserName"] = user.Fullname
			} else {
				data["UserName"] = uc.Email
			}
		}
	}

	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(c.Writer, "base", data); err != nil {
		log.Printf("template render error: %v", err)
	}
}

func (h *PageHandler) Login(c *gin.Context) {
	if cookie, err := c.Cookie("access_token"); err == nil && cookie != "" {
		if claims, err := h.jwtManager.ValidateToken(cookie); err == nil && claims.TokenType == "access" {
			if claims.Role == "noc" {
				c.Redirect(http.StatusFound, "/noc-pass")
			} else {
				c.Redirect(http.StatusFound, "/dashboard")
			}
			return
		}
		c.SetCookie("access_token", "", -1, "/", "", false, true)
		c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	}
	c.Header("Content-Type", "text/html; charset=utf-8")
	if err := h.loginTmpl.Execute(c.Writer, nil); err != nil {
		log.Printf("login template render error: %v", err)
	}
}

func (h *PageHandler) Dashboard(c *gin.Context) { h.render(c, "dashboard", nil) }
func (h *PageHandler) Devices(c *gin.Context)   { h.render(c, "devices", nil) }
func (h *PageHandler) Alerts(c *gin.Context)     { h.render(c, "alerts", nil) }
func (h *PageHandler) Backups(c *gin.Context)    { h.render(c, "backups", nil) }
func (h *PageHandler) AdminUsers(c *gin.Context)  { h.render(c, "admin-users", nil) }
func (h *PageHandler) Workflows(c *gin.Context)   { h.render(c, "workflows", nil) }
func (h *PageHandler) IPBackups(c *gin.Context)    { h.render(c, "ip-backups", nil) }
func (h *PageHandler) IPCmdOutput(c *gin.Context)  { h.render(c, "ip-cmd-output", nil) }
func (h *PageHandler) IPActivityLog(c *gin.Context) { h.render(c, "ip-activity-log", nil) }
func (h *PageHandler) IPSyslogAlerts(c *gin.Context) { h.render(c, "ip-syslog-alerts", nil) }
func (h *PageHandler) NocPass(c *gin.Context)        { h.render(c, "noc-pass", nil) }

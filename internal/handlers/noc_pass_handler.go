package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/nocpass"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type NocPassHandler struct {
	repo repository.NocPassRepository
	key  []byte
}

func NewNocPassHandler(repo repository.NocPassRepository, masterKey []byte) *NocPassHandler {
	return &NocPassHandler{repo: repo, key: masterKey}
}

type nocPassAccountDTO struct {
	Username string `json:"username"`
	Hint     string `json:"hint"`
}

type nocPassDeviceDTO struct {
	ID                uint                `json:"id"`
	DisplayName       string              `json:"display_name"`
	Host              string              `json:"host"`
	Vendor            string              `json:"vendor"`
	Accounts          []nocPassAccountDTO `json:"accounts"`
	LastApplyOK       bool                `json:"last_apply_ok"`
	LastApplyError    string              `json:"last_apply_error,omitempty"`
	LastAppliedAt     *time.Time          `json:"last_applied_at,omitempty"`
	PasswordRotatedAt *time.Time          `json:"password_rotated_at,omitempty"`
}

type nocPassKeepUserDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
}

func fixedAccountsDTO() []nocPassAccountDTO {
	out := make([]nocPassAccountDTO, 0, len(nocpass.AccountSummary))
	for _, a := range nocpass.AccountSummary {
		out = append(out, nocPassAccountDTO{Username: a.Username, Hint: a.Hint})
	}
	return out
}

func toDTO(d *models.NocPassDevice) nocPassDeviceDTO {
	return nocPassDeviceDTO{
		ID:                d.ID,
		DisplayName:       d.DisplayName,
		Host:              d.Host,
		Vendor:            d.Vendor,
		Accounts:          fixedAccountsDTO(),
		LastApplyOK:       d.LastApplyOK,
		LastApplyError:    d.LastApplyError,
		LastAppliedAt:     d.LastAppliedAt,
		PasswordRotatedAt: d.PasswordRotatedAt,
	}
}

func toKeepUserDTO(u *models.NocPassKeepUser) nocPassKeepUserDTO {
	return nocPassKeepUserDTO{
		ID:       u.ID,
		Username: u.Username,
	}
}

// ListDevices GET /api/noc-pass/devices?q=
func (h *NocPassHandler) ListDevices(c *gin.Context) {
	q := c.Query("q")
	list, err := h.repo.Search(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocPassDeviceDTO, 0, len(list))
	for i := range list {
		out = append(out, toDTO(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

// ListKeepUsers GET /api/noc-pass/keep-users
func (h *NocPassHandler) ListKeepUsers(c *gin.Context) {
	list, err := h.repo.ListKeepUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocPassKeepUserDTO, 0, len(list))
	for i := range list {
		out = append(out, toKeepUserDTO(&list[i]))
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

type nocPassCreateReq struct {
	DisplayName   string `json:"display_name" binding:"required"`
	Host          string `json:"host" binding:"required"`
	Vendor        string `json:"vendor" binding:"required"`
	AdminUsername string `json:"admin_username" binding:"required"`
	AdminPassword string `json:"admin_password" binding:"required"`
}

type nocPassKeepUserCreateReq struct {
	Username string `json:"username" binding:"required"`
}

// CreateDevice POST /api/noc-pass/devices
func (h *NocPassHandler) CreateDevice(c *gin.Context) {
	var req nocPassCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	v := strings.ToLower(strings.TrimSpace(req.Vendor))
	if v != "cisco_ios" && v != "cisco_nexus" && v != "mikrotik" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vendor must be cisco_ios, cisco_nexus, or mikrotik"})
		return
	}

	encUser, err := crypto.Encrypt(h.key, req.AdminUsername)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt admin user"})
		return
	}
	encPass, err := crypto.Encrypt(h.key, req.AdminPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt admin password"})
		return
	}

	d := &models.NocPassDevice{
		DisplayName:  strings.TrimSpace(req.DisplayName),
		Host:         strings.TrimSpace(req.Host),
		Vendor:       v,
		EncAdminUser: encUser,
		EncAdminPass: encPass,
		Enabled:      true,
	}
	if err := h.repo.Create(d); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	id := d.ID
	go func() {
		if err := nocpass.RotateAndApply(h.repo, h.key, id); err != nil {
			log.Printf("[noc-pass] initial apply id=%d: %v", id, err)
		}
	}()

	c.JSON(http.StatusCreated, gin.H{"device": toDTO(d)})
}

// CreateKeepUser POST /api/noc-pass/keep-users
func (h *NocPassHandler) CreateKeepUser(c *gin.Context) {
	var req nocPassKeepUserCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	username := nocpass.NormalizeUsername(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	if username == nocpass.NormalizeUsername(nocpass.UserFiberx) || username == nocpass.NormalizeUsername(nocpass.UserReadOnly) || username == nocpass.NormalizeUsername(nocpass.UserDev) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is already protected by default"})
		return
	}
	user := &models.NocPassKeepUser{Username: username}
	if err := h.repo.CreateKeepUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": toKeepUserDTO(user)})
}

// DeleteDevice DELETE /api/noc-pass/devices/:id
func (h *NocPassHandler) DeleteDevice(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.Delete(uint(id64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteKeepUser DELETE /api/noc-pass/keep-users/:id
func (h *NocPassHandler) DeleteKeepUser(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteKeepUser(uint(id64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Credential GET /api/noc-pass/devices/:id/credential
func (h *NocPassHandler) Credential(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	d, err := h.repo.GetByID(uint(id64))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	accts := fixedAccountsDTO()
	if len(d.EncNocPassword) == 0 {
		c.JSON(http.StatusAccepted, gin.H{
			"accounts":            accts,
			"password":            "",
			"pending":             true,
			"last_apply_ok":       d.LastApplyOK,
			"last_apply_error":    d.LastApplyError,
			"password_rotated_at": d.PasswordRotatedAt,
		})
		return
	}
	plain, err := crypto.Decrypt(h.key, d.EncNocPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt"})
		return
	}
	var nextRotate *time.Time
	if d.PasswordRotatedAt != nil {
		t := d.PasswordRotatedAt.Add(nocpass.RotateInterval)
		nextRotate = &t
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts":            accts,
		"password":            plain,
		"pending":             false,
		"password_rotated_at": d.PasswordRotatedAt,
		"next_rotation_at":    nextRotate,
		"last_apply_ok":       d.LastApplyOK,
		"last_apply_error":    d.LastApplyError,
	})
}

// RotateNow POST /api/noc-pass/devices/:id/rotate
func (h *NocPassHandler) RotateNow(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := nocpass.RotateAndApply(h.repo, h.key, uint(id64)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	d, _ := h.repo.GetByID(uint(id64))
	c.JSON(http.StatusOK, gin.H{"device": toDTO(d)})
}

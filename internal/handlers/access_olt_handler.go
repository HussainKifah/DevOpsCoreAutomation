package handlers

import (
	"log"
	"net/http"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
)

type AccessOltHandler struct {
	repo repository.AccessOltRepository
	key  []byte
}

func NewAccessOltHandler(repo repository.AccessOltRepository, key []byte) *AccessOltHandler {
	return &AccessOltHandler{repo: repo, key: key}
}

type accessOltDTO struct {
	ID        uint     `json:"id"`
	IP        string   `json:"ip"`
	Name      string   `json:"name"`
	Site      string   `json:"site"`
	OltType   string   `json:"olt_type"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
}

type accessOltCredentialDTO struct {
	ID           uint   `json:"id"`
	VendorFamily string `json:"vendor_family"`
	Username     string `json:"username"`
}

func (h *AccessOltHandler) ListOlts(c *gin.Context) {
	list, err := h.repo.ListOlts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]accessOltDTO, 0, len(list))
	for _, item := range list {
		out = append(out, toAccessOltDTO(&item))
	}
	c.JSON(http.StatusOK, gin.H{"olts": out})
}

func (h *AccessOltHandler) CreateOlt(c *gin.Context) {
	var req struct {
		IP        string   `json:"ip" binding:"required"`
		Name      string   `json:"name" binding:"required"`
		Site      string   `json:"site" binding:"required"`
		OltType   string   `json:"olt_type" binding:"required"`
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	oltType := normalizeAccessOltVendor(req.OltType)
	if oltType == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "olt_type must be nokia or huawei"})
		return
	}
	item := &models.AccessOlt{
		IP:        strings.TrimSpace(req.IP),
		Name:      strings.TrimSpace(req.Name),
		Site:      strings.TrimSpace(req.Site),
		OltType:   oltType,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}
	if item.IP == "" || item.Name == "" || item.Site == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip, name, and site are required"})
		return
	}
	if err := h.repo.CreateOlt(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"olt": toAccessOltDTO(item)})
}

func (h *AccessOltHandler) DeleteOlt(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteOlt(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AccessOltHandler) ListCredentials(c *gin.Context) {
	list, err := h.repo.ListCredentials(normalizeAccessOltVendor(c.Query("vendor_family")))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]accessOltCredentialDTO, 0, len(list))
	for _, item := range list {
		user, err := crypto.Decrypt(h.key, item.EncUsername)
		if err != nil {
			log.Printf("[access-olt] decrypt username credential id=%d: %v", item.ID, err)
			continue
		}
		out = append(out, accessOltCredentialDTO{
			ID:           item.ID,
			VendorFamily: item.VendorFamily,
			Username:     strings.TrimSpace(user),
		})
	}
	c.JSON(http.StatusOK, gin.H{"credentials": out})
}

func (h *AccessOltHandler) CreateCredential(c *gin.Context) {
	var req struct {
		VendorFamily string `json:"vendor_family" binding:"required"`
		Username     string `json:"username" binding:"required"`
		Password     string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	family := normalizeAccessOltVendor(req.VendorFamily)
	if family == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "vendor_family must be nokia or huawei"})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}
	encUser, err := crypto.Encrypt(h.key, username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt username"})
		return
	}
	encPass, err := crypto.Encrypt(h.key, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password"})
		return
	}
	item := &models.AccessOltCredential{
		VendorFamily: family,
		EncUsername:  encUser,
		EncPassword:  encPass,
		Enabled:      true,
	}
	if err := h.repo.CreateCredential(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"credential": accessOltCredentialDTO{
		ID:           item.ID,
		VendorFamily: family,
		Username:     username,
	}})
}

func (h *AccessOltHandler) DeleteCredential(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteCredential(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func toAccessOltDTO(item *models.AccessOlt) accessOltDTO {
	return accessOltDTO{
		ID:        item.ID,
		IP:        item.IP,
		Name:      item.Name,
		Site:      item.Site,
		OltType:   item.OltType,
		Latitude:  item.Latitude,
		Longitude: item.Longitude,
	}
}

func normalizeAccessOltVendor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "nokia":
		return "nokia"
	case "huawei":
		return "huawei"
	default:
		return ""
	}
}

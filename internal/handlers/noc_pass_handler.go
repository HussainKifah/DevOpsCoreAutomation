package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/nocpass"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NocPassHandler struct {
	repo        repository.NocPassRepository
	nocDataRepo repository.NocDataRepository
	key         []byte
}

func NewNocPassHandler(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte) *NocPassHandler {
	return &NocPassHandler{repo: repo, nocDataRepo: nocDataRepo, key: masterKey}
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
	SourceVendor      string              `json:"source_vendor,omitempty"`
	Site              string              `json:"site,omitempty"`
	Model             string              `json:"model,omitempty"`
	Province          string              `json:"province,omitempty"`
	NetworkType       string              `json:"network_type,omitempty"`
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

type nocPassSavedUserDTO struct {
	ID            uint     `json:"id"`
	Username      string   `json:"username"`
	Privilege     string   `json:"privilege"`
	NetworkTypes  []string `json:"network_types"`
	Provinces     []string `json:"provinces"`
	Vendors       []string `json:"vendors"`
	Models        []string `json:"models"`
	Devices       []string `json:"devices"`
	MatchingCount int      `json:"matching_count"`
}

type nocPassExclusionDTO struct {
	ID     uint   `json:"id"`
	Subnet string `json:"subnet"`
	Target string `json:"target"`
}

type nocPassPolicyDTO struct {
	ID                    uint                `json:"id"`
	Name                  string              `json:"name"`
	Enabled               bool                `json:"enabled"`
	TargetType            string              `json:"target_type"`
	TargetValue           string              `json:"target_value"`
	TargetLabel           string              `json:"target_label"`
	PasswordMode          string              `json:"password_mode"`
	ActiveFiberxPassword  string              `json:"active_fiberx_password"`
	ActiveSupportPassword string              `json:"active_support_password"`
	ActivePasswordDate    string              `json:"active_password_date"`
	LastRunAt             *time.Time          `json:"last_run_at,omitempty"`
	LastStatus            string              `json:"last_status"`
	LastMessage           string              `json:"last_message"`
	NextRotationAt        *time.Time          `json:"next_rotation_at,omitempty"`
	Accounts              []nocPassAccountDTO `json:"accounts"`
}

func fixedAccountsDTO() []nocPassAccountDTO {
	out := make([]nocPassAccountDTO, 0, len(nocpass.AccountSummary))
	for _, a := range nocpass.AccountSummary {
		out = append(out, nocPassAccountDTO{Username: a.Username, Hint: a.Hint})
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func (h *NocPassHandler) savedUserDTO(item *models.NocPassSavedUser, rows []models.NocDataDevice) nocPassSavedUserDTO {
	targets := nocpass.SavedUserTargets{
		NetworkTypes: nocpass.DecodeSavedUserList(item.NetworkTypesJSON),
		Provinces:    nocpass.DecodeSavedUserList(item.ProvincesJSON),
		Vendors:      nocpass.DecodeSavedUserList(item.VendorsJSON),
		Models:       nocpass.DecodeSavedUserList(item.ModelsJSON),
		Devices:      nocpass.NormalizeDeviceTargets(nocpass.DecodeSavedUserList(item.DevicesJSON)...),
	}
	matching := make([]models.NocDataDevice, 0)
	for i := range rows {
		if nocpass.SavedUserMatchesRow(item, &rows[i]) {
			matching = append(matching, rows[i])
		}
	}
	return nocPassSavedUserDTO{
		ID:            item.ID,
		Username:      item.Username,
		Privilege:     nocpass.NormalizeSavedUserPrivilege(item.Privilege),
		NetworkTypes:  targets.NetworkTypes,
		Provinces:     targets.Provinces,
		Vendors:       targets.Vendors,
		Models:        targets.Models,
		Devices:       targets.Devices,
		MatchingCount: len(matching),
	}
}

func (h *NocPassHandler) statusLookup() map[string]*models.NocPassDevice {
	list, err := h.repo.ListStatuses()
	if err != nil {
		return map[string]*models.NocPassDevice{}
	}
	out := make(map[string]*models.NocPassDevice, len(list))
	for i := range list {
		host := strings.ToLower(strings.TrimSpace(list[i].Host))
		if host == "" {
			continue
		}
		item := list[i]
		out[host] = &item
	}
	return out
}

func toNocPassDeviceDTO(row *models.NocDataDevice, state *models.NocPassDevice) nocPassDeviceDTO {
	displayName := nocpass.DeviceLabel(row)
	host := strings.TrimSpace(row.Host)
	sourceVendor := nocpass.NormalizeSourceVendor(row.Vendor, row.DeviceModel)
	dto := nocPassDeviceDTO{
		DisplayName:  displayName,
		Host:         host,
		Vendor:       sourceVendor,
		SourceVendor: sourceVendor,
		Site:         strings.TrimSpace(row.Site),
		Model:        strings.TrimSpace(row.DeviceModel),
		Province:     nocpass.ProvinceFromSite(row.Site),
		NetworkType:  nocpass.NetworkTypeFromSite(row.Site),
		Accounts:     fixedAccountsDTO(),
	}
	if state != nil {
		dto.ID = state.ID
		dto.LastApplyOK = state.LastApplyOK
		dto.LastApplyError = state.LastApplyError
		dto.LastAppliedAt = state.LastAppliedAt
		dto.PasswordRotatedAt = state.PasswordRotatedAt
	}
	return dto
}

func policyToDTO(policy *models.NocPassPolicy, key []byte) nocPassPolicyDTO {
	dto := nocPassPolicyDTO{
		ID:                 policy.ID,
		Name:               strings.TrimSpace(policy.Name),
		Enabled:            policy.Enabled,
		TargetType:         policy.TargetType,
		TargetValue:        policy.TargetValue,
		TargetLabel:        policy.TargetLabel,
		PasswordMode:       policy.PasswordMode,
		ActivePasswordDate: policy.ActivePasswordDate,
		LastRunAt:          policy.LastRunAt,
		LastStatus:         policy.LastStatus,
		LastMessage:        policy.LastMessage,
		Accounts:           fixedAccountsDTO(),
	}
	if len(policy.EncActiveFiberxPassword) > 0 {
		if plain, err := crypto.Decrypt(key, policy.EncActiveFiberxPassword); err == nil {
			dto.ActiveFiberxPassword = plain
		}
	} else if len(policy.EncActivePassword) > 0 {
		if plain, err := crypto.Decrypt(key, policy.EncActivePassword); err == nil {
			dto.ActiveFiberxPassword = plain
		}
	}
	if len(policy.EncActiveSupportPassword) > 0 {
		if plain, err := crypto.Decrypt(key, policy.EncActiveSupportPassword); err == nil {
			dto.ActiveSupportPassword = plain
		}
	} else if len(policy.EncActivePassword) > 0 {
		if plain, err := crypto.Decrypt(key, policy.EncActivePassword); err == nil {
			dto.ActiveSupportPassword = plain
		}
	}
	if policy.ActivePasswordDate != "" {
		if t, err := time.ParseInLocation("2006-01-02", policy.ActivePasswordDate, time.Local); err == nil {
			next := t.Add(24 * time.Hour)
			dto.NextRotationAt = &next
		}
	}
	return dto
}

func (h *NocPassHandler) listNocDataDevices(q string) ([]models.NocDataDevice, error) {
	if strings.TrimSpace(q) != "" {
		return h.nocDataRepo.List(q)
	}
	return h.nocDataRepo.ListAll()
}

func (h *NocPassHandler) normalizePolicyInput(policy *models.NocPassPolicy, req *struct {
	Name                  string `json:"name"`
	Enabled               *bool  `json:"enabled"`
	TargetType            string `json:"target_type"`
	TargetValue           string `json:"target_value"`
	TargetLabel           string `json:"target_label"`
	PasswordMode          string `json:"password_mode"`
	ManualFiberxPassword  string `json:"manual_fiberx_password"`
	ManualSupportPassword string `json:"manual_support_password"`
}) error {
	if req == nil {
		return nil
	}
	if strings.TrimSpace(req.Name) != "" {
		policy.Name = strings.TrimSpace(req.Name)
	}
	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}

	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	targetValue := strings.TrimSpace(req.TargetValue)
	targetLabel := strings.TrimSpace(req.TargetLabel)
	if targetType == "" {
		targetType = policy.TargetType
		targetValue = policy.TargetValue
		targetLabel = policy.TargetLabel
	}
	switch targetType {
	case nocpass.TargetDevice:
		if targetValue == "" {
			return errors.New("device target requires target_value")
		}
		targetValue = strings.TrimSpace(targetValue)
		if targetLabel == "" {
			rows, _ := h.nocDataRepo.ListAll()
			for i := range rows {
				if strings.EqualFold(strings.TrimSpace(rows[i].Host), targetValue) {
					targetLabel = nocpass.DefaultTargetLabel(targetType, targetValue, &rows[i])
					break
				}
			}
		}
	default:
		normalizedType, normalizedValue, err := nocpass.NormalizeGroupTarget(targetType, targetValue)
		if err != nil {
			return err
		}
		targetType = normalizedType
		targetValue = normalizedValue
		if targetLabel == "" {
			targetLabel = nocpass.DefaultTargetLabel(targetType, targetValue, nil)
		}
	}
	policy.TargetType = targetType
	policy.TargetValue = targetValue
	policy.TargetLabel = targetLabel

	if mode := strings.ToLower(strings.TrimSpace(req.PasswordMode)); mode != "" {
		if mode != "random" && mode != "manual" {
			return errors.New("password_mode must be random or manual")
		}
		policy.PasswordMode = mode
	}
	if policy.PasswordMode == "" {
		policy.PasswordMode = "random"
	}

	if policy.PasswordMode == "manual" {
		if strings.TrimSpace(req.ManualFiberxPassword) != "" {
			enc, err := crypto.Encrypt(h.key, req.ManualFiberxPassword)
			if err != nil {
				return errors.New("encrypt fiberx manual password")
			}
			policy.EncManualFiberxPassword = enc
		}
		if strings.TrimSpace(req.ManualSupportPassword) != "" {
			enc, err := crypto.Encrypt(h.key, req.ManualSupportPassword)
			if err != nil {
				return errors.New("encrypt support manual password")
			}
			policy.EncManualSupportPassword = enc
		}
		if len(policy.EncManualFiberxPassword) == 0 && len(policy.EncManualPassword) > 0 {
			policy.EncManualFiberxPassword = append([]byte(nil), policy.EncManualPassword...)
		}
		if len(policy.EncManualSupportPassword) == 0 && len(policy.EncManualPassword) > 0 {
			policy.EncManualSupportPassword = append([]byte(nil), policy.EncManualPassword...)
		}
		if len(policy.EncManualFiberxPassword) == 0 || len(policy.EncManualSupportPassword) == 0 {
			return errors.New("manual passwords are required for fiberx and support")
		}
	}
	if strings.TrimSpace(policy.Name) == "" {
		policy.Name = policy.TargetLabel
	}
	return nil
}

// ListDevices GET /api/noc-pass/devices?q=
func (h *NocPassHandler) ListDevices(c *gin.Context) {
	q := c.Query("q")
	rows, err := h.listNocDataDevices(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	statusByHost := h.statusLookup()
	out := make([]nocPassDeviceDTO, 0, len(rows))
	for i := range rows {
		row := rows[i]
		host := strings.ToLower(strings.TrimSpace(row.Host))
		if host == "" {
			continue
		}
		out = append(out, toNocPassDeviceDTO(&row, statusByHost[host]))
	}
	c.JSON(http.StatusOK, gin.H{"devices": out})
}

// ListPolicies GET /api/noc-pass/policies
func (h *NocPassHandler) ListPolicies(c *gin.Context) {
	policies, err := h.repo.ListPolicies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocPassPolicyDTO, 0, len(policies))
	for i := range policies {
		out = append(out, policyToDTO(&policies[i], h.key))
	}
	c.JSON(http.StatusOK, gin.H{"policies": out})
}

// CreatePolicy POST /api/noc-pass/policies
func (h *NocPassHandler) CreatePolicy(c *gin.Context) {
	policy := &models.NocPassPolicy{
		Name:         "NOC PASS Policy",
		Enabled:      false,
		TargetType:   nocpass.TargetAllNetworks,
		TargetValue:  "all",
		TargetLabel:  "All Networks",
		PasswordMode: "random",
		LastStatus:   "pending",
	}
	if err := h.repo.CreatePolicy(policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"policy": policyToDTO(policy, h.key)})
}

// UpdatePolicy PUT /api/noc-pass/policies/:id
func (h *NocPassHandler) UpdatePolicy(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name                  string `json:"name"`
		Enabled               *bool  `json:"enabled"`
		TargetType            string `json:"target_type"`
		TargetValue           string `json:"target_value"`
		TargetLabel           string `json:"target_label"`
		PasswordMode          string `json:"password_mode"`
		ManualFiberxPassword  string `json:"manual_fiberx_password"`
		ManualSupportPassword string `json:"manual_support_password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	policy, err := h.repo.GetPolicy(uint(id64))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if err := h.normalizePolicyInput(policy, &req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.repo.SavePolicy(policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"policy": policyToDTO(policy, h.key)})
}

// DeletePolicy DELETE /api/noc-pass/policies/:id
func (h *NocPassHandler) DeletePolicy(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	policies, err := h.repo.ListPolicies()
	if err == nil && len(policies) <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one policy must remain"})
		return
	}
	if err := h.repo.DeletePolicy(uint(id64)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// RunPolicyNow POST /api/noc-pass/policies/:id/run
func (h *NocPassHandler) RunPolicyNow(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	summary, err := nocpass.RunPolicy(h.repo, h.nocDataRepo, h.key, uint(id64), time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "summary": summary})
		return
	}
	policy, _ := h.repo.GetPolicy(uint(id64))
	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"summary": summary,
		"policy":  policyToDTO(policy, h.key),
	})
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
		out = append(out, nocPassKeepUserDTO{ID: list[i].ID, Username: list[i].Username})
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

// CreateKeepUser POST /api/noc-pass/keep-users
func (h *NocPassHandler) CreateKeepUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	display := strings.TrimSpace(req.Username)
	canonical := nocpass.NormalizeUsername(req.Username)
	if canonical == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	if canonical == nocpass.NormalizeUsername(nocpass.UserFiberx) || canonical == nocpass.NormalizeUsername(nocpass.UserSupport) || canonical == nocpass.NormalizeUsername(nocpass.UserDev) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is already protected by default"})
		return
	}
	user := &models.NocPassKeepUser{Username: display, CanonicalUsername: canonical}
	if err := h.repo.CreateKeepUser(user); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "already exists") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": nocPassKeepUserDTO{ID: user.ID, Username: user.Username}})
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

// ListSavedUsers GET /api/noc-pass/saved-users
func (h *NocPassHandler) ListSavedUsers(c *gin.Context) {
	list, err := h.repo.ListSavedUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.nocDataRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocPassSavedUserDTO, 0, len(list))
	for i := range list {
		out = append(out, h.savedUserDTO(&list[i], rows))
	}
	c.JSON(http.StatusOK, gin.H{"users": out})
}

func (h *NocPassHandler) ListExclusions(c *gin.Context) {
	list, err := h.repo.ListExclusions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]nocPassExclusionDTO, 0, len(list))
	for _, item := range list {
		out = append(out, nocPassExclusionDTO{
			ID:     item.ID,
			Subnet: item.Subnet,
			Target: item.Target,
		})
	}
	c.JSON(http.StatusOK, gin.H{"exclusions": out})
}

func (h *NocPassHandler) CreateExclusion(c *gin.Context) {
	var req struct {
		Subnet string `json:"subnet" binding:"required"`
		Target string `json:"target" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	target := nocpass.NormalizeExclusionTarget(req.Target)
	if err := nocpass.ValidateIPv4Spec(strings.TrimSpace(req.Subnet), target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	item := &models.NocPassExclusion{
		Subnet: strings.TrimSpace(req.Subnet),
		Target: target,
	}
	if err := h.repo.CreateExclusion(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"exclusion": nocPassExclusionDTO{
		ID:     item.ID,
		Subnet: item.Subnet,
		Target: item.Target,
	}})
}

func (h *NocPassHandler) DeleteExclusion(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.DeleteExclusion(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// CreateSavedUser POST /api/noc-pass/saved-users
func (h *NocPassHandler) CreateSavedUser(c *gin.Context) {
	var req struct {
		Username     string   `json:"username" binding:"required"`
		Password     string   `json:"password" binding:"required"`
		Privilege    string   `json:"privilege"`
		NetworkTypes []string `json:"network_types"`
		Provinces    []string `json:"provinces"`
		Vendors      []string `json:"vendors"`
		Models       []string `json:"models"`
		Devices      []string `json:"devices"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	display := strings.TrimSpace(req.Username)
	canonical := nocpass.NormalizeUsername(req.Username)
	if canonical == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password required"})
		return
	}

	targetCount := len(req.NetworkTypes) + len(req.Provinces) + len(req.Vendors) + len(req.Models) + len(req.Devices)
	if targetCount == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "select at least one network type, province, vendor, model, or device"})
		return
	}

	enc, err := crypto.Encrypt(h.key, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encrypt password"})
		return
	}

	item := &models.NocPassSavedUser{
		Username:          display,
		CanonicalUsername: canonical,
		Privilege:         nocpass.NormalizeSavedUserPrivilege(req.Privilege),
		EncPassword:       enc,
		NetworkTypesJSON:  nocpass.EncodeSavedUserList(req.NetworkTypes),
		ProvincesJSON:     nocpass.EncodeSavedUserList(req.Provinces),
		VendorsJSON:       nocpass.EncodeSavedUserList(req.Vendors),
		ModelsJSON:        nocpass.EncodeSavedUserList(req.Models),
		DevicesJSON:       nocpass.EncodeSavedUserList(req.Devices),
	}
	if err := h.repo.CreateSavedUser(item); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	rows, err := h.nocDataRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows := make([]models.NocDataDevice, 0)
	for i := range rows {
		if !nocpass.SavedUserMatchesRow(item, &rows[i]) {
			continue
		}
		targetRows = append(targetRows, rows[i])
	}
	failures := make([]string, 0)
	for i := range targetRows {
		if err := nocpass.ApplySavedUserToDevice(h.repo, h.nocDataRepo, h.key, &targetRows[i], item); err != nil {
			failures = append(failures, targetRows[i].Host+": "+err.Error())
		}
		if i < len(targetRows)-1 {
			time.Sleep(nocpass.DeviceApplyGap)
		}
	}
	dto := h.savedUserDTO(item, rows)
	if len(failures) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": strings.Join(failures, "; "), "user": dto})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": dto})
}

// DeleteSavedUser DELETE /api/noc-pass/saved-users/:id
func (h *NocPassHandler) DeleteSavedUser(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	item, err := h.repo.GetSavedUser(uint(id64))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "saved user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rows, err := h.nocDataRepo.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	targetRows := make([]models.NocDataDevice, 0)
	for i := range rows {
		if !nocpass.SavedUserMatchesRow(item, &rows[i]) {
			continue
		}
		targetRows = append(targetRows, rows[i])
	}
	failures := make([]string, 0)
	for i := range targetRows {
		if err := nocpass.DeleteSavedUserFromDevice(h.repo, h.nocDataRepo, h.key, &targetRows[i], item); err != nil {
			failures = append(failures, targetRows[i].Host+": "+err.Error())
		}
		if i < len(targetRows)-1 {
			time.Sleep(nocpass.DeviceApplyGap)
		}
	}
	if len(failures) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": strings.Join(failures, "; ")})
		return
	}
	if err := h.repo.DeleteSavedUser(item.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Deprecated legacy endpoints retained only to avoid router breakage during rollout.
func (h *NocPassHandler) CreateDevice(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "manual NOC PASS device registry has been replaced by NOC Data-backed policy mode"})
}

func (h *NocPassHandler) DeleteDevice(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "manual NOC PASS device registry has been replaced by NOC Data-backed policy mode"})
}

func (h *NocPassHandler) Credential(c *gin.Context) {
	id64, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	state, err := h.repo.GetByID(uint(id64))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	password := ""
	if len(state.EncNocPassword) > 0 {
		if plain, err := crypto.Decrypt(h.key, state.EncNocPassword); err == nil {
			password = plain
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"accounts":            fixedAccountsDTO(),
		"password":            password,
		"pending":             len(state.EncNocPassword) == 0,
		"password_rotated_at": state.PasswordRotatedAt,
		"last_apply_ok":       state.LastApplyOK,
		"last_apply_error":    state.LastApplyError,
	})
}

func (h *NocPassHandler) RotateNow(c *gin.Context) {
	c.JSON(http.StatusGone, gin.H{"error": "use the per-policy apply action instead"})
}

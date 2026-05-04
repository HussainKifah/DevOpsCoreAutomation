package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	auth "github.com/Flafl/DevOpsCore/internal/Auth"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type IPCapacityHandler struct {
	repo     repository.IPCapacityRepository
	costAuth func(email string) bool
}

func NewIPCapacityHandler(repo repository.IPCapacityRepository, cfg ...*config.Config) *IPCapacityHandler {
	h := &IPCapacityHandler{repo: repo, costAuth: func(string) bool { return false }}
	if len(cfg) > 0 && cfg[0] != nil {
		h.costAuth = cfg[0].CanViewIPCapacityCost
	}
	return h
}

type ipCapacityNodeRequest struct {
	Name               string `json:"name"`
	Type               string `json:"type"`
	Province           string `json:"province"`
	InitialCapacityIQD int64  `json:"initial_capacity_iqd"`
}

type ipCapacityActionRequest struct {
	NodeID         uint   `json:"node_id"`
	Type           string `json:"type"`
	AmountIQD      int64  `json:"amount_iqd"`
	CostPerMbpsIQD int64  `json:"cost_per_mbps_iqd"`
	ActionAt       string `json:"action_at"`
}

type ipCapacityImportRequest struct {
	Rows []ipCapacityImportRowRequest `json:"rows"`
}

type ipCapacityImportRowRequest struct {
	Name                    string `json:"name"`
	Type                    string `json:"type"`
	Province                string `json:"province"`
	CapacityBeforeUpdateIQD int64  `json:"capacity_before_update_iqd"`
	Action                  string `json:"action"`
	DifferenceIQD           int64  `json:"difference_iqd"`
	CostPerMbpsIQD          int64  `json:"cost_per_mbps_iqd"`
	ActionAt                string `json:"action_at"`
}

func (h *IPCapacityHandler) ListNodes(c *gin.Context) {
	nodes, err := h.repo.ListNodes(c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	h.redactNodeCosts(nodes, canViewCost)
	c.JSON(http.StatusOK, gin.H{"nodes": nodes, "can_view_cost": canViewCost})
}

func (h *IPCapacityHandler) CreateNode(c *gin.Context) {
	var req ipCapacityNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	node := &models.IPCapacityNode{
		Name:               strings.TrimSpace(req.Name),
		Type:               strings.TrimSpace(req.Type),
		Province:           strings.TrimSpace(req.Province),
		InitialCapacityIQD: req.InitialCapacityIQD,
	}
	if node.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if node.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type required"})
		return
	}
	if node.Province == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "province required"})
		return
	}
	if node.InitialCapacityIQD < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "initial_capacity_iqd cannot be negative"})
		return
	}
	if err := h.repo.CreateNode(node); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"node": node})
}

func (h *IPCapacityHandler) UpdateNode(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req ipCapacityNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	node := &models.IPCapacityNode{
		ID:                 id,
		Name:               strings.TrimSpace(req.Name),
		Type:               strings.TrimSpace(req.Type),
		Province:           strings.TrimSpace(req.Province),
		InitialCapacityIQD: req.InitialCapacityIQD,
	}
	if node.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	if node.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type required"})
		return
	}
	if node.Province == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "province required"})
		return
	}
	if node.InitialCapacityIQD < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "initial_capacity_iqd cannot be negative"})
		return
	}
	if err := h.repo.UpdateNode(node); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	updated, err := h.repo.GetNode(id)
	if err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"node": updated})
}

func (h *IPCapacityHandler) DeleteNode(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.repo.DeleteNode(id); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *IPCapacityHandler) ListActions(c *gin.Context) {
	actions, err := h.repo.ListActions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	h.redactActionCosts(actions, canViewCost)
	c.JSON(http.StatusOK, gin.H{"actions": actions, "can_view_cost": canViewCost})
}

func (h *IPCapacityHandler) CreateAction(c *gin.Context) {
	canViewCost := h.canViewCost(c)
	action, ok := h.bindAction(c, 0, nil, canViewCost)
	if !ok {
		return
	}
	if err := h.repo.CreateAction(action); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	created, err := h.repo.GetAction(action.ID)
	if err == nil {
		action = created
	}
	if !canViewCost {
		action.CostPerMbpsIQD = 0
		action.TotalCostIQD = 0
	}
	c.JSON(http.StatusCreated, gin.H{"action": action})
}

func (h *IPCapacityHandler) UpdateAction(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	existing, err := h.repo.GetAction(id)
	if err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	action, ok := h.bindAction(c, existing.NodeID, existing, canViewCost)
	if !ok {
		return
	}
	action.ID = id
	if err := h.repo.UpdateAction(action); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	updated, err := h.repo.GetAction(id)
	if err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	if !canViewCost {
		updated.CostPerMbpsIQD = 0
		updated.TotalCostIQD = 0
	}
	c.JSON(http.StatusOK, gin.H{"action": updated})
}

func (h *IPCapacityHandler) DeleteAction(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.repo.DeleteAction(id); err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *IPCapacityHandler) ImportActions(c *gin.Context) {
	var req ipCapacityImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	rows := make([]repository.IPCapacityImportRow, 0, len(req.Rows))
	for i, row := range req.Rows {
		actionAt, err := parseCapacityActionTime(row.ActionAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "row " + strconv.Itoa(i+1) + ": action_at must be RFC3339, YYYY-MM-DD HH:MM, or Jan 02, 2006"})
			return
		}
		rows = append(rows, repository.IPCapacityImportRow{
			Name:                    strings.TrimSpace(row.Name),
			Type:                    strings.TrimSpace(row.Type),
			Province:                strings.TrimSpace(row.Province),
			CapacityBeforeUpdateIQD: row.CapacityBeforeUpdateIQD,
			ActionType:              strings.TrimSpace(strings.ToLower(row.Action)),
			AmountIQD:               absInt64(row.DifferenceIQD),
			CostPerMbpsIQD:          costPerMbpsForWrite(row.CostPerMbpsIQD, canViewCost),
			ActionAt:                actionAt,
		})
	}
	result, err := h.repo.ImportActions(rows)
	if err != nil {
		c.JSON(statusForCapacityError(err), gin.H{"error": err.Error()})
		return
	}
	nodes, err := h.repo.ListNodes("")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.redactNodeCosts(nodes, canViewCost)
	actions, err := h.repo.ListActions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.redactActionCosts(actions, canViewCost)
	c.JSON(http.StatusOK, gin.H{"result": result, "nodes": nodes, "actions": actions, "can_view_cost": canViewCost})
}

func (h *IPCapacityHandler) ListHistoryDays(c *gin.Context) {
	days, err := h.repo.ListHistoryDays()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"days": days})
}

func (h *IPCapacityHandler) GetDayHistory(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("date"))
	if raw == "" {
		raw = time.Now().In(time.Local).Add(-3 * time.Hour).Format("2006-01-02")
	}
	day, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "date must be YYYY-MM-DD"})
		return
	}
	history, err := h.repo.GetDayHistory(day)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	h.redactDayHistoryCosts(history, canViewCost)
	c.JSON(http.StatusOK, gin.H{"date": raw, "history": history, "can_view_cost": canViewCost})
}

func (h *IPCapacityHandler) GetAllHistory(c *gin.Context) {
	history, err := h.repo.GetAllHistory()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	canViewCost := h.canViewCost(c)
	h.redactSnapshotCosts(history, canViewCost)
	c.JSON(http.StatusOK, gin.H{"history": history, "can_view_cost": canViewCost})
}

func (h *IPCapacityHandler) bindAction(c *gin.Context, existingNodeID uint, existing *models.IPCapacityAction, canViewCost bool) (*models.IPCapacityAction, bool) {
	var req ipCapacityActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return nil, false
	}
	nodeID := req.NodeID
	if existingNodeID != 0 {
		if nodeID != 0 && nodeID != existingNodeID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "node_id cannot be changed"})
			return nil, false
		}
		nodeID = existingNodeID
	}
	action := &models.IPCapacityAction{
		NodeID:         nodeID,
		Type:           strings.TrimSpace(strings.ToLower(req.Type)),
		AmountIQD:      req.AmountIQD,
		CostPerMbpsIQD: costPerMbpsForWrite(req.CostPerMbpsIQD, canViewCost),
		ActionAt:       time.Now(),
	}
	if !canViewCost && existing != nil {
		action.CostPerMbpsIQD = existing.CostPerMbpsIQD
	}
	if strings.TrimSpace(req.ActionAt) != "" {
		parsed, err := parseCapacityActionTime(req.ActionAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "action_at must be RFC3339 or YYYY-MM-DD HH:MM"})
			return nil, false
		}
		action.ActionAt = parsed
	}
	if action.NodeID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "node_id required"})
		return nil, false
	}
	if action.Type != models.IPCapacityActionUpgrade && action.Type != models.IPCapacityActionDowngrade {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be upgrade or downgrade"})
		return nil, false
	}
	if action.AmountIQD <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "amount_iqd must be greater than 0"})
		return nil, false
	}
	if canViewCost && action.CostPerMbpsIQD < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cost_per_mbps_iqd cannot be negative"})
		return nil, false
	}
	return action, true
}

func (h *IPCapacityHandler) canViewCost(c *gin.Context) bool {
	if h == nil || h.costAuth == nil {
		return false
	}
	user, ok := c.Get("user")
	if !ok {
		return false
	}
	claims, ok := user.(*auth.Claims)
	if !ok {
		return false
	}
	return h.costAuth(claims.Email)
}

func (h *IPCapacityHandler) redactNodeCosts(nodes []repository.IPCapacityNodeWithLatest, canViewCost bool) {
	if canViewCost {
		return
	}
	for i := range nodes {
		if nodes[i].LatestAction != nil {
			nodes[i].LatestAction.CostPerMbpsIQD = 0
			nodes[i].LatestAction.TotalCostIQD = 0
		}
	}
}

func (h *IPCapacityHandler) redactActionCosts(actions []repository.IPCapacityActionWithNode, canViewCost bool) {
	if canViewCost {
		return
	}
	for i := range actions {
		actions[i].CostPerMbpsIQD = 0
		actions[i].TotalCostIQD = 0
	}
}

func (h *IPCapacityHandler) redactDayHistoryCosts(history *repository.IPCapacityDayHistory, canViewCost bool) {
	if canViewCost || history == nil {
		return
	}
	for i := range history.Summaries {
		history.Summaries[i].TotalCostIQD = 0
	}
	h.redactActionCosts(history.Actions, false)
}

func (h *IPCapacityHandler) redactSnapshotCosts(history []repository.IPCapacityHistorySnapshot, canViewCost bool) {
	if canViewCost {
		return
	}
	for i := range history {
		history[i].TotalCostIQD = 0
	}
}

func costPerMbpsForWrite(value int64, canViewCost bool) int64 {
	if !canViewCost {
		return 0
	}
	return value
}

func parseCapacityActionTime(raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.In(time.Local), nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", raw, time.Local); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04", raw, time.Local); err == nil {
		return t, nil
	}
	return time.ParseInLocation("Jan 02, 2006", raw, time.Local)
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	id64, err := strconv.ParseUint(c.Param(name), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return 0, false
	}
	return uint(id64), true
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func statusForCapacityError(err error) int {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return http.StatusNotFound
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "duplicate") || strings.Contains(lower, "unique") || strings.Contains(lower, "already exists") {
		return http.StatusConflict
	}
	if strings.Contains(lower, "required") || strings.Contains(lower, "cannot") || strings.Contains(lower, "greater than") || strings.Contains(lower, "invalid") || strings.Contains(lower, "no import rows") {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

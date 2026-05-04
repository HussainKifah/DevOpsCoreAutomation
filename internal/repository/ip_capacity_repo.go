package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type IPCapacityNodeWithLatest struct {
	models.IPCapacityNode
	LatestAction *models.IPCapacityAction `json:"latest_action,omitempty"`
}

type IPCapacityActionWithNode struct {
	models.IPCapacityAction
	NodeName     string `json:"node_name"`
	NodeType     string `json:"node_type"`
	NodeProvince string `json:"node_province"`
}

type IPCapacityNodeDaySummary struct {
	NodeID             uint      `json:"node_id"`
	NodeName           string    `json:"node_name"`
	NodeType           string    `json:"node_type"`
	NodeProvince       string    `json:"node_province"`
	OpeningCapacityIQD int64     `json:"opening_capacity_iqd"`
	ClosingCapacityIQD int64     `json:"closing_capacity_iqd"`
	DifferenceIQD      int64     `json:"difference_iqd"`
	TotalCostIQD       int64     `json:"total_cost_iqd"`
	LatestActionAt     time.Time `json:"latest_action_at"`
}

type IPCapacityDayHistory struct {
	Summaries []IPCapacityNodeDaySummary `json:"summaries"`
	Actions   []IPCapacityActionWithNode `json:"actions"`
}

type IPCapacityHistorySnapshot struct {
	Day                string    `json:"day"`
	NodeID             uint      `json:"node_id"`
	NodeName           string    `json:"node_name"`
	NodeType           string    `json:"node_type"`
	NodeProvince       string    `json:"node_province"`
	CurrentCapacityIQD int64     `json:"current_capacity_iqd"`
	TotalCostIQD       int64     `json:"total_cost_iqd"`
	LatestActionAt     time.Time `json:"latest_action_at"`
}

type IPCapacityImportRow struct {
	Name                    string
	Type                    string
	Province                string
	CapacityBeforeUpdateIQD int64
	ActionType              string
	AmountIQD               int64
	CostPerMbpsIQD          int64
	ActionAt                time.Time
}

type IPCapacityImportResult struct {
	ImportedActions int `json:"imported_actions"`
	CreatedNodes    int `json:"created_nodes"`
}

type IPCapacityRepository interface {
	ListNodes(search string) ([]IPCapacityNodeWithLatest, error)
	CreateNode(node *models.IPCapacityNode) error
	GetNode(id uint) (*models.IPCapacityNode, error)
	UpdateNode(node *models.IPCapacityNode) error
	DeleteNode(id uint) error
	ListActions() ([]IPCapacityActionWithNode, error)
	CreateAction(action *models.IPCapacityAction) error
	GetAction(id uint) (*models.IPCapacityAction, error)
	UpdateAction(action *models.IPCapacityAction) error
	DeleteAction(id uint) error
	ImportActions(rows []IPCapacityImportRow) (*IPCapacityImportResult, error)
	ListHistoryDays() ([]string, error)
	GetDayHistory(day time.Time) (*IPCapacityDayHistory, error)
	GetAllHistory() ([]IPCapacityHistorySnapshot, error)
}

type ipCapacityRepo struct {
	db *gorm.DB
}

func NewIPCapacityRepository(db *gorm.DB) IPCapacityRepository {
	return &ipCapacityRepo{db: db}
}

func normalizeCapacityNode(node *models.IPCapacityNode) {
	if node == nil {
		return
	}
	node.Name = strings.TrimSpace(node.Name)
	node.Type = strings.TrimSpace(node.Type)
	node.Province = strings.TrimSpace(node.Province)
	if node.CurrentCapacityIQD == 0 {
		node.CurrentCapacityIQD = node.InitialCapacityIQD
	}
}

func validateCapacityAction(action *models.IPCapacityAction) error {
	if action == nil {
		return errors.New("nil action")
	}
	if action.NodeID == 0 {
		return errors.New("node_id required")
	}
	if action.Type != models.IPCapacityActionUpgrade && action.Type != models.IPCapacityActionDowngrade {
		return errors.New("type must be upgrade or downgrade")
	}
	if action.AmountIQD <= 0 {
		return errors.New("amount_iqd must be greater than 0")
	}
	if action.CostPerMbpsIQD < 0 {
		return errors.New("cost_per_mbps_iqd cannot be negative")
	}
	if action.ActionAt.IsZero() {
		action.ActionAt = time.Now()
	}
	return nil
}

func (r *ipCapacityRepo) ListNodes(search string) ([]IPCapacityNodeWithLatest, error) {
	var nodes []models.IPCapacityNode
	q := strings.TrimSpace(search)
	tx := r.db.Model(&models.IPCapacityNode{})
	if q != "" {
		pat := "%" + strings.ToLower(q) + "%"
		tx = tx.Where("LOWER(province) LIKE ? OR LOWER(name) LIKE ? OR LOWER(type) LIKE ? OR CAST(current_capacity_iqd AS TEXT) LIKE ? OR CAST(initial_capacity_iqd AS TEXT) LIKE ?", pat, pat, pat, pat, pat)
	}
	if err := tx.Order("province ASC, name ASC, type ASC, id ASC").Find(&nodes).Error; err != nil {
		return nil, err
	}
	out := make([]IPCapacityNodeWithLatest, 0, len(nodes))
	for i := range nodes {
		row := IPCapacityNodeWithLatest{IPCapacityNode: nodes[i]}
		var action models.IPCapacityAction
		err := r.db.Where("node_id = ?", nodes[i].ID).Order("action_at DESC, id DESC").First(&action).Error
		if err == nil {
			row.LatestAction = &action
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		out = append(out, row)
	}
	return out, nil
}

func (r *ipCapacityRepo) CreateNode(node *models.IPCapacityNode) error {
	if node == nil {
		return errors.New("nil node")
	}
	normalizeCapacityNode(node)
	if node.Name == "" {
		return errors.New("name required")
	}
	if node.Type == "" {
		return errors.New("type required")
	}
	if node.Province == "" {
		return errors.New("province required")
	}
	if node.InitialCapacityIQD < 0 {
		return errors.New("initial_capacity_iqd cannot be negative")
	}
	node.CurrentCapacityIQD = node.InitialCapacityIQD
	return r.db.Create(node).Error
}

func (r *ipCapacityRepo) GetNode(id uint) (*models.IPCapacityNode, error) {
	var node models.IPCapacityNode
	if err := r.db.First(&node, id).Error; err != nil {
		return nil, err
	}
	return &node, nil
}

func (r *ipCapacityRepo) UpdateNode(node *models.IPCapacityNode) error {
	if node == nil {
		return errors.New("nil node")
	}
	normalizeCapacityNode(node)
	if node.ID == 0 {
		return errors.New("id required")
	}
	if node.Name == "" {
		return errors.New("name required")
	}
	if node.Type == "" {
		return errors.New("type required")
	}
	if node.Province == "" {
		return errors.New("province required")
	}
	if node.InitialCapacityIQD < 0 {
		return errors.New("initial_capacity_iqd cannot be negative")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing models.IPCapacityNode
		if err := tx.First(&existing, node.ID).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.IPCapacityNode{}).Where("id = ?", node.ID).Updates(map[string]interface{}{
			"name":                 node.Name,
			"type":                 node.Type,
			"province":             node.Province,
			"initial_capacity_iqd": node.InitialCapacityIQD,
		}).Error; err != nil {
			return err
		}
		return recalculateCapacityNode(tx, node.ID)
	})
}

func (r *ipCapacityRepo) DeleteNode(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("node_id = ?", id).Delete(&models.IPCapacityAction{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&models.IPCapacityNode{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (r *ipCapacityRepo) ListActions() ([]IPCapacityActionWithNode, error) {
	var actions []models.IPCapacityAction
	if err := r.db.Preload("Node").Order("action_at DESC, id DESC").Find(&actions).Error; err != nil {
		return nil, err
	}
	return actionsWithNodeNames(actions), nil
}

func (r *ipCapacityRepo) CreateAction(action *models.IPCapacityAction) error {
	if err := validateCapacityAction(action); err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var node models.IPCapacityNode
		if err := tx.First(&node, action.NodeID).Error; err != nil {
			return err
		}
		if err := tx.Create(action).Error; err != nil {
			return err
		}
		return recalculateCapacityNode(tx, action.NodeID)
	})
}

func (r *ipCapacityRepo) GetAction(id uint) (*models.IPCapacityAction, error) {
	var action models.IPCapacityAction
	if err := r.db.First(&action, id).Error; err != nil {
		return nil, err
	}
	return &action, nil
}

func (r *ipCapacityRepo) UpdateAction(action *models.IPCapacityAction) error {
	if err := validateCapacityAction(action); err != nil {
		return err
	}
	if action.ID == 0 {
		return errors.New("id required")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		var existing models.IPCapacityAction
		if err := tx.First(&existing, action.ID).Error; err != nil {
			return err
		}
		if existing.NodeID != action.NodeID {
			return errors.New("node_id cannot be changed")
		}
		if err := tx.Model(&models.IPCapacityAction{}).Where("id = ?", action.ID).Updates(map[string]interface{}{
			"type":              action.Type,
			"amount_iqd":        action.AmountIQD,
			"cost_per_mbps_iqd": action.CostPerMbpsIQD,
			"action_at":         action.ActionAt,
		}).Error; err != nil {
			return err
		}
		return recalculateCapacityNode(tx, action.NodeID)
	})
}

func (r *ipCapacityRepo) DeleteAction(id uint) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		var action models.IPCapacityAction
		if err := tx.First(&action, id).Error; err != nil {
			return err
		}
		if err := tx.Delete(&models.IPCapacityAction{}, id).Error; err != nil {
			return err
		}
		return recalculateCapacityNode(tx, action.NodeID)
	})
}

func (r *ipCapacityRepo) ImportActions(rows []IPCapacityImportRow) (*IPCapacityImportResult, error) {
	if len(rows) == 0 {
		return nil, errors.New("no import rows provided")
	}
	result := &IPCapacityImportResult{}
	return result, r.db.Transaction(func(tx *gorm.DB) error {
		recalcNodeIDs := make(map[uint]struct{})
		for i := range rows {
			row := rows[i]
			row.Name = strings.TrimSpace(row.Name)
			row.Type = strings.TrimSpace(row.Type)
			row.Province = strings.TrimSpace(row.Province)
			row.ActionType = strings.TrimSpace(strings.ToLower(row.ActionType))
			if row.Name == "" {
				return errors.New("name required")
			}
			if row.Type == "" {
				return errors.New("type required")
			}
			if row.Province == "" {
				return errors.New("province required")
			}
			if row.CapacityBeforeUpdateIQD < 0 {
				return errors.New("capacity before update cannot be negative")
			}
			action := models.IPCapacityAction{
				Type:           row.ActionType,
				AmountIQD:      row.AmountIQD,
				CostPerMbpsIQD: row.CostPerMbpsIQD,
				ActionAt:       row.ActionAt,
			}

			var node models.IPCapacityNode
			err := tx.
				Where("province = ? AND name = ? AND type = ?", row.Province, row.Name, row.Type).
				First(&node).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				node = models.IPCapacityNode{
					Name:               row.Name,
					Type:               row.Type,
					Province:           row.Province,
					InitialCapacityIQD: row.CapacityBeforeUpdateIQD,
					CurrentCapacityIQD: row.CapacityBeforeUpdateIQD,
				}
				if err := tx.Create(&node).Error; err != nil {
					return err
				}
				result.CreatedNodes++
			} else if err != nil {
				return err
			}

			action.NodeID = node.ID
			if err := validateCapacityAction(&action); err != nil {
				return err
			}
			if err := tx.Create(&action).Error; err != nil {
				return err
			}
			recalcNodeIDs[node.ID] = struct{}{}
			result.ImportedActions++
		}
		for nodeID := range recalcNodeIDs {
			if err := recalculateCapacityNode(tx, nodeID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ipCapacityRepo) ListHistoryDays() ([]string, error) {
	var rows []struct {
		Day time.Time `gorm:"column:day"`
	}
	if err := r.db.Raw("SELECT DISTINCT DATE(action_at - INTERVAL '3 hours') AS day FROM ip_capacity_actions ORDER BY day DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	days := make([]string, 0, len(rows))
	for _, row := range rows {
		days = append(days, row.Day.Format("2006-01-02"))
	}
	return days, nil
}

func (r *ipCapacityRepo) GetDayHistory(day time.Time) (*IPCapacityDayHistory, error) {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 0, 1)
	rawStart := start.Add(3 * time.Hour)
	rawEnd := end.Add(3 * time.Hour)
	var actions []models.IPCapacityAction
	if err := r.db.Preload("Node").
		Where("action_at >= ? AND action_at < ?", rawStart, rawEnd).
		Order("action_at ASC, id ASC").
		Find(&actions).Error; err != nil {
		return nil, err
	}
	var nodes []models.IPCapacityNode
	if err := r.db.
		Order("province ASC, name ASC, type ASC, id ASC").
		Find(&nodes).Error; err != nil {
		return nil, err
	}
	return r.buildFullDayHistory(nodes, actions, rawStart, rawEnd)
}

func (r *ipCapacityRepo) GetAllHistory() ([]IPCapacityHistorySnapshot, error) {
	days, err := r.ListHistoryDays()
	if err != nil {
		return nil, err
	}
	snapshots := make([]IPCapacityHistorySnapshot, 0)
	for _, rawDay := range days {
		day, err := time.ParseInLocation("2006-01-02", rawDay, time.Local)
		if err != nil {
			return nil, err
		}
		history, err := r.GetDayHistory(day)
		if err != nil {
			return nil, err
		}
		for _, summary := range history.Summaries {
			snapshots = append(snapshots, IPCapacityHistorySnapshot{
				Day:                rawDay,
				NodeID:             summary.NodeID,
				NodeName:           summary.NodeName,
				NodeType:           summary.NodeType,
				NodeProvince:       summary.NodeProvince,
				CurrentCapacityIQD: summary.ClosingCapacityIQD,
				TotalCostIQD:       summary.TotalCostIQD,
				LatestActionAt:     summary.LatestActionAt,
			})
		}
	}
	return snapshots, nil
}

func (r *ipCapacityRepo) buildFullDayHistory(nodes []models.IPCapacityNode, actions []models.IPCapacityAction, rawStart, rawEnd time.Time) (*IPCapacityDayHistory, error) {
	history := &IPCapacityDayHistory{
		Summaries: make([]IPCapacityNodeDaySummary, 0, len(nodes)),
		Actions:   actionsWithNodeNames(actions),
	}
	dayActionByNode := make(map[uint]models.IPCapacityAction)
	for _, action := range actions {
		dayActionByNode[action.NodeID] = action
	}
	for _, node := range nodes {
		opening := node.InitialCapacityIQD
		var beforeDay models.IPCapacityAction
		err := r.db.
			Where("node_id = ? AND action_at < ?", node.ID, rawStart).
			Order("action_at DESC, id DESC").
			First(&beforeDay).Error
		if err == nil {
			opening = beforeDay.CapacityAfterIQD
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}

		closing := opening
		latestAt := time.Time{}
		var beforeEnd models.IPCapacityAction
		err = r.db.
			Where("node_id = ? AND action_at < ?", node.ID, rawEnd).
			Order("action_at DESC, id DESC").
			First(&beforeEnd).Error
		if err == nil {
			closing = beforeEnd.CapacityAfterIQD
			latestAt = beforeEnd.ActionAt
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		if dayAction, ok := dayActionByNode[node.ID]; ok {
			latestAt = dayAction.ActionAt
		}
		totalCost := int64(0)
		for _, action := range actions {
			if action.NodeID == node.ID {
				totalCost += action.TotalCostIQD
			}
		}

		history.Summaries = append(history.Summaries, IPCapacityNodeDaySummary{
			NodeID:             node.ID,
			NodeName:           node.Name,
			NodeType:           node.Type,
			NodeProvince:       node.Province,
			OpeningCapacityIQD: opening,
			ClosingCapacityIQD: closing,
			DifferenceIQD:      closing - opening,
			TotalCostIQD:       totalCost,
			LatestActionAt:     latestAt,
		})
	}
	return history, nil
}

func recalculateCapacityNode(tx *gorm.DB, nodeID uint) error {
	var node models.IPCapacityNode
	if err := tx.First(&node, nodeID).Error; err != nil {
		return err
	}
	var actions []models.IPCapacityAction
	if err := tx.Where("node_id = ?", nodeID).Order("action_at ASC, id ASC").Find(&actions).Error; err != nil {
		return err
	}
	total := node.InitialCapacityIQD
	for i := range actions {
		actions[i].CapacityBeforeIQD = total
		switch actions[i].Type {
		case models.IPCapacityActionUpgrade:
			total += actions[i].AmountIQD
			actions[i].TotalCostIQD = actions[i].AmountIQD * actions[i].CostPerMbpsIQD
		case models.IPCapacityActionDowngrade:
			total -= actions[i].AmountIQD
			actions[i].TotalCostIQD = -actions[i].AmountIQD * actions[i].CostPerMbpsIQD
		default:
			return errors.New("invalid action type")
		}
		actions[i].CapacityAfterIQD = total
		if err := tx.Model(&models.IPCapacityAction{}).Where("id = ?", actions[i].ID).Updates(map[string]interface{}{
			"capacity_before_iqd": actions[i].CapacityBeforeIQD,
			"capacity_after_iqd":  actions[i].CapacityAfterIQD,
			"total_cost_iqd":      actions[i].TotalCostIQD,
		}).Error; err != nil {
			return err
		}
	}
	return tx.Model(&models.IPCapacityNode{}).Where("id = ?", nodeID).Update("current_capacity_iqd", total).Error
}

func actionsWithNodeNames(actions []models.IPCapacityAction) []IPCapacityActionWithNode {
	out := make([]IPCapacityActionWithNode, 0, len(actions))
	for i := range actions {
		out = append(out, IPCapacityActionWithNode{
			IPCapacityAction: actions[i],
			NodeName:         actions[i].Node.Name,
			NodeType:         actions[i].Node.Type,
			NodeProvince:     actions[i].Node.Province,
		})
	}
	return out
}

func buildDayHistory(actions []models.IPCapacityAction) *IPCapacityDayHistory {
	history := &IPCapacityDayHistory{
		Summaries: []IPCapacityNodeDaySummary{},
		Actions:   actionsWithNodeNames(actions),
	}
	byNode := make(map[uint]int)
	for _, action := range actions {
		idx, ok := byNode[action.NodeID]
		if !ok {
			history.Summaries = append(history.Summaries, IPCapacityNodeDaySummary{
				NodeID:             action.NodeID,
				NodeName:           action.Node.Name,
				NodeType:           action.Node.Type,
				NodeProvince:       action.Node.Province,
				OpeningCapacityIQD: action.CapacityBeforeIQD,
				ClosingCapacityIQD: action.CapacityAfterIQD,
				DifferenceIQD:      action.CapacityAfterIQD - action.CapacityBeforeIQD,
				TotalCostIQD:       action.TotalCostIQD,
				LatestActionAt:     action.ActionAt,
			})
			byNode[action.NodeID] = len(history.Summaries) - 1
			continue
		}
		summary := &history.Summaries[idx]
		summary.ClosingCapacityIQD = action.CapacityAfterIQD
		summary.DifferenceIQD = summary.ClosingCapacityIQD - summary.OpeningCapacityIQD
		summary.TotalCostIQD += action.TotalCostIQD
		summary.LatestActionAt = action.ActionAt
	}
	return history
}

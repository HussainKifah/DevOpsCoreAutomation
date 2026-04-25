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
	NodeName string `json:"node_name"`
}

type IPCapacityNodeDaySummary struct {
	NodeID             uint      `json:"node_id"`
	NodeName           string    `json:"node_name"`
	OpeningCapacityIQD int64     `json:"opening_capacity_iqd"`
	ClosingCapacityIQD int64     `json:"closing_capacity_iqd"`
	DifferenceIQD      int64     `json:"difference_iqd"`
	LatestActionAt     time.Time `json:"latest_action_at"`
}

type IPCapacityDayHistory struct {
	Summaries []IPCapacityNodeDaySummary `json:"summaries"`
	Actions   []IPCapacityActionWithNode `json:"actions"`
}

type IPCapacityRepository interface {
	ListNodes(search string) ([]IPCapacityNodeWithLatest, error)
	CreateNode(node *models.IPCapacityNode) error
	GetNode(id uint) (*models.IPCapacityNode, error)
	UpdateNode(node *models.IPCapacityNode) error
	ListActions() ([]IPCapacityActionWithNode, error)
	CreateAction(action *models.IPCapacityAction) error
	GetAction(id uint) (*models.IPCapacityAction, error)
	UpdateAction(action *models.IPCapacityAction) error
	DeleteAction(id uint) error
	ListHistoryDays() ([]string, error)
	GetDayHistory(day time.Time) (*IPCapacityDayHistory, error)
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
		tx = tx.Where("LOWER(name) LIKE ? OR CAST(current_capacity_iqd AS TEXT) LIKE ? OR CAST(initial_capacity_iqd AS TEXT) LIKE ?", pat, pat, pat)
	}
	if err := tx.Order("name ASC, id ASC").Find(&nodes).Error; err != nil {
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
			"initial_capacity_iqd": node.InitialCapacityIQD,
		}).Error; err != nil {
			return err
		}
		return recalculateCapacityNode(tx, node.ID)
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
			"type":       action.Type,
			"amount_iqd": action.AmountIQD,
			"action_at":  action.ActionAt,
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
	return buildDayHistory(actions), nil
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
		case models.IPCapacityActionDowngrade:
			total -= actions[i].AmountIQD
		default:
			return errors.New("invalid action type")
		}
		actions[i].CapacityAfterIQD = total
		if err := tx.Model(&models.IPCapacityAction{}).Where("id = ?", actions[i].ID).Updates(map[string]interface{}{
			"capacity_before_iqd": actions[i].CapacityBeforeIQD,
			"capacity_after_iqd":  actions[i].CapacityAfterIQD,
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
				OpeningCapacityIQD: action.CapacityBeforeIQD,
				ClosingCapacityIQD: action.CapacityAfterIQD,
				DifferenceIQD:      action.CapacityAfterIQD - action.CapacityBeforeIQD,
				LatestActionAt:     action.ActionAt,
			})
			byNode[action.NodeID] = len(history.Summaries) - 1
			continue
		}
		summary := &history.Summaries[idx]
		summary.ClosingCapacityIQD = action.CapacityAfterIQD
		summary.DifferenceIQD = summary.ClosingCapacityIQD - summary.OpeningCapacityIQD
		summary.LatestActionAt = action.ActionAt
	}
	return history
}

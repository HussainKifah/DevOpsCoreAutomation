package models

import "time"

const (
	IPCapacityActionUpgrade   = "upgrade"
	IPCapacityActionDowngrade = "downgrade"
)

// IPCapacityNode is a permanent IP-team capacity ledger node.
type IPCapacityNode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Name               string `gorm:"size:200;not null;uniqueIndex:idx_ip_capacity_nodes_identity" json:"name"`
	Type               string `gorm:"size:80;not null;default:'';index;uniqueIndex:idx_ip_capacity_nodes_identity" json:"type"`
	Province           string `gorm:"size:120;not null;default:'';index;uniqueIndex:idx_ip_capacity_nodes_identity" json:"province"`
	InitialCapacityIQD int64  `gorm:"not null;default:0" json:"initial_capacity_iqd"`
	CurrentCapacityIQD int64  `gorm:"not null;default:0" json:"current_capacity_iqd"`
}

// IPCapacityAction stores an appendable/editable capacity action and its recalculated ledger totals.
type IPCapacityAction struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	NodeID            uint           `gorm:"not null;index" json:"node_id"`
	Node              IPCapacityNode `gorm:"constraint:OnDelete:CASCADE" json:"node,omitempty"`
	Type              string         `gorm:"size:20;not null;index" json:"type"`
	AmountIQD         int64          `gorm:"not null" json:"amount_iqd"`
	CostPerMbpsIQD    int64          `gorm:"not null;default:0" json:"cost_per_mbps_iqd"`
	TotalCostIQD      int64          `gorm:"not null;default:0" json:"total_cost_iqd"`
	CapacityBeforeIQD int64          `gorm:"not null;default:0" json:"capacity_before_iqd"`
	CapacityAfterIQD  int64          `gorm:"not null;default:0" json:"capacity_after_iqd"`
	ActionAt          time.Time      `gorm:"not null;index" json:"action_at"`
}

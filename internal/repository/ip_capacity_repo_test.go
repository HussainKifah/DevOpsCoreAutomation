package repository

import (
	"os"
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func openCapacityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_DSN")
	if dsn == "" {
		t.Skip("TEST_DATABASE_DSN not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&models.IPCapacityNode{}, &models.IPCapacityAction{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Exec("TRUNCATE TABLE ip_capacity_actions, ip_capacity_nodes RESTART IDENTITY CASCADE").Error; err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return db
}

func TestIPCapacityRepositoryRecalculatesActions(t *testing.T) {
	db := openCapacityTestDB(t)
	repo := NewIPCapacityRepository(db)
	base := time.Date(2026, 4, 25, 9, 0, 0, 0, time.Local)

	node := &models.IPCapacityNode{Name: "BNG-01", InitialCapacityIQD: 1000}
	if err := repo.CreateNode(node); err != nil {
		t.Fatalf("create node: %v", err)
	}

	up := &models.IPCapacityAction{NodeID: node.ID, Type: models.IPCapacityActionUpgrade, AmountIQD: 500, ActionAt: base}
	if err := repo.CreateAction(up); err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	down := &models.IPCapacityAction{NodeID: node.ID, Type: models.IPCapacityActionDowngrade, AmountIQD: 200, ActionAt: base.Add(time.Hour)}
	if err := repo.CreateAction(down); err != nil {
		t.Fatalf("create downgrade: %v", err)
	}

	actions, err := repo.ListActions()
	if err != nil {
		t.Fatalf("list actions: %v", err)
	}
	if len(actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(actions))
	}
	if actions[1].CapacityBeforeIQD != 1000 || actions[1].CapacityAfterIQD != 1500 {
		t.Fatalf("upgrade totals = %d/%d, want 1000/1500", actions[1].CapacityBeforeIQD, actions[1].CapacityAfterIQD)
	}
	if actions[0].CapacityBeforeIQD != 1500 || actions[0].CapacityAfterIQD != 1300 {
		t.Fatalf("downgrade totals = %d/%d, want 1500/1300", actions[0].CapacityBeforeIQD, actions[0].CapacityAfterIQD)
	}
	updatedNode, err := repo.GetNode(node.ID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if updatedNode.CurrentCapacityIQD != 1300 {
		t.Fatalf("current total = %d, want 1300", updatedNode.CurrentCapacityIQD)
	}

	down.AmountIQD = 100
	if err := repo.UpdateAction(down); err != nil {
		t.Fatalf("update action: %v", err)
	}
	updatedNode, _ = repo.GetNode(node.ID)
	if updatedNode.CurrentCapacityIQD != 1400 {
		t.Fatalf("current total after edit = %d, want 1400", updatedNode.CurrentCapacityIQD)
	}

	if err := repo.DeleteAction(up.ID); err != nil {
		t.Fatalf("delete action: %v", err)
	}
	updatedNode, _ = repo.GetNode(node.ID)
	if updatedNode.CurrentCapacityIQD != 900 {
		t.Fatalf("current total after delete = %d, want 900", updatedNode.CurrentCapacityIQD)
	}
}

func TestIPCapacityRepositoryDayHistorySummary(t *testing.T) {
	db := openCapacityTestDB(t)
	repo := NewIPCapacityRepository(db)
	day := time.Date(2026, 4, 25, 10, 0, 0, 0, time.Local)

	node := &models.IPCapacityNode{Name: "BNG-02", InitialCapacityIQD: 2000}
	if err := repo.CreateNode(node); err != nil {
		t.Fatalf("create node: %v", err)
	}
	if err := repo.CreateAction(&models.IPCapacityAction{NodeID: node.ID, Type: models.IPCapacityActionUpgrade, AmountIQD: 300, ActionAt: day}); err != nil {
		t.Fatalf("create upgrade: %v", err)
	}
	if err := repo.CreateAction(&models.IPCapacityAction{NodeID: node.ID, Type: models.IPCapacityActionDowngrade, AmountIQD: 100, ActionAt: day.Add(2 * time.Hour)}); err != nil {
		t.Fatalf("create downgrade: %v", err)
	}

	history, err := repo.GetDayHistory(day)
	if err != nil {
		t.Fatalf("day history: %v", err)
	}
	if len(history.Summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(history.Summaries))
	}
	got := history.Summaries[0]
	if got.OpeningCapacityIQD != 2000 || got.ClosingCapacityIQD != 2200 || got.DifferenceIQD != 200 {
		t.Fatalf("summary = %d/%d/%d, want 2000/2200/200", got.OpeningCapacityIQD, got.ClosingCapacityIQD, got.DifferenceIQD)
	}
	if len(history.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(history.Actions))
	}
}

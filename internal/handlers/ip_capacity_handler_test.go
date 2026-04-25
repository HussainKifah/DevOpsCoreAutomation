package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mockIPCapacityRepo struct {
	createActionErr error
	historyDay      time.Time
}

func (m *mockIPCapacityRepo) ListNodes(string) ([]repository.IPCapacityNodeWithLatest, error) {
	return nil, nil
}
func (m *mockIPCapacityRepo) CreateNode(*models.IPCapacityNode) error { return nil }
func (m *mockIPCapacityRepo) GetNode(uint) (*models.IPCapacityNode, error) {
	return &models.IPCapacityNode{}, nil
}
func (m *mockIPCapacityRepo) UpdateNode(*models.IPCapacityNode) error { return nil }
func (m *mockIPCapacityRepo) ListActions() ([]repository.IPCapacityActionWithNode, error) {
	return nil, nil
}
func (m *mockIPCapacityRepo) CreateAction(*models.IPCapacityAction) error {
	return m.createActionErr
}
func (m *mockIPCapacityRepo) GetAction(id uint) (*models.IPCapacityAction, error) {
	return &models.IPCapacityAction{ID: id, NodeID: 1, Type: models.IPCapacityActionUpgrade, AmountIQD: 1, ActionAt: time.Now()}, nil
}
func (m *mockIPCapacityRepo) UpdateAction(*models.IPCapacityAction) error { return nil }
func (m *mockIPCapacityRepo) DeleteAction(uint) error                     { return nil }
func (m *mockIPCapacityRepo) ListHistoryDays() ([]string, error)          { return nil, nil }
func (m *mockIPCapacityRepo) GetDayHistory(day time.Time) (*repository.IPCapacityDayHistory, error) {
	m.historyDay = day
	return &repository.IPCapacityDayHistory{Summaries: []repository.IPCapacityNodeDaySummary{}, Actions: []repository.IPCapacityActionWithNode{}}, nil
}

func setupIPCapacityRouter(repo repository.IPCapacityRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewIPCapacityHandler(repo)
	r.POST("/actions", h.CreateAction)
	r.GET("/history/day", h.GetDayHistory)
	return r
}

func postJSON(t *testing.T, r http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestIPCapacityHandlerRejectsInvalidActionType(t *testing.T) {
	r := setupIPCapacityRouter(&mockIPCapacityRepo{})
	w := postJSON(t, r, "/actions", map[string]any{
		"node_id":    1,
		"type":       "raise",
		"amount_iqd": 100,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestIPCapacityHandlerRejectsInvalidAmount(t *testing.T) {
	r := setupIPCapacityRouter(&mockIPCapacityRepo{})
	w := postJSON(t, r, "/actions", map[string]any{
		"node_id":    1,
		"type":       "upgrade",
		"amount_iqd": 0,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestIPCapacityHandlerMissingNodeReturnsNotFound(t *testing.T) {
	r := setupIPCapacityRouter(&mockIPCapacityRepo{createActionErr: gorm.ErrRecordNotFound})
	w := postJSON(t, r, "/actions", map[string]any{
		"node_id":    99,
		"type":       "upgrade",
		"amount_iqd": 100,
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
	}
}

func TestIPCapacityHandlerHistoryDateFiltering(t *testing.T) {
	repo := &mockIPCapacityRepo{}
	r := setupIPCapacityRouter(repo)
	req := httptest.NewRequest(http.MethodGet, "/history/day?date=2026-04-25", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if repo.historyDay.Format("2006-01-02") != "2026-04-25" {
		t.Fatalf("history day = %s, want 2026-04-25", repo.historyDay.Format("2006-01-02"))
	}
}

func TestStatusForCapacityError(t *testing.T) {
	if got := statusForCapacityError(errors.New("amount_iqd must be greater than 0")); got != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got)
	}
}

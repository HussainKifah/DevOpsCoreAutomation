package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/gin-gonic/gin"
)

type mockAccessOltRepo struct {
	olts        []models.AccessOlt
	credentials []models.AccessOltCredential
	nextOltID   uint
	nextCredID  uint
}

func (m *mockAccessOltRepo) ListOlts() ([]models.AccessOlt, error) {
	return m.olts, nil
}

func (m *mockAccessOltRepo) CreateOlt(olt *models.AccessOlt) error {
	m.nextOltID++
	olt.ID = m.nextOltID
	m.olts = append(m.olts, *olt)
	return nil
}

func (m *mockAccessOltRepo) DeleteOlt(id uint) error {
	out := m.olts[:0]
	for _, item := range m.olts {
		if item.ID != id {
			out = append(out, item)
		}
	}
	m.olts = out
	return nil
}

func (m *mockAccessOltRepo) ListCredentials(vendorFamily string) ([]models.AccessOltCredential, error) {
	if vendorFamily == "" {
		return m.credentials, nil
	}
	out := make([]models.AccessOltCredential, 0)
	for _, item := range m.credentials {
		if item.VendorFamily == vendorFamily {
			out = append(out, item)
		}
	}
	return out, nil
}

func (m *mockAccessOltRepo) CreateCredential(c *models.AccessOltCredential) error {
	m.nextCredID++
	c.ID = m.nextCredID
	m.credentials = append(m.credentials, *c)
	return nil
}

func (m *mockAccessOltRepo) DeleteCredential(id uint) error {
	out := m.credentials[:0]
	for _, item := range m.credentials {
		if item.ID != id {
			out = append(out, item)
		}
	}
	m.credentials = out
	return nil
}

func setupAccessOltRouter(repo *mockAccessOltRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAccessOltHandler(repo, []byte("test-secret"))
	r := gin.New()
	r.GET("/olts", h.ListOlts)
	r.POST("/olts", h.CreateOlt)
	r.DELETE("/olts/:id", h.DeleteOlt)
	r.GET("/credentials", h.ListCredentials)
	r.POST("/credentials", h.CreateCredential)
	r.DELETE("/credentials/:id", h.DeleteCredential)
	return r
}

func TestAccessOltHandlerCreateListDeleteOlt(t *testing.T) {
	repo := &mockAccessOltRepo{}
	r := setupAccessOltRouter(repo)
	body := []byte(`{"ip":"10.1.1.1","name":"OLT-A","site":"Baghdad","olt_type":"nokia","latitude":33.3,"longitude":44.4}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/olts", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d body=%s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/olts", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d body=%s", w.Code, w.Body.String())
	}
	var listed struct {
		Olts []accessOltDTO `json:"olts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Olts) != 1 || listed.Olts[0].OltType != "nokia" {
		t.Fatalf("listed olts = %+v", listed.Olts)
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/olts/1", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d body=%s", w.Code, w.Body.String())
	}
	if len(repo.olts) != 0 {
		t.Fatalf("expected deleted olt, got %d", len(repo.olts))
	}
}

func TestAccessOltHandlerRejectsInvalidOlt(t *testing.T) {
	repo := &mockAccessOltRepo{}
	r := setupAccessOltRouter(repo)
	cases := [][]byte{
		[]byte(`{"ip":"","name":"OLT-A","site":"Baghdad","olt_type":"nokia"}`),
		[]byte(`{"ip":"10.1.1.1","name":"OLT-A","site":"Baghdad","olt_type":"cisco"}`),
	}
	for _, body := range cases {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/olts", bytes.NewReader(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d want 400 body=%s", w.Code, w.Body.String())
		}
	}
}

func TestAccessOltHandlerCredentialsDoNotReturnPassword(t *testing.T) {
	repo := &mockAccessOltRepo{}
	r := setupAccessOltRouter(repo)
	body := []byte(`{"vendor_family":"huawei","username":"admin","password":"secret-pass"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/credentials", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("create credential status = %d body=%s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("secret-pass")) {
		t.Fatalf("create response leaked password: %s", w.Body.String())
	}

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/credentials", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list credential status = %d body=%s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("secret-pass")) {
		t.Fatalf("list response leaked password: %s", w.Body.String())
	}
	var listed struct {
		Credentials []accessOltCredentialDTO `json:"credentials"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Credentials) != 1 || listed.Credentials[0].VendorFamily != "huawei" || listed.Credentials[0].Username != "admin" {
		t.Fatalf("listed credentials = %+v", listed.Credentials)
	}
}

func TestAccessOltHandlerRejectsInvalidCredentialVendor(t *testing.T) {
	repo := &mockAccessOltRepo{}
	r := setupAccessOltRouter(repo)
	body := []byte(`{"vendor_family":"cisco","username":"admin","password":"secret"}`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/credentials", bytes.NewReader(body)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400 body=%s", w.Code, w.Body.String())
	}
}

package betterstack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientListUnresolvedIncidentsPaginationAndQuery(t *testing.T) {
	var seen []string
	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		seen = append(seen, r.URL.String())
		if r.URL.Query().Get("from") != "2026-05-03" {
			t.Fatalf("from = %q", r.URL.Query().Get("from"))
		}
		if r.URL.Query().Get("to") != "2026-05-04" {
			t.Fatalf("to = %q", r.URL.Query().Get("to"))
		}
		if r.URL.Query().Get("resolved") != "false" {
			t.Fatalf("resolved = %q", r.URL.Query().Get("resolved"))
		}

		if r.URL.Query().Get("page") == "2" {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": []map[string]interface{}{
					{
						"id": "2",
						"attributes": map[string]interface{}{
							"name":       "Second",
							"started_at": "2026-05-04T02:00:00Z",
							"status":     "Started",
						},
					},
				},
				"pagination": map[string]interface{}{"next": nil},
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "1",
					"attributes": map[string]interface{}{
						"name":       "First",
						"cause":      "Status 500",
						"started_at": "2026-05-04T01:00:00Z",
						"status":     "Started",
					},
				},
			},
			"pagination": map[string]interface{}{"next": serverURL + r.URL.Path + "?" + r.URL.RawQuery + "&page=2"},
		})
	}))
	defer srv.Close()
	serverURL = srv.URL

	c := NewClient("test-token")
	c.baseURL = srv.URL
	got, err := c.ListUnresolvedIncidents(context.Background(),
		time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].ID != "1" || got[0].Cause != "Status 500" || got[1].ID != "2" {
		t.Fatalf("unexpected incidents: %#v", got)
	}
	if len(seen) != 2 {
		t.Fatalf("requests = %d", len(seen))
	}
}

func TestClientGetIncidentResolved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/incidents/42" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id": "42",
				"attributes": map[string]interface{}{
					"name":        "Resolved incident",
					"started_at":  "2026-05-04T01:00:00Z",
					"resolved_at": "2026-05-04T03:00:00Z",
					"resolved_by": "ops@example.com",
					"status":      "Resolved",
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("test-token")
	c.baseURL = srv.URL
	got, err := c.GetIncident(context.Background(), "42")
	if err != nil {
		t.Fatal(err)
	}
	if got.ResolvedAt == nil || got.ResolvedBy != "ops@example.com" {
		t.Fatalf("unexpected resolved incident: %#v", got)
	}
}

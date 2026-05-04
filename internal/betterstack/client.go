package betterstack

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://uptime.betterstack.com/api/v3"

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

type Incident struct {
	ID         string
	Name       string
	URL        string
	Cause      string
	Status     string
	TeamName   string
	OriginURL  string
	ResolvedBy string

	StartedAt      time.Time
	AcknowledgedAt *time.Time
	ResolvedAt     *time.Time
}

type incidentResource struct {
	ID         string             `json:"id"`
	Type       string             `json:"type"`
	Attributes incidentAttributes `json:"attributes"`
}

type incidentAttributes struct {
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Cause          string     `json:"cause"`
	StartedAt      time.Time  `json:"started_at"`
	AcknowledgedAt *time.Time `json:"acknowledged_at"`
	ResolvedAt     *time.Time `json:"resolved_at"`
	ResolvedBy     string     `json:"resolved_by"`
	Status         string     `json:"status"`
	TeamName       string     `json:"team_name"`
	OriginURL      string     `json:"origin_url"`
}

type listIncidentsResponse struct {
	Data       []incidentResource `json:"data"`
	Pagination struct {
		Next string `json:"next"`
	} `json:"pagination"`
}

type getIncidentResponse struct {
	Data incidentResource `json:"data"`
}

func NewClient(token string) *Client {
	return &Client{
		token:      strings.TrimSpace(token),
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) ListUnresolvedIncidents(ctx context.Context, from, to time.Time) ([]Incident, error) {
	if c == nil {
		return nil, fmt.Errorf("nil client")
	}
	u, err := url.Parse(strings.TrimRight(c.baseURL, "/") + "/incidents")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("from", from.Format("2006-01-02"))
	q.Set("to", to.Format("2006-01-02"))
	q.Set("resolved", "false")
	q.Set("per_page", "50")
	u.RawQuery = q.Encode()

	var out []Incident
	next := u.String()
	for next != "" {
		var page listIncidentsResponse
		if err := c.get(ctx, next, &page); err != nil {
			return nil, err
		}
		for i := range page.Data {
			out = append(out, incidentFromResource(page.Data[i]))
		}
		next = strings.TrimSpace(page.Pagination.Next)
	}
	return out, nil
}

func (c *Client) GetIncident(ctx context.Context, id string) (Incident, error) {
	if c == nil {
		return Incident{}, fmt.Errorf("nil client")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Incident{}, fmt.Errorf("empty incident id")
	}
	u := strings.TrimRight(c.baseURL, "/") + "/incidents/" + url.PathEscape(id)
	var out getIncidentResponse
	if err := c.get(ctx, u, &out); err != nil {
		return Incident{}, err
	}
	return incidentFromResource(out.Data), nil
}

func (c *Client) get(ctx context.Context, endpoint string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}

func incidentFromResource(r incidentResource) Incident {
	return Incident{
		ID:             strings.TrimSpace(r.ID),
		Name:           strings.TrimSpace(r.Attributes.Name),
		URL:            strings.TrimSpace(r.Attributes.URL),
		Cause:          strings.TrimSpace(r.Attributes.Cause),
		Status:         strings.TrimSpace(r.Attributes.Status),
		TeamName:       strings.TrimSpace(r.Attributes.TeamName),
		OriginURL:      strings.TrimSpace(r.Attributes.OriginURL),
		ResolvedBy:     strings.TrimSpace(r.Attributes.ResolvedBy),
		StartedAt:      r.Attributes.StartedAt.UTC(),
		AcknowledgedAt: utcPtr(r.Attributes.AcknowledgedAt),
		ResolvedAt:     utcPtr(r.Attributes.ResolvedAt),
	}
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	v := t.UTC()
	return &v
}

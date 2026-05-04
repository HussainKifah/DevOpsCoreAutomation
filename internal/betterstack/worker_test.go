package betterstack

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

func TestWorkerPostsNewIncidentOnceAndReminds(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	store := newFakeStore()
	api := &fakeAPI{unresolved: []Incident{{
		ID:        "123",
		Name:      "OLT check",
		Cause:     "Status 500",
		Status:    "Started",
		StartedAt: now.Add(-time.Hour),
	}}}
	sl := &fakeSlack{}
	w := testWorker(store, api, sl)

	w.tick(context.Background(), now)
	if len(sl.posts) != 2 {
		t.Fatalf("posts after first tick = %d", len(sl.posts))
	}
	if len(store.rows) != 1 {
		t.Fatalf("rows = %d", len(store.rows))
	}

	w.tick(context.Background(), now.Add(time.Minute))
	if len(sl.posts) != 2 {
		t.Fatalf("duplicate post count = %d", len(sl.posts))
	}

	row := store.rows[1]
	row.NextReminderAt = now.Add(-time.Minute)
	store.rows[1] = row
	w.tick(context.Background(), now.Add(2*time.Minute))
	if len(sl.posts) != 3 {
		t.Fatalf("posts after reminder = %d", len(sl.posts))
	}
}

func TestWorkerPostsResolvedThreadOnce(t *testing.T) {
	now := time.Date(2026, 5, 4, 12, 0, 0, 0, time.UTC)
	resolvedAt := now.Add(10 * time.Minute)
	store := newFakeStore()
	store.rows[1] = models.BetterStackSlackIncident{
		ID:                    1,
		BetterStackIncidentID: "123",
		ChannelID:             "C123",
		MessageTS:             "1000.0001",
		Name:                  "OLT check",
		StartedAtUTC:          now.Add(-time.Hour),
		NextReminderAt:        now.Add(6 * time.Hour),
	}
	api := &fakeAPI{byID: map[string]Incident{
		"123": {
			ID:         "123",
			Name:       "OLT check",
			Status:     "Resolved",
			StartedAt:  now.Add(-time.Hour),
			ResolvedAt: &resolvedAt,
			ResolvedBy: "ops@example.com",
		},
	}}
	sl := &fakeSlack{}
	w := testWorker(store, api, sl)

	w.tick(context.Background(), now)
	if len(sl.posts) != 1 {
		t.Fatalf("posts = %d", len(sl.posts))
	}
	if store.rows[1].ResolvedAtUTC == nil {
		t.Fatal("incident was not marked resolved")
	}

	w.tick(context.Background(), now.Add(time.Minute))
	if len(sl.posts) != 1 {
		t.Fatalf("resolved posted twice, posts = %d", len(sl.posts))
	}
}

func testWorker(store *fakeStore, api *fakeAPI, sl *fakeSlack) *Worker {
	return NewWorker(&config.Config{
		BetterStackEnabled:          true,
		BetterStackAPIToken:         "token",
		SlackBotToken:               "xoxb-token",
		BetterStackSlackChannelID:   "C123",
		BetterStackReminderInterval: 6 * time.Hour,
		BetterStackPollInterval:     time.Minute,
		BetterStackLookbackDays:     1,
	}, store, api, sl)
}

type fakeAPI struct {
	unresolved []Incident
	byID       map[string]Incident
}

func (f *fakeAPI) ListUnresolvedIncidents(ctx context.Context, from, to time.Time) ([]Incident, error) {
	return f.unresolved, nil
}

func (f *fakeAPI) GetIncident(ctx context.Context, id string) (Incident, error) {
	if f.byID == nil {
		return Incident{ID: id, StartedAt: time.Now().UTC()}, nil
	}
	inc, ok := f.byID[id]
	if !ok {
		return Incident{ID: id, StartedAt: time.Now().UTC()}, nil
	}
	return inc, nil
}

type fakeSlack struct {
	posts []fakePost
}

type fakePost struct {
	channel string
}

func (f *fakeSlack) PostMessage(channelID string, options ...slack.MsgOption) (string, string, error) {
	f.posts = append(f.posts, fakePost{channel: channelID})
	ts := time.Unix(0, int64(len(f.posts))).Format("1504.000000")
	return channelID, ts, nil
}

type fakeStore struct {
	rows map[uint]models.BetterStackSlackIncident
	next uint
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: make(map[uint]models.BetterStackSlackIncident), next: 1}
}

func (f *fakeStore) CreateSlackIncidentIfNew(inc *models.BetterStackSlackIncident) (bool, error) {
	for _, row := range f.rows {
		if row.ChannelID == inc.ChannelID && row.BetterStackIncidentID == inc.BetterStackIncidentID {
			return false, nil
		}
	}
	inc.ID = f.next
	f.next++
	f.rows[inc.ID] = *inc
	return true, nil
}

func (f *fakeStore) FindByBetterStackIncident(channelID, incidentID string) (*models.BetterStackSlackIncident, error) {
	for _, row := range f.rows {
		if row.ChannelID == channelID && row.BetterStackIncidentID == incidentID {
			cp := row
			return &cp, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (f *fakeStore) ListOpenSlackIncidents() ([]models.BetterStackSlackIncident, error) {
	var out []models.BetterStackSlackIncident
	for _, row := range f.rows {
		if row.ResolvedAtUTC == nil {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeStore) ListOpenSlackIncidentsDueReminder(until time.Time) ([]models.BetterStackSlackIncident, error) {
	var out []models.BetterStackSlackIncident
	for _, row := range f.rows {
		if row.ResolvedAtUTC == nil && !row.NextReminderAt.After(until) {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeStore) UpdateSlackIncidentSnapshot(id uint, fields map[string]interface{}) error {
	row, ok := f.rows[id]
	if !ok {
		return errors.New("missing row")
	}
	if v, ok := fields["name"].(string); ok {
		row.Name = v
	}
	if v, ok := fields["last_seen_at_utc"].(time.Time); ok {
		row.LastSeenAtUTC = v
	}
	f.rows[id] = row
	return nil
}

func (f *fakeStore) BumpSlackIncidentReminder(id uint, next time.Time) error {
	row, ok := f.rows[id]
	if !ok {
		return errors.New("missing row")
	}
	row.NextReminderAt = next
	f.rows[id] = row
	return nil
}

func (f *fakeStore) MarkSlackIncidentResolved(id uint, resolvedBy string, at time.Time) error {
	row, ok := f.rows[id]
	if !ok {
		return errors.New("missing row")
	}
	row.ResolvedAtUTC = &at
	row.ResolvedBy = resolvedBy
	f.rows[id] = row
	return nil
}

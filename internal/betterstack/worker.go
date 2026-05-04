package betterstack

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

type IncidentAPI interface {
	ListUnresolvedIncidents(ctx context.Context, from, to time.Time) ([]Incident, error)
	GetIncident(ctx context.Context, id string) (Incident, error)
}

type Store interface {
	CreateSlackIncidentIfNew(inc *models.BetterStackSlackIncident) (bool, error)
	FindByBetterStackIncident(channelID, incidentID string) (*models.BetterStackSlackIncident, error)
	ListOpenSlackIncidents() ([]models.BetterStackSlackIncident, error)
	ListOpenSlackIncidentsDueReminder(until time.Time) ([]models.BetterStackSlackIncident, error)
	UpdateSlackIncidentSnapshot(id uint, fields map[string]interface{}) error
	BumpSlackIncidentReminder(id uint, next time.Time) error
	MarkSlackIncidentResolved(id uint, resolvedBy string, at time.Time) error
}

type SlackPoster interface {
	PostMessage(channelID string, options ...slack.MsgOption) (string, string, error)
}

type Worker struct {
	cfg   *config.Config
	store Store
	api   IncidentAPI
	slack SlackPoster

	stop     chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

func NewWorker(cfg *config.Config, store Store, api IncidentAPI, slack SlackPoster) *Worker {
	return &Worker{
		cfg:   cfg,
		store: store,
		api:   api,
		slack: slack,
		stop:  make(chan struct{}),
	}
}

func (w *Worker) Start() {
	if w == nil || !w.cfg.BetterStackConfigured() || w.store == nil || w.api == nil || w.slack == nil {
		return
	}
	w.wg.Add(1)
	go w.loop()
	log.Printf("[betterstack] worker started channel=%s poll=%s reminder=%s lookback_days=%d",
		w.cfg.BetterStackSlackChannelID, pollInterval(w.cfg), reminderInterval(w.cfg), w.lookbackDays())
}

func (w *Worker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}

func (w *Worker) loop() {
	defer w.wg.Done()
	t := time.NewTicker(pollInterval(w.cfg))
	defer t.Stop()
	w.tick(context.Background(), time.Now().UTC())
	for {
		select {
		case <-w.stop:
			return
		case now := <-t.C:
			w.tick(context.Background(), now.UTC())
		}
	}
}

func (w *Worker) tick(ctx context.Context, now time.Time) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	from := now.AddDate(0, 0, -w.lookbackDays())
	unresolved, err := w.api.ListUnresolvedIncidents(ctx, from, now)
	if err != nil {
		log.Printf("[betterstack] list incidents: %v", err)
	} else {
		w.handleUnresolved(unresolved, now)
	}
	w.checkResolved(ctx)
	w.sendDueReminders(now)
}

func (w *Worker) handleUnresolved(list []Incident, now time.Time) {
	for i := range list {
		inc := normalizeIncident(list[i], now)
		if inc.ID == "" || inc.ResolvedAt != nil {
			continue
		}
		existing, err := w.store.FindByBetterStackIncident(w.cfg.BetterStackSlackChannelID, inc.ID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[betterstack] find incident=%s: %v", inc.ID, err)
			continue
		}
		if existing != nil {
			if err := w.store.UpdateSlackIncidentSnapshot(existing.ID, snapshotFields(inc, now)); err != nil {
				log.Printf("[betterstack] update snapshot incident=%s: %v", inc.ID, err)
			}
			continue
		}
		w.postNewIncident(inc, now)
	}
}

func (w *Worker) postNewIncident(inc Incident, now time.Time) {
	_, ts, err := w.slack.PostMessage(
		w.cfg.BetterStackSlackChannelID,
		slack.MsgOptionAttachments(BuildIncidentAttachment(inc)),
	)
	if err != nil {
		log.Printf("[betterstack] slack post incident=%s: %v", inc.ID, err)
		return
	}
	_, _, err = w.slack.PostMessage(
		w.cfg.BetterStackSlackChannelID,
		slack.MsgOptionTS(ts),
		slack.MsgOptionText(FirstThreadReminder(w.cfg), false),
	)
	if err != nil {
		log.Printf("[betterstack] first thread reminder incident=%s: %v", inc.ID, err)
	}

	row := modelFromIncident(inc, w.cfg.BetterStackSlackChannelID, ts, now, now.Add(reminderInterval(w.cfg)))
	inserted, err := w.store.CreateSlackIncidentIfNew(row)
	if err != nil {
		log.Printf("[betterstack] save incident=%s: %v", inc.ID, err)
		return
	}
	if !inserted {
		log.Printf("[betterstack] incident already tracked after post incident=%s ts=%s", inc.ID, ts)
		return
	}
	log.Printf("[betterstack] posted incident=%s ts=%s", inc.ID, ts)
}

func (w *Worker) checkResolved(ctx context.Context) {
	open, err := w.store.ListOpenSlackIncidents()
	if err != nil {
		log.Printf("[betterstack] list open incidents: %v", err)
		return
	}
	for i := range open {
		row := open[i]
		inc, err := w.api.GetIncident(ctx, row.BetterStackIncidentID)
		if err != nil {
			log.Printf("[betterstack] get incident=%s: %v", row.BetterStackIncidentID, err)
			continue
		}
		if inc.ResolvedAt == nil {
			continue
		}
		resolvedAt := inc.ResolvedAt.UTC()
		resolvedBy := strings.TrimSpace(inc.ResolvedBy)
		if err := w.store.MarkSlackIncidentResolved(row.ID, resolvedBy, resolvedAt); err != nil {
			log.Printf("[betterstack] mark resolved incident=%s: %v", row.BetterStackIncidentID, err)
			continue
		}
		_, _, err = w.slack.PostMessage(
			row.ChannelID,
			slack.MsgOptionTS(row.MessageTS),
			slack.MsgOptionAttachments(BuildResolvedAttachment(row, resolvedAt, resolvedBy)),
			slack.MsgOptionText(":white_check_mark: Better Stack incident resolved. Reminders stopped.", false),
		)
		if err != nil {
			log.Printf("[betterstack] resolved thread post incident=%s: %v", row.BetterStackIncidentID, err)
		}
	}
}

func (w *Worker) sendDueReminders(now time.Time) {
	list, err := w.store.ListOpenSlackIncidentsDueReminder(now)
	if err != nil {
		log.Printf("[betterstack] list reminders: %v", err)
		return
	}
	next := now.Add(reminderInterval(w.cfg))
	for i := range list {
		inc := &list[i]
		_, _, err := w.slack.PostMessage(
			inc.ChannelID,
			slack.MsgOptionTS(inc.MessageTS),
			slack.MsgOptionText(ReminderText(w.cfg), false),
		)
		if err != nil {
			log.Printf("[betterstack] reminder post incident=%s: %v", inc.BetterStackIncidentID, err)
			continue
		}
		if err := w.store.BumpSlackIncidentReminder(inc.ID, next); err != nil {
			log.Printf("[betterstack] bump reminder incident=%s: %v", inc.BetterStackIncidentID, err)
		}
	}
}

func (w *Worker) lookbackDays() int {
	if w == nil || w.cfg == nil || w.cfg.BetterStackLookbackDays < 0 {
		return 1
	}
	return w.cfg.BetterStackLookbackDays
}

func pollInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.BetterStackPollInterval < time.Minute {
		return time.Minute
	}
	return cfg.BetterStackPollInterval
}

func normalizeIncident(inc Incident, now time.Time) Incident {
	if inc.StartedAt.IsZero() {
		inc.StartedAt = now.UTC()
	} else {
		inc.StartedAt = inc.StartedAt.UTC()
	}
	inc.AcknowledgedAt = utcPtr(inc.AcknowledgedAt)
	inc.ResolvedAt = utcPtr(inc.ResolvedAt)
	return inc
}

func snapshotFields(inc Incident, now time.Time) map[string]interface{} {
	return map[string]interface{}{
		"name":                inc.Name,
		"cause":               inc.Cause,
		"url":                 inc.URL,
		"origin_url":          inc.OriginURL,
		"status":              inc.Status,
		"team_name":           inc.TeamName,
		"started_at_utc":      inc.StartedAt,
		"acknowledged_at_utc": inc.AcknowledgedAt,
		"last_seen_at_utc":    now.UTC(),
	}
}

func modelFromIncident(inc Incident, channelID, messageTS string, now, nextReminder time.Time) *models.BetterStackSlackIncident {
	return &models.BetterStackSlackIncident{
		BetterStackIncidentID: inc.ID,
		ChannelID:             channelID,
		MessageTS:             messageTS,
		Name:                  inc.Name,
		Cause:                 inc.Cause,
		URL:                   inc.URL,
		OriginURL:             inc.OriginURL,
		Status:                inc.Status,
		TeamName:              inc.TeamName,
		StartedAtUTC:          inc.StartedAt,
		AcknowledgedAtUTC:     inc.AcknowledgedAt,
		ResolvedAtUTC:         inc.ResolvedAt,
		ResolvedBy:            inc.ResolvedBy,
		LastSeenAtUTC:         now.UTC(),
		NextReminderAt:        nextReminder.UTC(),
	}
}

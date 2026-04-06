package syslog

import (
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/slack-go/slack"
	"gorm.io/gorm"
)

// SlackSyslogBatcher coalesces new alerts per device for a short window, then posts one Slack message.
type SlackSyslogBatcher struct {
	cfg  *config.Config
	repo *repository.EsSyslogRepository
	api  *slack.Client

	mu       sync.Mutex
	pending  map[string]*slackBatch
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

type slackBatch struct {
	alerts []models.EsSyslogAlert
	timer  *time.Timer
}

func NewSlackSyslogBatcher(cfg *config.Config, repo *repository.EsSyslogRepository, api *slack.Client) *SlackSyslogBatcher {
	return &SlackSyslogBatcher{
		cfg:     cfg,
		repo:    repo,
		api:     api,
		pending: make(map[string]*slackBatch),
		stopCh:  make(chan struct{}),
	}
}

func (b *SlackSyslogBatcher) batchWindow() time.Duration {
	w := b.cfg.SlackSyslogBatchWindow
	if w < 5*time.Second {
		return 5 * time.Second
	}
	if w > 5*time.Minute {
		return 5 * time.Minute
	}
	return w
}

// Start is optional no-op if you only use Enqueue from poller; timers still run.
func (b *SlackSyslogBatcher) Start() {}

// SlackAlarmKey batches Slack delivery per device + dedup fingerprint (same logical alarm).
func SlackAlarmKey(a models.EsSyslogAlert) string {
	return SlackDeviceKey(a.Host, a.DeviceName) + "\x1e" + strings.TrimSpace(a.DedupFingerprint)
}

// Enqueue schedules a flush for this device+fingerprint batch after the batch window.
func (b *SlackSyslogBatcher) Enqueue(a models.EsSyslogAlert) {
	if b == nil || b.api == nil || b.repo == nil || !b.cfg.SlackSyslogConfigured() {
		return
	}
	key := SlackAlarmKey(a)

	b.mu.Lock()
	defer b.mu.Unlock()
	select {
	case <-b.stopCh:
		return
	default:
	}

	bat, ok := b.pending[key]
	if !ok {
		bat = &slackBatch{}
		b.pending[key] = bat
	}
	bat.alerts = append(bat.alerts, a)
	if bat.timer != nil {
		bat.timer.Stop()
	}
	win := b.batchWindow()
	bat.timer = time.AfterFunc(win, func() { b.flush(key) })
}

func (b *SlackSyslogBatcher) flush(batchKey string) {
	b.mu.Lock()
	bat, ok := b.pending[batchKey]
	if !ok {
		b.mu.Unlock()
		return
	}
	delete(b.pending, batchKey)
	if bat.timer != nil {
		bat.timer.Stop()
	}
	full := bat.alerts
	b.mu.Unlock()

	if len(full) == 0 {
		return
	}

	fp := strings.TrimSpace(full[0].DedupFingerprint)
	deviceKey := SlackDeviceKey(full[0].Host, full[0].DeviceName)

	const maxSlackAlerts = 18
	display := full
	trunc := 0
	if len(display) > maxSlackAlerts {
		trunc = len(display) - maxSlackAlerts
		display = display[:maxSlackAlerts]
	}

	var open *models.EsSyslogSlackIncident
	if fp != "" {
		inc, err := b.repo.FindOpenSlackIncidentByChannelFingerprint(b.cfg.SlackChannelID, fp)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[slack-syslog] find open incident: %v", err)
			return
		}
		open = inc
	}

	if open != nil {
		att := BuildSyslogSlackThreadAppendAttachment(display, b.cfg.SlackSyslogDisplayOffset, trunc)
		_, _, err := b.api.PostMessage(
			b.cfg.SlackChannelID,
			slack.MsgOptionTS(open.MessageTS),
			slack.MsgOptionAttachments(att),
		)
		if err != nil {
			log.Printf("[slack-syslog] thread follow-up post: %v", err)
			return
		}
		ids := make([]uint, 0, len(full))
		for i := range full {
			if full[i].ID != 0 {
				ids = append(ids, full[i].ID)
			}
		}
		if err := b.repo.LinkAlertsToSlackIncident(ids, open.ID); err != nil {
			log.Printf("[slack-syslog] link alerts: %v", err)
		}
		log.Printf("[slack-syslog] thread follow-up incident=%d device=%q fp=%s alerts=%d", open.ID, deviceKey, fp, len(full))
		return
	}

	att := BuildSyslogSlackAttachment(display, false, nil, "", trunc, b.cfg.SlackSyslogDisplayOffset)
	_, ts, err := b.api.PostMessage(
		b.cfg.SlackChannelID,
		slack.MsgOptionAttachments(att),
	)
	if err != nil {
		log.Printf("[slack-syslog] post failed: %v", err)
		return
	}

	_, _, err = b.api.PostMessage(
		b.cfg.SlackChannelID,
		slack.MsgOptionTS(ts),
		slack.MsgOptionText(SlackSyslogFirstThreadReminder(b.cfg), false),
	)
	if err != nil {
		log.Printf("[slack-syslog] thread instruction post: %v", err)
	}

	rem := time.Now().UTC().Add(b.cfg.SlackReminderInterval)
	inc := &models.EsSyslogSlackIncident{
		DeviceKey:        deviceKey,
		DedupFingerprint: fp,
		ChannelID:        b.cfg.SlackChannelID,
		MessageTS:        ts,
		NextReminderAt:   rem,
	}
	if err := b.repo.CreateSlackIncident(inc); err != nil {
		log.Printf("[slack-syslog] save incident: %v", err)
		return
	}
	ids := make([]uint, 0, len(full))
	for i := range full {
		if full[i].ID != 0 {
			ids = append(ids, full[i].ID)
		}
	}
	if err := b.repo.LinkAlertsToSlackIncident(ids, inc.ID); err != nil {
		log.Printf("[slack-syslog] link alerts: %v", err)
	}
	log.Printf("[slack-syslog] posted incident id=%d device=%q alerts=%d ts=%s", inc.ID, deviceKey, len(full), ts)
}

// Stop flushes pending batches synchronously (best-effort) then prevents new enqueue.
func (b *SlackSyslogBatcher) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() { close(b.stopCh) })

	b.mu.Lock()
	keys := make([]string, 0, len(b.pending))
	for k := range b.pending {
		keys = append(keys, k)
	}
	b.mu.Unlock()
	for _, k := range keys {
		b.flush(k)
	}
}

// SlackDeviceKey groups alerts for one device (same host + device name).
func SlackDeviceKey(host, deviceName string) string {
	return strings.TrimSpace(host) + "|" + strings.TrimSpace(deviceName)
}


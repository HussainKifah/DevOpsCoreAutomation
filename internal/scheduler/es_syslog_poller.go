package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/syslog"
)

// EsSyslogPoller polls Elasticsearch per enabled filter every EsSyslogPollInterval and stores deduplicated alerts.
type EsSyslogPoller struct {
	cfg  *config.Config
	repo *repository.EsSyslogRepository
	stop    chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup
	mu      sync.Mutex
	lastRet time.Time
}

func NewEsSyslogPoller(cfg *config.Config, repo *repository.EsSyslogRepository) *EsSyslogPoller {
	return &EsSyslogPoller{cfg: cfg, repo: repo, stop: make(chan struct{})}
}

func (p *EsSyslogPoller) Start() {
	if p.cfg == nil || p.repo == nil {
		return
	}
	if p.cfg.EsSyslogPollInterval < 30*time.Second {
		p.cfg.EsSyslogPollInterval = time.Minute
	}
	p.wg.Add(1)
	go p.loop()
	log.Printf("[es-syslog] poller started (interval=%s, retention=%dd)", p.cfg.EsSyslogPollInterval, p.cfg.EsSyslogRetentionDays)
}

func (p *EsSyslogPoller) Stop() {
	p.stopOnce.Do(func() {
		close(p.stop)
		p.wg.Wait()
	})
}

func (p *EsSyslogPoller) loop() {
	defer p.wg.Done()
	t := time.NewTicker(p.cfg.EsSyslogPollInterval)
	defer t.Stop()
	p.tick(context.Background())
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.tick(context.Background())
		}
	}
}

func (p *EsSyslogPoller) tick(ctx context.Context) {
	client, err := syslog.NewClient(p.cfg)
	if err != nil {
		log.Printf("[es-syslog] client error: %v", err)
		return
	}
	if client == nil {
		return
	}

	p.maybeRetention()

	filters, err := p.repo.ListFilters()
	if err != nil {
		log.Printf("[es-syslog] list filters: %v", err)
		return
	}
	if len(filters) == 0 {
		return
	}

	now := time.Now().UTC()
	// ~2m window with overlap so minute-boundary logs are not missed; duplicates suppressed by DB unique key.
	from := now.Add(-2 * time.Minute)
	to := now.Add(30 * time.Second)

	for _, f := range filters {
		if !f.Enabled || f.QueryText == "" {
			continue
		}
		hits, err := client.SearchLastWindow(ctx, f.QueryText, from, to)
		if err != nil {
			log.Printf("[es-syslog] search filter id=%d: %v", f.ID, err)
			continue
		}
		label := f.Label
		if label == "" {
			label = f.QueryText
			if len(label) > 80 {
				label = label[:80] + "…"
			}
		}
		dedupWin := p.cfg.EsSyslogDedupWindow
		if dedupWin < time.Minute {
			dedupWin = time.Hour
		}

		for _, h := range hits {
			ts, err := time.Parse(time.RFC3339Nano, h.Source.Timestamp)
			if err != nil {
				ts, err = time.Parse(time.RFC3339, h.Source.Timestamp)
				if err != nil {
					ts = now
				}
			}
			ts = ts.UTC()
			dev := syslog.DeviceNameFromMessage(h.Source.Message)
			fp := syslog.DedupFingerprint(h.Source.Host, dev, h.Source.Message)
			since := ts.Add(-dedupWin)
			dup, err := p.repo.AlertDedupExists(fp, since)
			if err != nil {
				log.Printf("[es-syslog] dedup check: %v", err)
				continue
			}
			if dup {
				continue
			}
			a := &models.EsSyslogAlert{
				EsIndex:          h.Index,
				EsDocID:          h.ID,
				TimestampUTC:     ts,
				Host:             h.Source.Host,
				DeviceName:       dev,
				Message:          h.Source.Message,
				FilterID:         f.ID,
				FilterLabel:      label,
				DedupFingerprint: fp,
			}
			if err := p.repo.InsertAlertIfNew(a); err != nil {
				log.Printf("[es-syslog] insert: %v", err)
			}
		}
	}
}

func (p *EsSyslogPoller) maybeRetention() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.lastRet.IsZero() && time.Since(p.lastRet) < 24*time.Hour {
		return
	}
	p.lastRet = time.Now()
	days := p.cfg.EsSyslogRetentionDays
	if days < 1 {
		days = 30
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	n, err := p.repo.DeleteAlertsOlderThan(cutoff)
	if err != nil {
		log.Printf("[es-syslog] retention delete: %v", err)
		return
	}
	if n > 0 {
		log.Printf("[es-syslog] retention: hard-deleted %d alert row(s) older than %v", n, cutoff)
	}
}

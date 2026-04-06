package syslog

import (
	"log"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/slack-go/slack"
)

// SlackReminderWorker posts thread reminders for open Slack syslog incidents.
type SlackReminderWorker struct {
	cfg  *config.Config
	repo *repository.EsSyslogRepository
	api  *slack.Client

	stop    chan struct{}
	stopOnce sync.Once
	wg      sync.WaitGroup
}

func NewSlackReminderWorker(cfg *config.Config, repo *repository.EsSyslogRepository, api *slack.Client) *SlackReminderWorker {
	return &SlackReminderWorker{
		cfg:  cfg,
		repo: repo,
		api:  api,
		stop: make(chan struct{}),
	}
}

func (w *SlackReminderWorker) Start() {
	if w == nil || !w.cfg.SlackSyslogConfigured() {
		return
	}
	w.wg.Add(1)
	go w.loop()
}

func (w *SlackReminderWorker) loop() {
	defer w.wg.Done()
	t := time.NewTicker(2 * time.Minute)
	defer t.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-t.C:
			w.tick()
		}
	}
}

func (w *SlackReminderWorker) tick() {
	now := time.Now().UTC()
	list, err := w.repo.ListOpenSlackIncidentsDueReminder(now)
	if err != nil {
		log.Printf("[slack-syslog] list reminders: %v", err)
		return
	}
	interval := w.cfg.SlackReminderInterval
	if interval < time.Hour {
		interval = 6 * time.Hour
	}
	next := now.Add(interval)
	remText := HumanReminderEvery(interval)
	mention := SlackTeamMention(w.cfg.SlackSyslogTeamMention)

	for i := range list {
		inc := &list[i]
		_, _, err := w.api.PostMessage(
			inc.ChannelID,
			slack.MsgOptionTS(inc.MessageTS),
			slack.MsgOptionText(
				":alarm_clock: "+mention+" — Reminder: this syslog alert is still open. Add :white_check_mark: on the main alert (or a thread message) when handled. "+
					"(Next reminder in ~"+remText+".)",
				false,
			),
		)
		if err != nil {
			log.Printf("[slack-syslog] reminder post incident=%d: %v", inc.ID, err)
			continue
		}
		if err := w.repo.BumpSlackIncidentReminder(inc.ID, next); err != nil {
			log.Printf("[slack-syslog] bump reminder incident=%d: %v", inc.ID, err)
		}
	}
}

func (w *SlackReminderWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
}

package scheduler

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/slack-go/slack"
)

type ActivityLogSlackWorker struct {
	cfg    *config.Config
	repo   repository.WorkflowRepository
	api    *slack.Client
	stop   chan struct{}
	done   chan struct{}
	mu     sync.Mutex
	posted map[string]struct{}
}

func NewActivityLogSlackWorker(cfg *config.Config, repo repository.WorkflowRepository, api *slack.Client) *ActivityLogSlackWorker {
	return &ActivityLogSlackWorker{
		cfg:    cfg,
		repo:   repo,
		api:    api,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		posted: make(map[string]struct{}),
	}
}

func (w *ActivityLogSlackWorker) Start() {
	if w == nil || w.cfg == nil || !w.cfg.SlackActivityLogConfigured() || w.repo == nil || w.api == nil {
		return
	}
	go w.loop()
	log.Printf("[slack-activity-log] enabled channel=%s daily_time=%s", w.cfg.SlackActivityLogChannelID, w.cfg.SlackActivityLogDailyTime)
}

func (w *ActivityLogSlackWorker) Stop() {
	if w == nil {
		return
	}
	select {
	case <-w.done:
		return
	default:
	}
	close(w.stop)
	<-w.done
}

func (w *ActivityLogSlackWorker) loop() {
	defer close(w.done)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	w.tick(time.Now())
	for {
		select {
		case <-ticker.C:
			w.tick(time.Now())
		case <-w.stop:
			return
		}
	}
}

func (w *ActivityLogSlackWorker) tick(now time.Time) {
	hour, minute := activityLogSlackClock(w.cfg.SlackActivityLogDailyTime)
	localNow := now.In(time.Local)
	if localNow.Hour() != hour || localNow.Minute() != minute {
		return
	}
	dayKey := localNow.Format("2006-01-02")
	w.mu.Lock()
	if _, ok := w.posted[dayKey]; ok {
		w.mu.Unlock()
		return
	}
	w.posted[dayKey] = struct{}{}
	w.mu.Unlock()
	if err := w.postDailySummary(localNow); err != nil {
		log.Printf("[slack-activity-log] daily summary: %v", err)
	}
}

func (w *ActivityLogSlackWorker) postDailySummary(now time.Time) error {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	end := start.Add(24 * time.Hour)
	logs, err := w.repo.ListLogsBetween(start, end)
	if err != nil {
		return err
	}
	att := buildActivityLogSlackAttachment(logs, start, w.cfg.SlackActivityLogDisplayOffset)
	_, _, err = w.api.PostMessage(
		w.cfg.SlackActivityLogChannelID,
		slack.MsgOptionAttachments(att),
		slack.MsgOptionText(att.Fallback, false),
	)
	return err
}

func activityLogSlackClock(value string) (int, int) {
	t, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 9, 0
	}
	return t.Hour(), t.Minute()
}

func buildActivityLogSlackAttachment(logs []models.WorkflowLog, day time.Time, displayOffset time.Duration) slack.Attachment {
	errors := make([]models.WorkflowLog, 0)
	backupSuccess := 0
	backupErrors := 0
	for i := range logs {
		entry := logs[i]
		if strings.EqualFold(entry.Level, "error") {
			errors = append(errors, entry)
			if strings.EqualFold(entry.JobType, "backup") {
				backupErrors++
			}
			continue
		}
		if strings.EqualFold(entry.JobType, "backup") && strings.EqualFold(entry.Event, "job_success") {
			backupSuccess++
		}
	}
	sort.Slice(errors, func(i, j int) bool { return errors[i].CreatedAt.Before(errors[j].CreatedAt) })

	dayLabel := day.Format("2006-01-02")
	if len(errors) == 0 {
		return slack.Attachment{
			Color:    "#2eb886",
			Fallback: fmt.Sprintf("Activity log daily summary - %s - all backups completed successfully", dayLabel),
			Blocks: slack.Blocks{BlockSet: []slack.Block{
				slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", "Activity Log Daily Summary", false, false)),
				slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
					fmt.Sprintf("*%s*\nAll backup jobs completed cleanly today. No activity-log errors were found.", escapeActivitySlack(dayLabel)),
					false, false), nil, nil),
				slack.NewContextBlock("",
					slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Backup successes:* `%d`", backupSuccess), false, false),
				),
			}},
		}
	}

	lines := make([]string, 0, len(errors))
	maxItems := 12
	for i := range errors {
		if i >= maxItems {
			break
		}
		entry := errors[i]
		when := entry.CreatedAt.In(time.Local).UTC().Add(displayOffset).Format("15:04")
		device := strings.TrimSpace(entry.DeviceName)
		if device == "" {
			device = strings.TrimSpace(entry.Host)
		}
		if device == "" {
			device = "system"
		}
		jobType := strings.TrimSpace(entry.JobType)
		if jobType == "" {
			jobType = "job"
		}
		msg := firstNonEmptyActivity(strings.TrimSpace(entry.Message), strings.TrimSpace(entry.Command), strings.TrimSpace(entry.Event))
		lines = append(lines, fmt.Sprintf("*%s* `%s` - *%s* - %s",
			escapeActivitySlack(when),
			escapeActivitySlack(jobType),
			escapeActivitySlack(device),
			escapeActivitySlack(truncateActivitySlack(msg, 420)),
		))
	}
	if extra := len(errors) - len(lines); extra > 0 {
		lines = append(lines, fmt.Sprintf("_and %d more error(s) in the Activity Log_", extra))
	}

	return slack.Attachment{
		Color:    "#d72b2b",
		Fallback: fmt.Sprintf("Activity log daily summary - %s - %d error(s)", dayLabel, len(errors)),
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", "Activity Log Daily Summary", false, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("*%s*\n%d error(s) found today. Backup errors: `%d`.",
					escapeActivitySlack(dayLabel), len(errors), backupErrors),
				false, false), nil, nil),
			slack.NewDividerBlock(),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", strings.Join(lines, "\n"), false, false), nil, nil),
			slack.NewContextBlock("",
				slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Backup successes:* `%d`", backupSuccess), false, false),
			),
		}},
	}
}

func firstNonEmptyActivity(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return "no details"
}

func truncateActivitySlack(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max-3] + "..."
}

func escapeActivitySlack(value string) string {
	value = strings.ReplaceAll(value, "&", "&amp;")
	value = strings.ReplaceAll(value, "<", "&lt;")
	value = strings.ReplaceAll(value, ">", "&gt;")
	return value
}

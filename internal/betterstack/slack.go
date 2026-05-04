package betterstack

import (
	"fmt"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/syslog"
	"github.com/slack-go/slack"
)

const (
	colorAlert    = "#d72b2b"
	colorResolved = "#2eb886"
)

func BuildIncidentAttachment(inc Incident) slack.Attachment {
	title := strings.TrimSpace(inc.Name)
	if title == "" {
		title = "Better Stack incident"
	}
	header := slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", truncatePlain("Better Stack - "+title, 140), false, false))
	blocks := []slack.Block{header}

	context := []slack.MixedElement{
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Incident:* `%s`", escapeSlack(inc.ID)), false, false),
	}
	if inc.Status != "" {
		context = append(context, slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Status:* `%s`", escapeSlack(inc.Status)), false, false))
	}
	if inc.TeamName != "" {
		context = append(context, slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Team:* %s", escapeSlack(inc.TeamName)), false, false))
	}
	blocks = append(blocks, slack.NewContextBlock("", context...))

	lines := []string{fmt.Sprintf("*Started:* `%s`", formatUTC(inc.StartedAt))}
	if inc.AcknowledgedAt != nil {
		lines = append(lines, fmt.Sprintf("*Acknowledged:* `%s`", formatUTC(*inc.AcknowledgedAt)))
	}
	if inc.Cause != "" {
		lines = append(lines, fmt.Sprintf("*Cause:* %s", escapeSlack(inc.Cause)))
	}
	if inc.URL != "" {
		lines = append(lines, fmt.Sprintf("*URL:* <%s|%s>", escapeSlack(inc.URL), escapeSlack(inc.URL)))
	}
	if inc.OriginURL != "" && inc.OriginURL != inc.URL {
		lines = append(lines, fmt.Sprintf("*Origin:* <%s|%s>", escapeSlack(inc.OriginURL), escapeSlack(inc.OriginURL)))
	}
	blocks = append(blocks, slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", strings.Join(lines, "\n"), false, false), nil, nil))

	return slack.Attachment{
		Color:    colorAlert,
		Fallback: "Better Stack incident - " + title,
		Blocks:   slack.Blocks{BlockSet: blocks},
	}
}

func BuildResolvedAttachment(inc models.BetterStackSlackIncident, resolvedAt time.Time, resolvedBy string) slack.Attachment {
	title := strings.TrimSpace(inc.Name)
	if title == "" {
		title = "Better Stack incident"
	}
	by := strings.TrimSpace(resolvedBy)
	if by == "" {
		by = "Better Stack"
	}
	body := fmt.Sprintf("*Resolved:* `%s`\n*Resolved by:* %s\n*Incident:* `%s`",
		formatUTC(resolvedAt), escapeSlack(by), escapeSlack(inc.BetterStackIncidentID))
	return slack.Attachment{
		Color:    colorResolved,
		Fallback: "Resolved - " + title,
		Blocks: slack.Blocks{BlockSet: []slack.Block{
			slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", truncatePlain("Resolved - "+title, 140), false, false)),
			slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn", body, false, false), nil, nil),
		}},
	}
}

func FirstThreadReminder(cfg *config.Config) string {
	mention := syslog.SlackTeamMention("")
	interval := 6 * time.Hour
	if cfg != nil {
		mention = syslog.SlackTeamMention(cfg.BetterStackSlackTeamMention)
		interval = reminderInterval(cfg)
	}
	return fmt.Sprintf("%s - Better Stack incident is unresolved. Reminders every *%s* until Better Stack resolves it.", mention, syslog.HumanReminderEvery(interval))
}

func ReminderText(cfg *config.Config) string {
	interval := reminderInterval(cfg)
	mention := syslog.SlackTeamMention("")
	if cfg != nil {
		mention = syslog.SlackTeamMention(cfg.BetterStackSlackTeamMention)
	}
	return ":alarm_clock: " + mention + " - Reminder: this Better Stack incident is still unresolved. (Next reminder in ~" + syslog.HumanReminderEvery(interval) + ".)"
}

func reminderInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.BetterStackReminderInterval < time.Hour {
		return 6 * time.Hour
	}
	return cfg.BetterStackReminderInterval
}

func formatUTC(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func truncatePlain(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "..."
}

func escapeSlack(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

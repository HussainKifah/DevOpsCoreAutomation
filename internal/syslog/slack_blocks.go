package syslog

import (
	"fmt"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/slack-go/slack"
)

const (
	slackColorAlert   = "#d72b2b"
	slackColorResolved = "#2eb886"
)

// formatSyslogSlackLocalTime formats alert time for Slack: add offset to UTC, no ISO T/Z (YYYY-MM-DD HH:MM:SS).
func formatSyslogSlackLocalTime(t time.Time, utcOffset time.Duration) string {
	return t.UTC().Add(utcOffset).Format("2006-01-02 15:04:05")
}

// SlackTeamMention returns mrkdwn snippet for the IP team ping (user group ID or fallback text).
func SlackTeamMention(teamMention string) string {
	if s := strings.TrimSpace(teamMention); s != "" {
		return s
	}
	return "@ip-core"
}

// BuildSyslogSlackAttachment builds a single attachment (red/green left border) with block content.
// displayOffset shifts UTC timestamps for display (e.g. 3h). truncatedExtra: alerts omitted count footer.
func BuildSyslogSlackAttachment(alerts []models.EsSyslogAlert, resolved bool, resolvedAt *time.Time, resolvedBy string, truncatedExtra int, displayOffset time.Duration) slack.Attachment {
	if len(alerts) == 0 {
		return slack.Attachment{Color: slackColorAlert, Fallback: "Syslog alert"}
	}
	dev := strings.TrimSpace(alerts[0].DeviceName)
	if dev == "" {
		dev = "Unknown device"
	}
	host := strings.TrimSpace(alerts[0].Host)

	var title string
	if resolved {
		title = fmt.Sprintf("Resolved — %s", dev)
	} else {
		title = fmt.Sprintf("Syslog alert — %s", dev)
	}

	header := slack.NewHeaderBlock(slack.NewTextBlockObject("plain_text", truncatePlain(title, 140), false, false))

	ctxBits := []slack.MixedElement{
		slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Host:* `%s`", escapeSlack(host)), false, false),
	}
	if resolved && resolvedAt != nil {
		by := strings.TrimSpace(resolvedBy)
		if by == "" {
			by = "someone"
		}
		ctxBits = append(ctxBits,
			slack.NewTextBlockObject("mrkdwn", fmt.Sprintf("*Marked resolved* _%s_ by %s",
				formatSyslogSlackLocalTime(*resolvedAt, displayOffset), escapeSlack(by)), false, false),
		)
	}
	ctx := slack.NewContextBlock("", ctxBits...)

	blocks := []slack.Block{header, ctx}

	for i := range alerts {
		a := &alerts[i]
		if i > 0 {
			blocks = append(blocks, slack.NewDividerBlock())
		}
		lbl := strings.TrimSpace(a.FilterLabel)
		if lbl == "" {
			lbl = "filter"
		}
		tsLocal := formatSyslogSlackLocalTime(a.TimestampUTC, displayOffset)
		msg := strings.TrimSpace(a.Message)
		if len(msg) > 2800 {
			msg = msg[:2800] + "…"
		}
		body := fmt.Sprintf("*%s*\n_%s_\n```%s```",
			escapeSlack(lbl), escapeSlack(tsLocal), sanitizeCodeFence(msg))
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", body, false, false), nil, nil))
	}
	if truncatedExtra > 0 {
		blocks = append(blocks, slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("_…and %d more alert(s) — see syslog alerts in DevOps_", truncatedExtra),
				false, false)))
	}

	color := slackColorAlert
	if resolved {
		color = slackColorResolved
	}

	return slack.Attachment{
		Color:    color,
		Fallback: title,
		Blocks:   slack.Blocks{BlockSet: blocks},
	}
}

// BuildSyslogSlackThreadAppendAttachment is a compact attachment for follow-up hits on an existing open incident (thread reply).
func BuildSyslogSlackThreadAppendAttachment(alerts []models.EsSyslogAlert, displayOffset time.Duration, truncatedExtra int) slack.Attachment {
	if len(alerts) == 0 {
		return slack.Attachment{Color: slackColorAlert, Fallback: "Syslog follow-up"}
	}
	blocks := []slack.Block{
		slack.NewSectionBlock(slack.NewTextBlockObject("mrkdwn",
			"_Same issue — additional occurrence(s):_", false, false), nil, nil),
	}
	for i := range alerts {
		a := &alerts[i]
		if i > 0 {
			blocks = append(blocks, slack.NewDividerBlock())
		}
		lbl := strings.TrimSpace(a.FilterLabel)
		if lbl == "" {
			lbl = "filter"
		}
		tsLocal := formatSyslogSlackLocalTime(a.TimestampUTC, displayOffset)
		msg := strings.TrimSpace(a.Message)
		if len(msg) > 1200 {
			msg = msg[:1200] + "…"
		}
		body := fmt.Sprintf("*%s*\n_%s_\n```%s```",
			escapeSlack(lbl), escapeSlack(tsLocal), sanitizeCodeFence(msg))
		blocks = append(blocks, slack.NewSectionBlock(
			slack.NewTextBlockObject("mrkdwn", body, false, false), nil, nil))
	}
	if truncatedExtra > 0 {
		blocks = append(blocks, slack.NewContextBlock("",
			slack.NewTextBlockObject("mrkdwn",
				fmt.Sprintf("_…and %d more — see DevOps syslog UI_", truncatedExtra), false, false)))
	}
	return slack.Attachment{
		Color:    slackColorAlert,
		Fallback: "Syslog follow-up",
		Blocks:   slack.Blocks{BlockSet: blocks},
	}
}

func truncatePlain(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func escapeSlack(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func sanitizeCodeFence(s string) string {
	s = strings.ReplaceAll(s, "```", "`\u200b``")
	return s
}

// HumanReminderEvery renders reminder interval for Slack copy (e.g. "6 hours").
func HumanReminderEvery(d time.Duration) string {
	if d < time.Hour {
		return d.Round(time.Minute).String()
	}
	h := d / time.Hour
	if d%time.Hour < 2*time.Minute {
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	return d.Round(time.Minute).String()
}

// SlackSyslogFirstThreadReminder is the first reply under the alert (instructions + mention).
func SlackSyslogFirstThreadReminder(cfg *config.Config) string {
	if cfg == nil {
		return SlackTeamMention("") + " — Add a :white_check_mark: reaction to the main alert message (or any message in this thread) when resolved."
	}
	m := SlackTeamMention(cfg.SlackSyslogTeamMention)
	every := HumanReminderEvery(cfg.SlackReminderInterval)
	return fmt.Sprintf("%s — Add a :white_check_mark: reaction to the main alert message (or any message in this thread) when resolved. Reminders every *%s* until then.", m, every)
}

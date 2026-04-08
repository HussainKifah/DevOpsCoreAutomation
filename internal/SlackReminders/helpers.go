package slackreminders

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/slack-go/slack"
)

const resolutionLinePrefix = "Reminder Status:"

type ThreadReply struct {
	TS       string
	UserID   string
	UserName string
	Text     string
	At       time.Time
}

func LooksLikeTicketMessage(text string) bool {
	body := strings.ToLower(strings.TrimSpace(text))
	if body == "" {
		return false
	}
	if !strings.Contains(body, "new request") && !strings.Contains(body, "new incident") {
		return false
	}
	for _, part := range []string{"date:", "province:", "to team:", "sender:"} {
		if !strings.Contains(body, part) {
			return false
		}
	}
	// Original ticket bot uses "Type:"; incident-style templates often use "Project:" instead.
	if !strings.Contains(body, "type:") && !strings.Contains(body, "project:") {
		return false
	}
	return true
}

func BuildTicketRecord(cfg *config.Config, channelID, messageTS, threadRootTS, text string, now time.Time) *models.SlackTicketReminder {
	title := fieldValue(text, "Customer Name:")
	if title == "" {
		title = fieldValue(text, "Customer Profiles:")
	}
	if title == "" {
		title = fieldValue(text, "Description:")
	}
	if title == "" {
		title = fieldValue(text, "Outage:")
	}
	if title == "" {
		title = fieldValue(text, "Project:")
	}
	if title == "" {
		title = "Slack ticket"
	}
	ticketType := fieldValue(text, "Type:")
	if ticketType == "" {
		ticketType = fieldValue(text, "Project:")
	}
	if ticketType == "" {
		ticketType = fieldValue(text, "Outage:")
	}
	interval := cfg.SlackTicketFirstReminderAfter
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}
	return &models.SlackTicketReminder{
		ChannelID:      strings.TrimSpace(channelID),
		MessageTS:      strings.TrimSpace(messageTS),
		ThreadRootTS:   strings.TrimSpace(threadRootTS),
		MessageText:    strings.TrimSpace(text),
		TicketTitle:    truncate(title, 255),
		Province:       truncate(fieldValue(text, "Province:"), 255),
		TicketType:     truncate(ticketType, 255),
		ToTeam:         truncate(fieldValue(text, "To Team:"), 255),
		Sender:         truncate(fieldValue(text, "Sender:"), 255),
		NextReminderAt: now.UTC().Add(interval),
	}
}

func fieldValue(text, label string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.ReplaceAll(line, "\u00a0", " "))
		if strings.HasPrefix(strings.ToLower(line), strings.ToLower(label)) {
			return strings.TrimSpace(strings.TrimPrefix(line, label))
		}
	}
	return ""
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func FormatSlackLocalTime(t time.Time, offset time.Duration) string {
	return t.UTC().Add(offset).Format("2006-01-02 15:04:05")
}

func HumanEvery(d time.Duration) string {
	if d < time.Minute {
		return "a moment"
	}
	if d%time.Hour == 0 {
		h := int(d / time.Hour)
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	}
	if d%time.Minute == 0 {
		m := int(d / time.Minute)
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	}
	return d.String()
}

func TeamMention(mention string) string {
	if s := strings.TrimSpace(mention); s != "" {
		return s
	}
	return "@ip-team"
}

func BuildNoReplyReminder(cfg *config.Config, ticket *models.SlackTicketReminder) string {
	title := truncate(ticketLabel(ticket), 120)
	return fmt.Sprintf(
		":alarm_clock: %s Reminder: ticket *%s* still has no replies. Please update this thread. (Next reminder in ~%s.)",
		ticketTeam(ticket, cfg),
		escapeSlack(title),
		HumanEvery(cfg.SlackTicketReminderInterval),
	)
}

func BuildTrackingStartedReminder(cfg *config.Config, ticket *models.SlackTicketReminder) string {
	title := truncate(ticketLabel(ticket), 120)
	return fmt.Sprintf(
		":alarm_clock: %s Reminder tracking started for *%s*. I will reply in this thread every ~%s until someone adds :white_check_mark: to mark it resolved.",
		ticketTeam(ticket, cfg),
		escapeSlack(title),
		HumanEvery(cfg.SlackTicketReminderInterval),
	)
}

func BuildReplyReminder(cfg *config.Config, ticket *models.SlackTicketReminder, reply *ThreadReply) string {
	label := ticketLabel(ticket)
	when := FormatSlackLocalTime(reply.At, cfg.SlackTicketDisplayOffset)
	owner := strings.TrimSpace(reply.UserName)
	if reply.UserID != "" {
		owner = "<@" + reply.UserID + ">"
	}
	if owner == "" {
		owner = "someone"
	}
	text := truncate(strings.ReplaceAll(strings.TrimSpace(reply.Text), "\n", " "), 220)
	if text == "" {
		text = "No text preview."
	}
	return fmt.Sprintf(
		":alarm_clock: Reminder for *%s*.\nLast reply: %s at `%s`\n>%s\nAdd :white_check_mark: when this ticket is done. (Next reminder in ~%s.)",
		escapeSlack(label),
		owner,
		when,
		escapeSlack(text),
		HumanEvery(cfg.SlackTicketReminderInterval),
	)
}

func ticketLabel(ticket *models.SlackTicketReminder) string {
	parts := make([]string, 0, 3)
	if s := strings.TrimSpace(ticket.Province); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(ticket.TicketType); s != "" {
		parts = append(parts, s)
	}
	if s := strings.TrimSpace(ticket.TicketTitle); s != "" {
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "Slack ticket"
	}
	return strings.Join(parts, " | ")
}

func ticketTeam(ticket *models.SlackTicketReminder, cfg *config.Config) string {
	if ticket != nil {
		if s := strings.TrimSpace(ticket.ToTeam); s != "" {
			return s
		}
		if s := fieldValue(ticket.MessageText, "To Team:"); s != "" {
			return s
		}
	}
	return TeamMention(cfg.SlackTicketIPTeamMention)
}

func BuildResolvedStatus(cfg *config.Config, resolvedAt time.Time, resolvedBy string) string {
	owner := strings.TrimSpace(resolvedBy)
	if owner == "" {
		owner = "someone"
	}
	return fmt.Sprintf(
		":white_check_mark: Resolved at %s by %s. Reminders stopped.",
		FormatSlackLocalTime(resolvedAt, cfg.SlackTicketDisplayOffset),
		owner,
	)
}

func AppendResolutionLine(original string, cfg *config.Config, resolvedAt time.Time, resolvedBy string) string {
	updated := strings.TrimRight(original, "\n")
	line := resolutionLinePrefix + " " + BuildResolvedStatus(cfg, resolvedAt, resolvedBy)
	if strings.Contains(updated, resolutionLinePrefix) {
		return updated
	}
	if updated == "" {
		return line
	}
	return updated + "\n\n" + line
}

func IsResolveReaction(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "white_check_mark", "heavy_check_mark", "ballot_box_with_check":
		return true
	default:
		return false
	}
}

func ResolveThreadParentTS(api *slack.Client, channelID, itemTS string) string {
	if api == nil || itemTS == "" {
		return ""
	}
	resp, err := api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    itemTS,
		Limit:     1,
		Inclusive: true,
	})
	if err != nil || resp == nil || len(resp.Messages) == 0 {
		return itemTS
	}
	m := resp.Messages[0]
	if strings.TrimSpace(m.ThreadTimestamp) != "" {
		return m.ThreadTimestamp
	}
	return m.Timestamp
}

func parseSlackTimestamp(ts string) time.Time {
	ts = strings.TrimSpace(ts)
	if ts == "" {
		return time.Time{}
	}
	parts := strings.SplitN(ts, ".", 2)
	sec, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0).UTC()
}

func escapeSlack(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

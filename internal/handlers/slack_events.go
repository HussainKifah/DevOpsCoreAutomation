package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	ruijie "github.com/Flafl/DevOpsCore/internal/Ruijie"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/syslog"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
)

// SlackEventsHandler handles Slack Events API (URL verification + reaction to resolve).
type SlackEventsHandler struct {
	cfg        *config.Config
	repo       *repository.EsSyslogRepository
	ruijieRepo *repository.RuijieMailRepository
	api        *slack.Client
	secret     string
	botUserID  string

	seenMu sync.Mutex
	seen   map[string]time.Time
}

func NewSlackEventsHandler(cfg *config.Config, repo *repository.EsSyslogRepository, ruijieRepo *repository.RuijieMailRepository, api *slack.Client) *SlackEventsHandler {
	h := &SlackEventsHandler{
		cfg:        cfg,
		repo:       repo,
		ruijieRepo: ruijieRepo,
		api:        api,
		secret:     strings.TrimSpace(cfg.SlackSigningSecret),
		seen:       make(map[string]time.Time),
	}
	if api != nil {
		if a, err := api.AuthTest(); err == nil {
			h.botUserID = a.UserID
		} else {
			log.Printf("[slack-events] AuthTest: %v", err)
		}
	}
	return h
}

func (h *SlackEventsHandler) Handle(c *gin.Context) {
	if h == nil || h.secret == "" || h.api == nil {
		c.Status(http.StatusNotFound)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}
	sig := c.GetHeader("X-Slack-Signature")
	ts := c.GetHeader("X-Slack-Request-Timestamp")
	if err := syslog.VerifySlackRequestSignature(h.secret, body, sig, ts); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}

	var outer struct {
		Type      string          `json:"type"`
		Challenge string          `json:"challenge"`
		Event     json.RawMessage `json:"event"`
		EventID   string          `json:"event_id"`
	}
	if err := json.Unmarshal(body, &outer); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "json"})
		return
	}

	if outer.Type == "url_verification" {
		c.JSON(http.StatusOK, gin.H{"challenge": outer.Challenge})
		return
	}

	if outer.Type != "event_callback" {
		c.Status(http.StatusOK)
		return
	}

	if outer.EventID != "" && h.dedupe(outer.EventID) {
		c.Status(http.StatusOK)
		return
	}

	var evType struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(outer.Event, &evType); err != nil {
		c.Status(http.StatusOK)
		return
	}

	switch evType.Type {
	case "reaction_added":
		h.handleReactionAdded(outer.Event)
	case "reaction_removed":
		h.handleReactionRemoved(outer.Event)
	default:
		// Other event types ignored
	}
	c.Status(http.StatusOK)
}

func (h *SlackEventsHandler) handleReactionAdded(raw json.RawMessage) {
	ev, ok := h.parseReactionEvent(raw)
	if !ok {
		return
	}
	if h.botUserID != "" && ev.User == h.botUserID {
		return
	}
	if isResolveReaction(ev.Reaction) {
		h.resolveSyslogIncident(ev.Channel, ev.ParentTS, ev.User)
		h.resolveRuijieIncident(ev.Channel, ev.ParentTS, ev.User)
		return
	}
	if isPauseReaction(ev.Reaction) {
		h.setSyslogIncidentSnoozed(ev.Channel, ev.ParentTS, ev.User, true)
		h.setRuijieIncidentSnoozed(ev.Channel, ev.ParentTS, ev.User, true)
	}
}

func (h *SlackEventsHandler) handleReactionRemoved(raw json.RawMessage) {
	ev, ok := h.parseReactionEvent(raw)
	if !ok {
		return
	}
	if h.botUserID != "" && ev.User == h.botUserID {
		return
	}
	if isPauseReaction(ev.Reaction) {
		h.setSyslogIncidentSnoozed(ev.Channel, ev.ParentTS, ev.User, false)
		h.setRuijieIncidentSnoozed(ev.Channel, ev.ParentTS, ev.User, false)
	}
}

func (h *SlackEventsHandler) resolveSyslogIncident(ch, parentTS, userID string) {
	if h.repo == nil || ch != strings.TrimSpace(h.cfg.SlackChannelID) {
		return
	}
	inc, err := h.repo.GetSlackIncidentByMessage(ch, parentTS)
	if err != nil || inc == nil {
		return
	}
	if inc.ResolvedAt != nil {
		return
	}

	resolvedBy := userID
	if u, err := h.api.GetUserInfo(userID); err == nil && u != nil {
		if u.RealName != "" {
			resolvedBy = u.RealName
		} else if u.Name != "" {
			resolvedBy = u.Name
		}
	}

	now := time.Now().UTC()
	if err := h.repo.MarkSlackIncidentResolved(inc.ID, resolvedBy, now); err != nil {
		log.Printf("[slack-events] mark resolved: %v", err)
		return
	}

	alerts, err := h.repo.AlertsForSlackIncident(inc.ID)
	if err != nil {
		log.Printf("[slack-events] load alerts: %v", err)
		return
	}

	att := syslog.BuildSyslogSlackAttachment(alerts, true, &now, resolvedBy, 0, h.cfg.SlackSyslogDisplayOffset)
	_, _, _, err = h.api.UpdateMessage(inc.ChannelID, inc.MessageTS, slack.MsgOptionAttachments(att))
	if err != nil {
		log.Printf("[slack-events] UpdateMessage: %v", err)
	}

	_, _, _ = h.api.PostMessage(ch,
		slack.MsgOptionTS(parentTS),
		slack.MsgOptionText(":white_check_mark: Issue marked resolved (reaction). Reminders stopped.", false),
	)
}

func (h *SlackEventsHandler) setSyslogIncidentSnoozed(ch, parentTS, userID string, snoozed bool) {
	if h.repo == nil || ch != strings.TrimSpace(h.cfg.SlackChannelID) {
		return
	}
	inc, err := h.repo.GetSlackIncidentByMessage(ch, parentTS)
	if err != nil || inc == nil || inc.ResolvedAt != nil {
		return
	}
	if snoozed && inc.SnoozedAt != nil {
		return
	}
	if !snoozed && inc.SnoozedAt == nil {
		return
	}

	name := h.lookupSlackName(userID)
	now := time.Now().UTC()
	if snoozed {
		if err := h.repo.SnoozeSlackIncident(inc.ID, name, now); err != nil {
			log.Printf("[slack-events] snooze syslog incident: %v", err)
			return
		}
		_, _, _ = h.api.PostMessage(ch,
			slack.MsgOptionTS(parentTS),
			slack.MsgOptionText(":hourglass_flowing_sand: Syslog reminders are paused for this alarm fingerprint until this reaction is removed. Repeated alarm thread replies will continue.", false),
		)
		return
	}

	next := now.Add(h.syslogReminderInterval())
	if err := h.repo.UnsnoozeSlackIncident(inc.ID, next); err != nil {
		log.Printf("[slack-events] unsnooze syslog incident: %v", err)
		return
	}
	_, _, _ = h.api.PostMessage(ch,
		slack.MsgOptionTS(parentTS),
		slack.MsgOptionText(":white_check_mark: Syslog fingerprint reminder pause removed. Reminders are active again.", false),
	)
}

func (h *SlackEventsHandler) resolveRuijieIncident(ch, parentTS, userID string) {
	if h.ruijieRepo == nil || ch != strings.TrimSpace(h.cfg.RuijieSlackChannelID) {
		return
	}
	inc, err := h.ruijieRepo.GetSlackIncidentByMessage(ch, parentTS)
	if err != nil || inc == nil {
		return
	}
	if inc.ResolvedAt != nil {
		return
	}

	resolvedBy := userID
	if u, err := h.api.GetUserInfo(userID); err == nil && u != nil {
		if u.RealName != "" {
			resolvedBy = u.RealName
		} else if u.Name != "" {
			resolvedBy = u.Name
		}
	}

	now := time.Now().UTC()
	if err := h.ruijieRepo.MarkSlackIncidentResolved(inc.ID, resolvedBy, now); err != nil {
		log.Printf("[ruijie-mail] mark resolved: %v", err)
		return
	}

	alerts, err := h.ruijieRepo.AlertsForSlackIncident(inc.ID)
	if err != nil {
		log.Printf("[ruijie-mail] load alerts: %v", err)
		return
	}

	att := ruijie.BuildRuijieSlackAttachment(alerts, true, &now, resolvedBy, h.cfg.RuijieSlackDisplayOffset)
	_, _, _, err = h.api.UpdateMessage(inc.ChannelID, inc.MessageTS, slack.MsgOptionAttachments(att))
	if err != nil {
		log.Printf("[ruijie-mail] UpdateMessage: %v", err)
	}

	_, _, _ = h.api.PostMessage(ch,
		slack.MsgOptionTS(parentTS),
		slack.MsgOptionText(":white_check_mark: Ruijie alarm marked resolved. Reminders stopped.", false),
	)
}

func (h *SlackEventsHandler) setRuijieIncidentSnoozed(ch, parentTS, userID string, snoozed bool) {
	if h.ruijieRepo == nil || ch != strings.TrimSpace(h.cfg.RuijieSlackChannelID) {
		return
	}
	inc, err := h.ruijieRepo.GetSlackIncidentByMessage(ch, parentTS)
	if err != nil || inc == nil || inc.ResolvedAt != nil {
		return
	}
	if snoozed && inc.SnoozedAt != nil {
		return
	}
	if !snoozed && inc.SnoozedAt == nil {
		return
	}

	name := h.lookupSlackName(userID)
	now := time.Now().UTC()
	if snoozed {
		if err := h.ruijieRepo.SnoozeSlackIncident(inc.ID, name, now); err != nil {
			log.Printf("[ruijie-mail] snooze incident: %v", err)
			return
		}
		_, _, _ = h.api.PostMessage(ch,
			slack.MsgOptionTS(parentTS),
			slack.MsgOptionText(":hourglass_flowing_sand: Ruijie reminders and repeated thread replies are paused for this alarm fingerprint until this reaction is removed.", false),
		)
		return
	}

	next := now.Add(ruijieReminderInterval(h.cfg))
	if err := h.ruijieRepo.UnsnoozeSlackIncident(inc.ID, next); err != nil {
		log.Printf("[ruijie-mail] unsnooze incident: %v", err)
		return
	}
	_, _, _ = h.api.PostMessage(ch,
		slack.MsgOptionTS(parentTS),
		slack.MsgOptionText(":white_check_mark: Ruijie fingerprint pause removed. Reminders and repeated thread replies are active again.", false),
	)
}

func (h *SlackEventsHandler) dedupe(eventID string) bool {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()
	now := time.Now()
	for k, t := range h.seen {
		if now.Sub(t) > 10*time.Minute {
			delete(h.seen, k)
		}
	}
	if _, ok := h.seen[eventID]; ok {
		return true
	}
	h.seen[eventID] = now
	return false
}

type slackReactionEvent struct {
	User      string
	Reaction  string
	Channel   string
	MessageTS string
	ParentTS  string
}

func (h *SlackEventsHandler) parseReactionEvent(raw json.RawMessage) (slackReactionEvent, bool) {
	var ev struct {
		User     string `json:"user"`
		Reaction string `json:"reaction"`
		Item     struct {
			Type     string `json:"type"`
			Channel  string `json:"channel"`
			TS       string `json:"ts"`
			ThreadTS string `json:"thread_ts"`
		} `json:"item"`
	}
	if err := json.Unmarshal(raw, &ev); err != nil {
		return slackReactionEvent{}, false
	}
	if ev.Item.Type != "message" {
		return slackReactionEvent{}, false
	}
	ch := strings.TrimSpace(ev.Item.Channel)
	if ch == "" {
		return slackReactionEvent{}, false
	}
	messageTS := strings.TrimSpace(ev.Item.TS)
	parentTS := strings.TrimSpace(ev.Item.ThreadTS)
	if parentTS == "" {
		parentTS = h.resolveThreadParentTS(ch, messageTS)
	}
	if parentTS == "" {
		return slackReactionEvent{}, false
	}
	return slackReactionEvent{
		User:      ev.User,
		Reaction:  strings.TrimSpace(ev.Reaction),
		Channel:   ch,
		MessageTS: messageTS,
		ParentTS:  parentTS,
	}, true
}

func isResolveReaction(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "white_check_mark", "heavy_check_mark", "ballot_box_with_check":
		return true
	default:
		return false
	}
}

func (h *SlackEventsHandler) resolveThreadParentTS(channelID, itemTS string) string {
	if h == nil || h.api == nil || itemTS == "" {
		return ""
	}
	resp, err := h.api.GetConversationHistory(&slack.GetConversationHistoryParameters{
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

func (h *SlackEventsHandler) lookupSlackName(userID string) string {
	name := userID
	if u, err := h.api.GetUserInfo(userID); err == nil && u != nil {
		if u.RealName != "" {
			name = u.RealName
		} else if u.Name != "" {
			name = u.Name
		}
	}
	return name
}

func (h *SlackEventsHandler) syslogReminderInterval() time.Duration {
	if h == nil || h.cfg == nil || h.cfg.SlackReminderInterval < time.Hour {
		return 6 * time.Hour
	}
	return h.cfg.SlackReminderInterval
}

func ruijieReminderInterval(cfg *config.Config) time.Duration {
	if cfg == nil || cfg.RuijieSlackReminderInterval < time.Hour {
		return 6 * time.Hour
	}
	return cfg.RuijieSlackReminderInterval
}

func isPauseReaction(reaction string) bool {
	switch strings.TrimSpace(strings.ToLower(reaction)) {
	case "hourglass", "hourglass_flowing_sand", "hourglass_done", "sand_clock":
		return true
	default:
		return false
	}
}

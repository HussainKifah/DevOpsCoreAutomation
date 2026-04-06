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
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/syslog"
	"github.com/gin-gonic/gin"
	"github.com/slack-go/slack"
)

// SlackEventsHandler handles Slack Events API (URL verification + reaction to resolve).
type SlackEventsHandler struct {
	cfg       *config.Config
	repo      *repository.EsSyslogRepository
	api       *slack.Client
	secret    string
	botUserID string

	seenMu sync.Mutex
	seen   map[string]time.Time
}

func NewSlackEventsHandler(cfg *config.Config, repo *repository.EsSyslogRepository, api *slack.Client) *SlackEventsHandler {
	h := &SlackEventsHandler{
		cfg:    cfg,
		repo:   repo,
		api:    api,
		secret: strings.TrimSpace(cfg.SlackSigningSecret),
		seen:   make(map[string]time.Time),
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
	default:
		// Other event types ignored
	}
	c.Status(http.StatusOK)
}

func (h *SlackEventsHandler) handleReactionAdded(raw json.RawMessage) {
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
		return
	}
	if ev.Item.Type != "message" {
		return
	}
	if !isResolveReaction(ev.Reaction) {
		return
	}
	if h.botUserID != "" && ev.User == h.botUserID {
		return
	}
	ch := strings.TrimSpace(ev.Item.Channel)
	if ch != strings.TrimSpace(h.cfg.SlackChannelID) {
		return
	}

	parentTS := strings.TrimSpace(ev.Item.ThreadTS)
	if parentTS == "" {
		parentTS = h.resolveThreadParentTS(ch, strings.TrimSpace(ev.Item.TS))
	}

	inc, err := h.repo.GetSlackIncidentByMessage(ch, parentTS)
	if err != nil || inc == nil {
		return
	}
	if inc.ResolvedAt != nil {
		return
	}

	resolvedBy := ev.User
	if u, err := h.api.GetUserInfo(ev.User); err == nil && u != nil {
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

// resolveThreadParentTS returns the root message ts for a reaction target (parent or fetched via history).
func (h *SlackEventsHandler) resolveThreadParentTS(channelID, itemTS string) string {
	if itemTS == "" {
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

func isResolveReaction(name string) bool {
	switch strings.TrimSpace(strings.ToLower(name)) {
	case "white_check_mark", "heavy_check_mark", "ballot_box_with_check":
		return true
	default:
		return false
	}
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

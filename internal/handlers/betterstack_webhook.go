package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const betterStackWebhookMaxBody = 1 << 20

type BetterStackWebhookHandler struct {
	secret string
}

func NewBetterStackWebhookHandler(secret string) *BetterStackWebhookHandler {
	return &BetterStackWebhookHandler{secret: strings.TrimSpace(secret)}
}

func (h *BetterStackWebhookHandler) Handle(c *gin.Context) {
	if h == nil {
		c.Status(http.StatusNotFound)
		return
	}
	if h.secret != "" && !h.validSecret(c) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid webhook secret"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, betterStackWebhookMaxBody+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "read body"})
		return
	}
	if len(body) > betterStackWebhookMaxBody {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "payload too large"})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
		return
	}

	summary := betterStackPayloadSummary(payload)
	log.Printf("[betterstack-webhook] event=%q incident=%q monitor=%q status=%q", summary.Event, summary.IncidentID, summary.MonitorName, summary.Status)

	c.JSON(http.StatusOK, gin.H{
		"ok":      true,
		"message": "betterstack webhook received",
		"event":   summary.Event,
	})
}

func (h *BetterStackWebhookHandler) validSecret(c *gin.Context) bool {
	provided := strings.TrimSpace(c.GetHeader("X-BetterStack-Webhook-Secret"))
	if provided == "" {
		auth := strings.TrimSpace(c.GetHeader("Authorization"))
		provided = strings.TrimPrefix(auth, "Bearer ")
	}
	if provided == "" {
		provided = strings.TrimSpace(c.Query("secret"))
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(h.secret)) == 1
}

type betterStackSummary struct {
	Event       string
	IncidentID  string
	MonitorName string
	Status      string
}

func betterStackPayloadSummary(payload map[string]any) betterStackSummary {
	return betterStackSummary{
		Event:       firstString(payload, "event", "event_type", "type"),
		IncidentID:  firstNestedString(payload, []string{"incident", "id"}, []string{"data", "incident", "id"}, []string{"incident_id"}, []string{"id"}),
		MonitorName: firstNestedString(payload, []string{"monitor", "name"}, []string{"data", "monitor", "name"}, []string{"monitor_name"}, []string{"name"}),
		Status:      firstNestedString(payload, []string{"incident", "status"}, []string{"data", "incident", "status"}, []string{"status"}),
	}
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstNestedString(payload map[string]any, paths ...[]string) string {
	for _, path := range paths {
		if value := nestedString(payload, path...); value != "" {
			return value
		}
	}
	return ""
}

func nestedString(value any, path ...string) string {
	current := value
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[key]
	}
	if text, ok := current.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

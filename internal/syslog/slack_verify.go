package syslog

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifySlackRequestSignature checks Slack's X-Slack-Signature (v0=...) for the raw body.
func VerifySlackRequestSignature(signingSecret string, body []byte, slackSig, slackTs string) error {
	if signingSecret == "" || slackSig == "" || slackTs == "" {
		return fmt.Errorf("missing signing parameters")
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(slackTs), 10, 64)
	if err != nil {
		return fmt.Errorf("bad timestamp")
	}
	if delta := time.Since(time.Unix(ts, 0)); delta > 5*time.Minute || delta < -1*time.Minute {
		return fmt.Errorf("stale timestamp")
	}
	mac := hmac.New(sha256.New, []byte(signingSecret))
	_, _ = mac.Write([]byte("v0:" + slackTs + ":"))
	_, _ = mac.Write(body)
	// Slack: X-Slack-Signature is v0=<hex(HMAC-SHA256)>, not base64.
	want := "v0=" + hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(slackSig)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

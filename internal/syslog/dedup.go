package syslog

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

var digitRuns = regexp.MustCompile(`\d+`)

// DedupFingerprint identifies “the same” syslog line across repeats (e.g. changing sequence numbers).
// Host + device name (case-insensitive) + message with every run of digits replaced by "#".
func DedupFingerprint(host, deviceName, message string) string {
	h := strings.TrimSpace(strings.ToLower(host))
	d := strings.TrimSpace(strings.ToLower(deviceName))
	m := strings.TrimSpace(message)
	m = digitRuns.ReplaceAllString(m, "#")
	raw := h + "\x00" + d + "\x00" + m
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

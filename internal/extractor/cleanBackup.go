package extractor

import (
	"regexp"
	"strings"
)

var (
	alarmLineRe  = regexp.MustCompile(`(?m)^\d{2}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} .+ alarm .+$`)
	spinnerRe    = regexp.MustCompile(`[-\\|/]{2,}[-\\|/ ]*`)
	blankLinesRe = regexp.MustCompile(`\n{3,}`)
)

func CleanBackupOutput(raw string) string {
	out := alarmLineRe.ReplaceAllString(raw, "")
	out = spinnerRe.ReplaceAllString(out, "")
	out = blankLinesRe.ReplaceAllString(out, "\n\n")
	out = strings.TrimSpace(out)
	return out
}

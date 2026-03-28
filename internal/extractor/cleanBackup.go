package extractor

import (
	"regexp"
	"strings"
)

var (
	// Lines containing major/minor alarm banners (any case).
	nokiaBackupAlarmRE = regexp.MustCompile(`(?i)\b(major|minor)\s+alarm\b`)
	// Dated alarm lines from Nokia CLI (e.g. "26/03/25 14:54:09 major alarm occurred ...").
	nokiaBackupAlarmDatedRE = regexp.MustCompile(`(?m)^\d{2}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} .+ alarm .+$`)
	// Short spinner-only lines from interactive CLI.
	nokiaBackupSpinnerLineRE = regexp.MustCompile(`^[\-\\|/\s]+$`)
)

// CleanNokiaBackupStep returns one SSH command’s output with noise removed:
// major/minor alarm lines, typ:devops echo, Nokia #--- separators, spinners, and blank lines.
// Non-empty lines are trimmed; inner newlines are preserved.
func CleanNokiaBackupStep(raw string) string {
	var b strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		tr := strings.TrimSpace(line)
		if tr == "" {
			continue
		}
		if strings.EqualFold(tr, "typ:devops") {
			continue
		}
		if nokiaDashSeparatorLine(tr) {
			continue
		}
		if nokiaBackupAlarmRE.MatchString(line) || nokiaBackupAlarmDatedRE.MatchString(line) {
			continue
		}
		if len(tr) < 32 && nokiaBackupSpinnerLineRE.MatchString(tr) {
			continue
		}
		b.WriteString(tr)
		b.WriteByte('\n')
	}
	return strings.TrimSpace(b.String())
}

// nokiaDashSeparatorLine matches banner lines like "#---...---" from Nokia CLI.
func nokiaDashSeparatorLine(s string) bool {
	if len(s) < 8 || s[0] != '#' {
		return false
	}
	for _, r := range s[1:] {
		if r != '-' {
			return false
		}
	}
	return true
}

// FinalizeNokiaMultistepBackup cleans each step’s raw output and joins chunks with a single space
// (no blank line between steps). Used when one backup file aggregates several CLI commands.
func FinalizeNokiaMultistepBackup(rawSteps []string) string {
	var parts []string
	for _, raw := range rawSteps {
		c := CleanNokiaBackupStep(raw)
		if c != "" {
			parts = append(parts, c)
		}
	}
	return strings.Join(parts, " ")
}

// CleanBackupOutput cleans a single Nokia CLI blob (e.g. one-shot backup job). Same rules as CleanNokiaBackupStep.
func CleanBackupOutput(raw string) string {
	return CleanNokiaBackupStep(raw)
}
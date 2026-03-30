package extractor

import (
	"regexp"
	"strconv"
	"strings"
)

type PortProtection struct {
	Port        string `json:"port"`
	PairedPort  string `json:"paired_port"`
	PortState   string `json:"port_state"`
	PairedState string `json:"paired_state"`
	SwoReason   string `json:"swo_reason"`
	NumSwo      int    `json:"num_swo"`
	alert       string `json:"alert"`
}

// Space-separated Nokia table row: port paired-port port-state paired-state swo-reason num-swo
var rePortSpace = regexp.MustCompile(
	`(?m)^(pon:\S+)\s+(pon:\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d+)\s*$`,
)

// Pipe-bordered CLI (ISAM): | port | paired-port | port-state | paired-state | swo-reason | num-swo |
var rePortPipe = regexp.MustCompile(
	`\|\s*(pon:\S+)\s*\|\s*(pon:\S+)\s*\|\s*(\S+)\s*\|\s*(\S+)\s*\|\s*([^|]+?)\s*\|\s*(\d+)\s*\|`,
)

func ExtractPortProtection(output string) []PortProtection {
	seen := make(map[string]bool)
	results := make([]PortProtection, 0, 32)

	add := func(m []string) {
		if len(m) < 7 {
			return
		}
		port, paired := m[1], m[2]
		key := port + "\x00" + paired
		if seen[key] {
			return
		}
		seen[key] = true
		n, _ := strconv.Atoi(m[6])
		results = append(results, PortProtection{
			Port:        port,
			PairedPort:  paired,
			PortState:   m[3],
			PairedState: m[4],
			SwoReason:   strings.TrimSpace(m[5]),
			NumSwo:      n,
		})
	}

	for _, m := range rePortSpace.FindAllStringSubmatch(output, -1) {
		add(m)
	}
	for _, m := range rePortPipe.FindAllStringSubmatch(output, -1) {
		add(m)
	}
	return results
}

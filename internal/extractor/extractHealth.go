package extractor

import (
	"regexp"
	"strconv"
	"strings"
)

type CpuLoad struct {
	Slot    string `json:"slot"`
	Average int    `json:"average_pct"`
}

type Temperature struct {
	Slot     string `json:"slot"`
	SensorID int    `json:"sensor_id"`
	ActTemp  int    `json:"act_temp"`
	TcaHigh  int    `json:"tca_high"`
	ShutHigh int    `json:"shut_high"`
}

type Health struct {
	Uptime       string        `json:"uptime"`
	CpuLoads     []CpuLoad     `json:"cpu_loads"`
	Temperatures []Temperature `json:"temperatures"`
}

var (
	reCpu      = regexp.MustCompile(`(?s)slot\s*:\s*(\S+)\s.*?average[^:\n]*:\s*(\d+)`)
	reCpuTable = regexp.MustCompile(`(?m)^\s*(nt-[a-z]\S*|lt:\d+/\d+/\d+)\s+\d+\s+(\d+)`)
	reUptime   = regexp.MustCompile(`System Up Time\s*:\s*(.+?)(?:\s*\(|$)`)
	reTemp     = regexp.MustCompile(`(?m)^((?:nt-[ab]|lt:\S+))\s+(\d+)\s+(\d+)\s+\d+\s+(\d+)\s+\d+\s+(\d+)`)
)

func ExtractHealth(output string) Health {
	var h Health

	// CPU loads — deduplicate by Slot (pool reuse can cause duplicate output)
	cpuSeen := map[string]bool{}
	for _, m := range reCpu.FindAllStringSubmatch(output, -1) {
		if cpuSeen[m[1]] {
			continue
		}
		cpuSeen[m[1]] = true
		avg, _ := strconv.Atoi(m[2])
		h.CpuLoads = append(h.CpuLoads, CpuLoad{Slot: m[1], Average: avg})
	}
	// Fallback: table format "nt-a  <current>  <average>" (FX-16 style)
	if len(h.CpuLoads) == 0 {
		for _, m := range reCpuTable.FindAllStringSubmatch(output, -1) {
			if cpuSeen[m[1]] {
				continue
			}
			cpuSeen[m[1]] = true
			avg, _ := strconv.Atoi(m[2])
			h.CpuLoads = append(h.CpuLoads, CpuLoad{Slot: m[1], Average: avg})
		}
	}

	// Uptime — take the LAST match so stale pool data is ignored
	if matches := reUptime.FindAllStringSubmatch(output, -1); len(matches) > 0 {
		h.Uptime = strings.TrimSpace(matches[len(matches)-1][1])
	}

	// Temperatures — deduplicate by Slot+SensorID
	tempSeen := map[string]bool{}
	for _, m := range reTemp.FindAllStringSubmatch(output, -1) {
		key := m[1] + ":" + m[2]
		if tempSeen[key] {
			continue
		}
		tempSeen[key] = true
		sid, _ := strconv.Atoi(m[2])
		act, _ := strconv.Atoi(m[3])
		tcaH, _ := strconv.Atoi(m[4])
		shutH, _ := strconv.Atoi(m[5])
		h.Temperatures = append(h.Temperatures, Temperature{
			Slot:     m[1],
			SensorID: sid,
			ActTemp:  act,
			TcaHigh:  tcaH,
			ShutHigh: shutH,
		})
	}

	return h
}
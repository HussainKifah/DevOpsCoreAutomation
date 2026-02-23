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
	reCpu    = regexp.MustCompile(`slot\s*:\s*(\S+)\s+.*?average\(%\)\s*:\s*(\d+)`)
	reUptime = regexp.MustCompile(`System Up Time\s*:\s*(.+?)\s*\(`)
	reTemp   = regexp.MustCompile(`(?m)^((?:nt-[ab]|lt:\S+))\s+(\d+)\s+(\d+)\s+\d+\s+(\d+)\s+\d+\s+(\d+)`)
)

func ExtractHealth(output string) Health {
	var h Health

	// CPU loads
	for _, m := range reCpu.FindAllStringSubmatch(output, -1) {
		avg, _ := strconv.Atoi(m[2])
		h.CpuLoads = append(h.CpuLoads, CpuLoad{
			Slot:    m[1],
			Average: avg,
		})
	}

	// Uptime
	if m := reUptime.FindStringSubmatch(output); m != nil {
		h.Uptime = strings.TrimSpace(m[1])
	}

	// Temperatures
	for _, m := range reTemp.FindAllStringSubmatch(output, -1) {
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
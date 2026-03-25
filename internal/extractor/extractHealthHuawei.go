package extractor

import (
	"regexp"
	"strconv"
	"strings"
)

type HuaweiHealth struct {
	Uptime       string              `json:"uptime"`
	CpuLoads     []HuaweiCpuLoad     `json:"cpu_loads"`
	Temperatures []HuaweiTemperature `json:"temperatures"`
}

type HuaweiCpuLoad struct {
	Slot    string `json:"slot"`
	CpuUsage int   `json:"cpu_usage_pct"`
}

type HuaweiTemperature struct {
	Slot    string `json:"slot"`
	TempC   int    `json:"temp_c"`
}

// "display sysuptime" output:  "System up time : 128 day(s), 3 hour(s), ..."
var reHwUptime = regexp.MustCompile(`(?i)(?:system\s+up\s+time|up\s*time)\s*:\s*(.+)`)

// "display cpu 0/1" output contains lines like:
//   CPU occupancy:  5%
var reHwCpu = regexp.MustCompile(`(?i)CPU occupancy\s*:\s*(\d+)`)

// "display temperature 0/1" — Huawei output variants:
//   The temperature of the board:  37C(  98F)
//   The current temperature of board is 45 degree(s)
var reHwTemp = regexp.MustCompile(`(?i)(?:temperature.*?:\s*(\d+)\s*C|current temperature.*?(\d+)\s*degree)`)

func ExtractHuaweiUptime(output string) string {
	if m := reHwUptime.FindStringSubmatch(output); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func ExtractHuaweiCpu(slot, output string) *HuaweiCpuLoad {
	if m := reHwCpu.FindStringSubmatch(output); len(m) > 1 {
		usage, _ := strconv.Atoi(m[1])
		return &HuaweiCpuLoad{Slot: slot, CpuUsage: usage}
	}
	return nil
}

func ExtractHuaweiTemperature(slot, output string) *HuaweiTemperature {
	if m := reHwTemp.FindStringSubmatch(output); len(m) > 1 {
		s := m[1]
		if s == "" {
			s = m[2]
		}
		if s != "" {
			temp, _ := strconv.Atoi(s)
			return &HuaweiTemperature{Slot: slot, TempC: temp}
		}
	}
	return nil
}

package extractor

import (
	"bufio"
	"regexp"
	"strings"
)

var EquipVendor = map[string]string{
	// Nokia ONT
	"G1425GE":   "Nokia ONT",
	"G1425GB":   "Nokia ONT",
	"XS-010X-Q": "Nokia ONT",
	"I240WA":    "Nokia ONT",
	// Nokia ONU
	"G-010G-R":             "Nokia ONU",
	"__________G-010G-R__": "Nokia ONU",
	// ORFA
	"OG-976V":  "ORFA",
	"OG-976V2": "ORFA",
	// HWTC (Huawei)
	"NEXXT G8421H": "HWTC",
	"HG8145V6":     "HWTC",
	"EG8145V5":     "HWTC",
	"EG8021V5":     "HWTC",
	"GP1702-1Gv2":  "HWTC",
	"HG8120C":      "HWTC",
	"HG8145C":      "HWTC",
	"HG8245C":      "HWTC",
	"HG8245Q2":     "HWTC",
	"HG8321V":      "HWTC",
	"HG8346M":      "HWTC",
	"HG8340M":      "HWTC",
	"HS8125C":      "HWTC",
	"HS8145C5":     "HWTC",
	"HS8546V":      "HWTC",
	"EG8041X6-10":  "HWTC",
	"K662D":        "HWTC",
	"JZ8600":       "HWTC",
	"SA1456C":      "HWTC",
	"OG-978VX":     "HWTC",
	"540M":         "HWTC",
	"EG8141A5":     "HWTC",
	"HG8245":       "HWTC",
	"HG8342R":      "HWTC",
	"HG8546M":      "HWTC",
	"OG-92SR":      "HWTC",
	"5850ON":       "HWTC",
	"HG8245A":      "HWTC",
	"HG8310M":      "HWTC",
	"HG8546M-RMS":  "HWTC",
	"HS8145V5":     "HWTC",
	"HG8020C":      "HWTC",
	"HG8240":       "HWTC",
	"HG8321R":      "HWTC",
	"HG8540M":      "HWTC",
	"HS8545M":      "HWTC",
	"EG8120L":      "HWTC",
	"HG8240R":      "HWTC",
	"HG8245H":      "HWTC",
	"HG8541M":      "HWTC",
	"HS8145C":      "HWTC",
	"P4021A":       "HWTC",
	"HG8346R":      "HWTC",
	"HS8545M5":     "HWTC",
	// BDCM (Broadcom)
	"GP1702-1G":   "BDCM",
	"GP1704-2F-E": "BDCM",
	"GP1705-2G":   "BDCM",
	"GP1706-4G":   "BDCM",
	"GP1706-4GV":  "BDCM",
}

type VendorCount struct {
	Vendor string `json:"vendor"`
	Count  int    `json:"count"`
}

func GetVender(equipID string) string {
	if v, ok := EquipVendor[equipID]; ok {
		return v
	}
	if v, ok := EquipVendor[strings.ToUpper(equipID)]; ok {
		return v
	}
	return "Unknown"
}

type EquipID struct {
	ID string `json:"equip_id"`
}
type EquipIDCount struct {
	ID    string `json:"equip_id"`
	Count int    `json:"count"`
}

func ExtractAllEquipID(output string) []EquipID {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")

	var results []EquipID

	re := regexp.MustCompile(`(?i)equip-id\s*:\s*(.*?)(?:\s{2,}|$)`)

	sc := bufio.NewScanner(strings.NewReader(output))
	for sc.Scan() {
		line := sc.Text()

		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			if len(m) >= 2 {
				id := strings.Trim(m[1], `"`)
				id = strings.TrimSpace(id)
				id = strings.TrimPrefix(id, "NOCLEICODE")
				id = strings.Trim(id, "_")

				idLower := strings.ToLower(id)
				if id != "" &&
					id != "equip_id" &&
					!strings.Contains(idLower, "actual-num-slots") {
					results = append(results, EquipID{ID: id})
				}
			}
		}
	}
	return results
}

func ExtractUniqueEquipID(output string) []string {
	all := ExtractAllEquipID(output)
	seen := make(map[string]bool)
	var unique []string
	for _, e := range all {
		if !seen[e.ID] {
			seen[e.ID] = true
			unique = append(unique, e.ID)
		}
	}
	return unique
}

func CountEquipIDs(output string) []EquipIDCount {
	all := ExtractAllEquipID(output)
	counts := make(map[string]int)
	var order []string

	for _, e := range all {
		if counts[e.ID] == 0 {
			order = append(order, e.ID)
		}
		counts[e.ID]++
	}
	results := make([]EquipIDCount, 0, len(order))
	for _, id := range order {
		results = append(results, EquipIDCount{ID: id, Count: counts[id]})
	}
	return results
}

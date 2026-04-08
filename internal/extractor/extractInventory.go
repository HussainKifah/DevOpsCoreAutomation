package extractor

import (
	"regexp"
	"strings"
)

type VendorCount struct {
	Vendor string `json:"vendor"`
	Count  int    `json:"count"`
}

var (
	reEquipID     = regexp.MustCompile(`(?i)equip-id\s*:\s*(.*?)(?:\s{2,}|$)`)
	reSwVerAct    = regexp.MustCompile(`(?i)sw-ver-act\s*:\s*(.+?)(?:\s{2,}|$)`)
	reVendorID    = regexp.MustCompile(`(?i)vendor-id\s*:\s*(\S+)`)
	reYpSerial    = regexp.MustCompile(`(?i)yp-serial-no\s*:\s*(.+)`)
	reOntID       = regexp.MustCompile(`(?i)ont-id\s*:\s*(\d+(?:/\d+)+)`)
	reOntIDAlt    = regexp.MustCompile(`(?i)equipment\s+ont\s+(?:ont-id\s+)?(\d+(?:/\d+)+)`)
	reOntIDLoose  = regexp.MustCompile(`(?i)ont-id\s*:?\s*(\d+(?:/\d+)+)`)
	reOntIDHyphen = regexp.MustCompile(`(?i)ont-id\s*:\s*(\d+(?:-\d+)+)`)
	blockSplit    = regexp.MustCompile(`(?m)^-{20,}\s*$`)
)

type EquipID struct {
	ID       string `json:"equip_id"`
	OntIdx   string `json:"ont_idx,omitempty"`
	Vendor   string `json:"vendor,omitempty"`
	SwVerAct string `json:"sw_ver_act,omitempty"`
	SerialNo string `json:"serial_no,omitempty"`
}

// OntInventoryItem holds per-ONT model and serial for devices tab.
type OntInventoryItem struct {
	OntIdx   string `json:"ont_idx"`
	EquipID  string `json:"equip_id"`
	SerialNo string `json:"serial_no,omitempty"`
}

type OntInterface struct {
	OntIdx         string `json:"ont_idx"`
	EqptVerNum     string `json:"eqpt_ver_num"`
	SwVerAct       string `json:"sw_ver_act"`
	ActualNumSlots string `json:"actual_num_slots"`
	VersionNumber  string `json:"version_number"`
	SerNum         string `json:"sernum"`
	YpSerialNo     string `json:"yp_serial_no"`
	CfgFile1VerAct string `json:"cfgfile1_ver_act"`
	CfgFile2VerAct string `json:"cfgfile2_ver_act"`
}

type EquipIDCount struct {
	ID       string `json:"equip_id"`
	Vendor   string `json:"vendor,omitempty"`
	Count    int    `json:"count"`
	SwVerAct string `json:"sw_ver_act,omitempty"`
	SerialNo string `json:"serial_no,omitempty"`
}

// vendorByModel maps equip-id (ONT model) to display vendor name for richer vendor breakdown.
// Used when OLT returns generic vendor-id; restores Nokia ONT, Nokia ONU, ORFA, ALCL, HWTC, BDCM, etc.
var vendorByModel = map[string]string{
	"G1425GE": "Nokia ONT", "G1425GB": "Nokia ONT", "XS-010X-Q": "Nokia ONT", "I240WA": "Nokia ONT",
	"G-010G-R": "Nokia ONU",
	"OG-976V":  "ORFA", "OG-976V2": "ORFA",
	"NEXXT G8421H": "HWTC", "HG8145V6": "HWTC", "EG8145V5": "HWTC", "EG8021V5": "HWTC",
	"GP1702-1Gv2": "HWTC", "HG8120C": "HWTC", "HG8145C": "HWTC", "HG8245C": "HWTC",
	"HG8245Q2": "HWTC", "HG8321V": "HWTC", "HG8346M": "HWTC", "HG8340M": "HWTC",
	"HS8125C": "HWTC", "HS8145C5": "HWTC", "HS8546V": "HWTC", "EG8041X6-10": "HWTC",
	"K662D": "HWTC", "JZ8600": "HWTC", "SA1456C": "HWTC", "OG-978VX": "HWTC", "540M": "HWTC",
	"EG8141A5": "HWTC", "HG8245": "HWTC", "HG8342R": "HWTC", "HG8546M": "HWTC",
	"OG-92SR": "HWTC", "5850ON": "HWTC", "HG8245A": "HWTC", "HG8310M": "HWTC",
	"HG8546M-RMS": "HWTC", "HS8145V5": "HWTC", "HG8020C": "HWTC", "HG8240": "HWTC",
	"HG8321R": "HWTC", "HG8540M": "HWTC", "HS8545M": "HWTC", "EG8120L": "HWTC",
	"HG8240R": "HWTC", "HG8245H": "HWTC", "HG8541M": "HWTC", "HS8145C": "HWTC",
	"P4021A": "HWTC", "HG8346R": "HWTC", "HS8545M5": "HWTC",
	"GP1702-1G": "BDCM", "GP1704-2F-E": "BDCM", "GP1705-2G": "BDCM", "GP1706-4G": "BDCM", "GP1706-4GV": "BDCM",
	"I-240W-A": "ALCL", "G-240W-A": "ALCL", "I-241W-A": "ALCL", "G-241W-A": "ALCL", "I-240G-Q": "ALCL", "G-010G-Q": "ALCL",
}

func extractField(re *regexp.Regexp, block string) string {
	if m := re.FindStringSubmatch(block); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// normalizeOntIdx converts ont_idx to slash format to match power_readings (optics table).
// Power readings use "1/2/3" format; Nokia inventory may use "1-2-3".
func normalizeOntIdx(ontIdx string) string {
	if ontIdx == "" {
		return ""
	}
	return strings.ReplaceAll(ontIdx, "-", "/")
}

func ExtractAllEquipID(output string) []EquipID {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")

	var results []EquipID
	blocks := blockSplit.Split(output, -1)

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		id := extractField(reEquipID, block)
		if id == "" {
			continue
		}
		id = strings.Trim(id, `"`)
		id = strings.TrimSpace(id)
		id = strings.TrimPrefix(id, "NOCLEICODE")
		id = strings.Trim(id, "_")

		idLower := strings.ToLower(id)
		if id == "" || id == "equip_id" || strings.Contains(idLower, "actual-num-slots") {
			continue
		}
		ontIdx := extractField(reOntID, block)
		if ontIdx == "" {
			ontIdx = extractField(reOntIDAlt, block)
		}
		if ontIdx == "" {
			ontIdx = extractField(reOntIDLoose, block)
		}
		if ontIdx == "" {
			ontIdx = extractField(reOntIDHyphen, block)
		}
		ontIdx = normalizeOntIdx(ontIdx)
		results = append(results, EquipID{
			ID:       id,
			OntIdx:   ontIdx,
			Vendor:   extractField(reVendorID, block),
			SwVerAct: extractField(reSwVerAct, block),
			SerialNo: extractField(reYpSerial, block),
		})
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
	first := make(map[string]*EquipID)
	var order []string

	for i := range all {
		e := &all[i]
		if counts[e.ID] == 0 {
			order = append(order, e.ID)
			first[e.ID] = e
		}
		counts[e.ID]++
	}
	results := make([]EquipIDCount, 0, len(order))
	for _, id := range order {
		r := EquipIDCount{ID: id, Count: counts[id]}
		if e := first[id]; e != nil {
			r.Vendor = e.Vendor
			r.SwVerAct = e.SwVerAct
			r.SerialNo = e.SerialNo
		}
		results = append(results, r)
	}
	return results
}

// VendorOrUnknown returns the vendor from EquipIDCount, or "Unknown" if empty (fallback for output without vendor-id).
func (c EquipIDCount) VendorOrUnknown() string {
	if c.Vendor != "" {
		return c.Vendor
	}
	return "Unknown"
}

// VendorDisplay returns the display vendor for inventory counts.
// Uses model-based map when available (Nokia ONT, Nokia ONU, ORFA, ALCL, HWTC, etc.); otherwise raw vendor-id from OLT output.
func (c EquipIDCount) VendorDisplay() string {
	if v := vendorByModel[c.ID]; v != "" {
		return v
	}
	if v := vendorByModel[strings.ToUpper(c.ID)]; v != "" {
		return v
	}
	if c.Vendor != "" {
		return c.Vendor
	}
	return "Unknown"
}

// SwVerCount holds software version (sw-ver-act) aggregate.
type SwVerCount struct {
	SwVerAct string `json:"sw_ver_act"`
	Count    int    `json:"count"`
	Vendor   string `json:"vendor,omitempty"`
}

// CountBySwVerAct aggregates ONTs by sw-ver-act (software version).
func CountBySwVerAct(output string) []SwVerCount {
	all := ExtractAllEquipID(output)
	counts := make(map[string]int)
	first := make(map[string]*EquipID)
	var order []string

	for i := range all {
		e := &all[i]
		ver := strings.TrimSpace(e.SwVerAct)
		if ver == "" {
			ver = "unknown"
		}
		if counts[ver] == 0 {
			order = append(order, ver)
			first[ver] = e
		}
		counts[ver]++
	}
	results := make([]SwVerCount, 0, len(order))
	for _, ver := range order {
		r := SwVerCount{SwVerAct: ver, Count: counts[ver]}
		if e := first[ver]; e != nil && e.Vendor != "" {
			r.Vendor = e.Vendor
		}
		results = append(results, r)
	}
	return results
}

// ExtractPerOntInventory returns per-ONT equip-id and serial for linking to power readings.
// Uses ont-id from block when present; otherwise empty OntIdx (caller may infer from block order).
func ExtractPerOntInventory(output string) []OntInventoryItem {
	all := ExtractAllEquipID(output)
	out := make([]OntInventoryItem, 0, len(all))
	for _, e := range all {
		out = append(out, OntInventoryItem{
			OntIdx:   e.OntIdx,
			EquipID:  e.ID,
			SerialNo: e.SerialNo,
		})
	}
	return out
}

func ExtractOntInterfaces(output string) []OntInterface {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")

	var out []OntInterface
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.Contains(line, "|") || strings.HasPrefix(line, "=") || strings.HasPrefix(line, "-") {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "interface table") || strings.HasPrefix(lower, "typ:") || strings.HasPrefix(lower, "ont-idx") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 7 || !looksLikeOntIdx(fields[0]) {
			continue
		}
		row := OntInterface{
			OntIdx:         normalizeOntIdx(fields[0]),
			EqptVerNum:     fields[1],
			SwVerAct:       fields[2],
			ActualNumSlots: fields[3],
			VersionNumber:  fields[4],
			SerNum:         fields[5],
			YpSerialNo:     fields[6],
		}
		if len(fields) > 7 {
			row.CfgFile1VerAct = fields[7]
		}
		if len(fields) > 8 {
			row.CfgFile2VerAct = fields[8]
		}
		out = append(out, row)
	}
	return out
}

func looksLikeOntIdx(s string) bool {
	parts := strings.Split(s, "/")
	if len(parts) < 3 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

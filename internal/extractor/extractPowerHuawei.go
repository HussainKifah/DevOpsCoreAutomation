package extractor

import (
	"strconv"
	"strings"
)

// HuaweiOntOptical holds optical power info from "display ont optical-info" output.
type HuaweiOntOptical struct {
	Slot          int
	Pon           int
	Ont           int
	RxPower       float64
	TxPower       float64
	OltRxOntPower float64
	Vendor        string // From "Vendor PN" in raw output
}

// ExtractAllHuaweiOntOptical parses the "display ont optical-info" output and returns
// power values. Blocks are matched to indices in order (slot, pon, ont).
func ExtractAllHuaweiOntOptical(output string, indices []struct{ Slot, Pon, Ont int }) []HuaweiOntOptical {
	blocks := splitHuaweiOpticalBlocks(output)
	var out []HuaweiOntOptical
	idx := 0
	for _, block := range blocks {
		if idx >= len(indices) {
			break
		}
		rx, tx, oltRx, vendor, ok := parseHuaweiOpticalBlock(block)
		if !ok {
			continue
		}
		i := indices[idx]
		idx++
		out = append(out, HuaweiOntOptical{
			Slot:          i.Slot,
			Pon:           i.Pon,
			Ont:           i.Ont,
			RxPower:       rx,
			TxPower:       tx,
			OltRxOntPower: oltRx,
			Vendor:        vendor,
		})
	}
	// Fallback: if no blocks found (e.g. different separator or format), scan whole output
	if len(out) == 0 {
		out = extractPowersFromWholeOutput(output, indices)
	}
	return out
}

// extractPowersFromWholeOutput scans the entire output for power lines when block splitting fails.
// Handles formats without clear "------------" separators.
func extractPowersFromWholeOutput(output string, indices []struct{ Slot, Pon, Ont int }) []HuaweiOntOptical {
	var out []HuaweiOntOptical
	lines := strings.Split(output, "\n")
	var rx, tx, oltRx float64
	var vendor string
	var haveRx, haveTx, haveOltRx bool
	idx := 0

	flush := func() {
		if idx < len(indices) && (haveRx || haveTx || haveOltRx) {
			i := indices[idx]
			idx++
			out = append(out, HuaweiOntOptical{
				Slot:          i.Slot,
				Pon:           i.Pon,
				Ont:           i.Ont,
				RxPower:       rx,
				TxPower:       tx,
				OltRxOntPower: oltRx,
				Vendor:        vendor,
			})
			rx, tx, haveRx, haveTx, haveOltRx, vendor = 0, 0, false, false, false, ""
		}
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(strings.TrimPrefix(line[colonIdx:], ":"))
		if val == "" || val == "-" {
			continue
		}

		// Vendor PN from raw output (e.g. "Vendor PN" : "LTY9775M-CHG+" or "-")
		if strings.Contains(strings.ToLower(key), "vendor") && strings.Contains(strings.ToLower(key), "pn") {
			if val != "" && val != "-" {
				vendor = val
			}
			continue
		}

		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			continue
		}

		// Match various Huawei output key names (exclude CATV Rx/Tx which overwrite with 0.00)
		if strings.Contains(key, "Rx optical power") && !strings.Contains(key, "OLT") && !strings.Contains(key, "CATV") {
			flush()
			rx, haveRx = v, true
			tx, haveTx, haveOltRx = 0, false, false
		} else if strings.Contains(key, "OLT Rx") && strings.Contains(key, "optical power") {
			oltRx, haveOltRx = v, true
			flush()
			rx, tx, haveRx, haveTx, haveOltRx = 0, 0, false, false, false
		} else if strings.Contains(key, "Tx optical power") && !strings.Contains(key, "CATV") {
			tx, haveTx = v, true
		} else if strings.Contains(key, "Rx power") && !strings.Contains(key, "OLT") && !strings.Contains(key, "CATV") && !strings.Contains(key, "threshold") {
			rx, haveRx = v, true
		} else if strings.Contains(key, "Tx power") && !strings.Contains(key, "CATV") && !strings.Contains(key, "threshold") {
			tx, haveTx = v, true
		}
	}
	flush()
	return out
}

func splitHuaweiOpticalBlocks(output string) []string {
	// Blocks are separated by lines of dashes (e.g. "------------" or "  -----------------------------------------------------------------------------")
	sep := "------------"
	var blocks []string
	start := 0
	for {
		i := strings.Index(output[start:], sep)
		if i == -1 {
			break
		}
		blockStart := start + i
		rest := output[blockStart+len(sep):]
		j := strings.Index(rest, sep)
		if j == -1 {
			blocks = append(blocks, output[blockStart:])
			break
		}
		blocks = append(blocks, output[blockStart:blockStart+len(sep)+j])
		start = blockStart + len(sep) + j
	}
	return blocks
}

// parseHuaweiOpticalBlock extracts Rx, Tx, OLT Rx ONT, and Vendor PN from a block. Returns ok=false if not valid.
// Raw format: "Rx optical power(dBm)", "Tx optical power(dBm)", "Vendor PN" (exclude CATV Rx which overwrites with 0.00).
func parseHuaweiOpticalBlock(block string) (rx, tx, oltRx float64, vendor string, ok bool) {
	// Look for: "Rx optical power(dBm)" : -24.31  (exclude "CATV Rx optical power" which is 0.00 and overwrites)
	//           "Tx optical power(dBm)" : 2.00
	//           "OLT Rx ONT optical power(dBm)" : -26.63
	//           "Vendor PN" : LTY9775M-CHG+ (or "-" when unknown)
	var rxOk, txOk, oltOk bool
	lines := strings.Split(block, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		colonIdx := strings.Index(line, ":")
		if colonIdx == -1 {
			continue
		}
		key := strings.TrimSpace(line[:colonIdx])
		val := strings.TrimSpace(strings.TrimPrefix(line[colonIdx:], ":"))

		// Vendor PN from raw output (can be "-" when unknown)
		if strings.Contains(strings.ToLower(key), "vendor") && strings.Contains(strings.ToLower(key), "pn") {
			if val != "" {
				vendor = val
			}
			continue
		}

		if val == "" || val == "-" {
			continue
		}
		v, err := strconv.ParseFloat(val, 64)
		if err != nil {
			continue
		}
		switch {
		case (strings.Contains(key, "Rx optical power") || strings.Contains(key, "Rx power")) && !strings.Contains(key, "OLT") && !strings.Contains(key, "CATV"):
			rx, rxOk = v, true
		case (strings.Contains(key, "Tx optical power") || strings.Contains(key, "Tx power")) && !strings.Contains(key, "CATV"):
			tx, txOk = v, true
		case strings.Contains(key, "OLT Rx ONT") || strings.Contains(key, "OLT Rx") || strings.Contains(key, "OLT optical"):
			oltRx, oltOk = v, true
		}
	}
	ok = rxOk || txOk || oltOk
	return rx, tx, oltRx, vendor, ok
}

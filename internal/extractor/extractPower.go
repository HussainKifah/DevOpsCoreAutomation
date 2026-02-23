package extractor

import (
	"bufio"
	"strconv"
	"strings"
)

type OntPower struct {
	OntIdx string
	OltRx  float64
}

func ExtractOntIdxBelowOltRx(output string, threshold float64) []string {

	onts := ExtractOntPowerBelowOltRx(output, threshold)

	out := make([]string, 0, len(onts))
	for _, o := range onts {
		out = append(out, o.OntIdx)
	}
	return out

}

func ExtractAllOntPower(output string) []OntPower {
	table, ok := extractOpticsTableSection(output)
	if !ok {
		return nil
	}

	var out []OntPower

	sc := bufio.NewScanner(strings.NewReader(table))
	sc.Buffer(make([]byte, 1024), 1024*1024)

	inRows := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())

		if strings.HasPrefix(line, "--------------+") {
			inRows = true
			continue
		}
		if !inRows {
			continue
		}

		if strings.HasPrefix(line, "optics count") || strings.HasPrefix(line, "====") {
			break
		}
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ontIdx := fields[0]
		oltRxStr := fields[1]

		oltRx, ok := parseFloat(oltRxStr)
		if !ok {
			continue
		}
		out = append(out, OntPower{OntIdx: ontIdx, OltRx: oltRx})
	}
	return out
}

func ExtractOntPowerBelowOltRx(output string, threshold float64) []OntPower {
	all := ExtractAllOntPower(output)
	var out []OntPower
	for _, p := range all {
		if p.OltRx < threshold {
			out = append(out, p)
		}
	}
	return out
}

func parseFloat(s string) (float64, bool) {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func extractOpticsTableSection(out string) (string, bool) {

	start := strings.Index(out, "optics table")
	if start == -1 {
		return "", false
	}

	if s2 := strings.LastIndex(out[:start], "========================================================================================"); s2 != -1 {
		start = s2
	}

	endRel := strings.Index(out[start:], "optics count")
	if endRel == -1 {
		return "", false
	}
	end := start + endRel

	// include the "optics count" line
	if nl := strings.Index(out[end:], "\n"); nl != -1 {
		end = end + nl + 1
	}

	return out[start:end], true
}

package extractor

import (
	"bufio"
	"regexp"
	"strings"
)

var ontIdxRe = regexp.MustCompile(`^\d+(?:/\d+)+$`)

func ExtractOntIdxFromTable (tableText string) []string {
	var ontIdxs []string

	sc := bufio.NewScanner(strings.NewReader(tableText))
	inRows := false

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line,"--------------+") {
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
		if len(fields) == 0 {
			continue
		}
		if ontIdxRe.MatchString(fields[0]) {
			ontIdxs = append(ontIdxs, fields[0])
		}
	}

	return ontIdxs
}
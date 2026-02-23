package extractor

import (
	"regexp"
	"strconv"
	"strings"
)

type OntDesc struct {
	OntIdx string `json:"ont_idx"`
	Desc1  string `json:"desc1"`
	Desc2  string `json:"desc2"`
}

var re = regexp.MustCompile(
	`(?m)^\s*\S+\s+(\S+)\s+\S+\s+\S+\s+\S+\s+(-?\d+(?:\.\d+)?)\s+\S+\s+(\S+)\s+(.*)$`,
)

func ExtractAllDesc(output string) []OntDesc {

	output = strings.ToValidUTF8(output, "")
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")

	matches := re.FindAllStringSubmatch(output, -1)
	if matches == nil {
		return nil
	}

	results := make([]OntDesc, 0, len(matches))
	for _, m := range matches {


		desc1 := strings.Trim(m[3], `"`)
		desc1 = strings.NewReplacer("\t", "", "\n", "").Replace(desc1)
		desc1 = strings.TrimSpace(desc1)

		desc2 := strings.TrimSpace(strings.Trim(m[4], `"`))
		desc2 = strings.TrimSuffix(desc2, "undefined")
		desc2 = strings.TrimSpace(desc2)
		desc2 = strings.NewReplacer("\t", "", "\n", "", "\ufffd", "", "*", "").Replace(desc2)
		desc2 = strings.TrimRight(desc2, `" `)
		desc2 = strings.TrimSpace(desc2)

		if desc1 == "" {
			parts := regexp.MustCompile(`\s{2,}`).Split(desc2, 2)
			if len(parts) == 2 {
				desc1 = strings.Trim(parts[0], `"`)
				desc2 = strings.Trim(parts[1], `"`)
			}
		}

		results = append(results, OntDesc{
			OntIdx: m[1],
			Desc1:  strings.ToValidUTF8(desc1, ""),
			Desc2:  strings.ToValidUTF8(desc2, ""),
		})
	}
	return results
}

func ExtractDesc (line string) (rx float64, desc1, desc2 string, ok bool){
	line = strings.ReplaceAll(line, "\r\n", "\n")
	line = strings.ReplaceAll(line, "\r", "\n")

	m := re.FindStringSubmatch(line)
	if m == nil {
		return 0,"", "", false
	}

	rx, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, "", "", false
	}

	desc1 = strings.Trim(m[3], `"`)
	desc1 = strings.NewReplacer("\t", "", "\n", "").Replace(desc1)
	desc1 = strings.TrimSpace(desc1)

	desc2 = strings.TrimSpace(strings.Trim(m[4], `"`))
	desc2 = strings.TrimSuffix(desc2, "undefined")
	desc2 = strings.TrimSpace(desc2)
	desc2 = strings.TrimRight(desc2, `"`)
	desc2 = strings.NewReplacer("\t", "", "\n", "", "\ufffd", "", "*", "").Replace(desc2)

	if desc1 == "" {
	parts := regexp.MustCompile(`\s{2,}`).Split(desc2, 2)
	if len(parts) == 2 {
		desc1 = strings.Trim(parts[0], `"`)
		desc2 = strings.Trim(parts[1], `"`)
	}
}
	return rx, desc1, desc2, true
}
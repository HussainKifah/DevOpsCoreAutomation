package extractor

import (
	"regexp"
	"strconv"
)

type PortProtection struct {
	Port		string	`json:"port"`
	PortState	string	`json:"port_state"`
	PairedState string  `json:"paired_state"`
	SwoReason	string  `json:"swo_reason"`
	NumSwo		int     `json:"num_swo"`
	alert     	string  `json:"alert"`
}

var rePort = regexp.MustCompile(
	`(?m)^(pon:\S+)\s+\S+\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d+)\s*$`,)


func ExtractPortProtection (output string) []PortProtection {
	matches := rePort.FindAllStringSubmatch(output, -1)

	if matches == nil {
		return nil
	}

	results := make([]PortProtection, 0, len(matches))
	for _,m := range matches {
		n,_ := strconv.Atoi(m[5])
		results = append(results, PortProtection{
			Port: m[1],
			PortState: m[2],
			PairedState: m[3],
			SwoReason: m[4],
			NumSwo: n,
		})
	}
	return results
}
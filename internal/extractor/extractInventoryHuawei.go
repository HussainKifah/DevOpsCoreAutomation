package extractor

import (
	"regexp"
	"strings"
)

// HuaweiOntVersion holds one row from "display ont version 0 all".
type HuaweiOntVersion struct {
	Index      string `json:"index"`       // F/S/P/ONT-ID e.g. "0/0/0/0"
	VendorID   string `json:"vendor_id"`   // e.g. "HWTC"
	OntModel   string `json:"ont_model"`   // e.g. "OG-976V2"
	SwVersion  string `json:"sw_version"`  // e.g. "V3.1.0-160815"
}

// Matches rows like:  "  0/ 0/ 0/  0   HWTC    OG-976V2                  V3.1.0-160815     -"
// Index has spaces: "0/ 0/ 0/  0". Capture index, vendor, model, version; trailing OUI column optional.
var reHwOntVersion = regexp.MustCompile(
	`(?m)^\s*(\d+\s*/\s*\d+\s*/\s*\d+\s*/\s*\d+)\s+(\S+)\s+(\S+)\s+(\S+)`)

func ExtractHuaweiOntVersions(output string) []HuaweiOntVersion {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	output = strings.ReplaceAll(output, "\r", "\n")
	output = stripANSICodes(output)
	matches := reHwOntVersion.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}

	out := make([]HuaweiOntVersion, 0, len(matches))
	for _, m := range matches {
		idx := strings.ReplaceAll(m[1], " ", "")
		out = append(out, HuaweiOntVersion{
			Index:     idx,
			VendorID:  m[2],
			OntModel:  m[3],
			SwVersion: m[4],
		})
	}
	return out
}

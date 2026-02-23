package excesscommands

import (
	"fmt"
	"log"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

type PortResult struct {
	Device string                     `json:"device"`
	Site   string                     `json:"site"`
	Host   string                     `json:"host"`
	Data   []extractor.PortProtection `json:"data"`
	Err    error                      `json:"error"`
}

func PortProtection(username, password string) []PortResult {

	cmd := "show port-protection"
	var output []PortResult

	for olt := range shell.SendCommandNokiaOLTs(username, password, cmd) {
		if olt.Err != nil {
			output = append(output, PortResult{Host: olt.Host, Err: olt.Err})
			continue
		}

		data := extractor.ExtractPortProtection(olt.Data)

		var filtered []extractor.PortProtection
		for _, d := range data {
			if strings.Contains(d.PortState, "down") || strings.Contains(d.PairedState, "down") {
				filtered = append(filtered, d)
			}
		}
		if len(filtered) > 0 {
			output = append(output, PortResult{Device: olt.Device, Site: olt.Site, Host: olt.Host, Data: filtered})
		}
	}
	path, err := utils.SaveJSON("json", "PortProtection", output)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("saved:", path)

	return output

}

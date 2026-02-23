package excesscommands

import (
	"fmt"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

// (string, error)
func CpuUpTime(username, password string) {

	cmds := []string{"show system cpu-load detail",
		"show core1-uptime",
		"show equipment temperature",
	}

	type HostHealth struct {
		Host   string           `json:host`
		Health extractor.Health `json:health`
	}
	var results []HostHealth
	for output := range shell.SendCommandNokiaOLTs(username, password, cmds...) {
		if output.Err != nil {
			fmt.Printf("ERROR %s: %v\n", output.Host, output.Err)
		}
		h := extractor.ExtractHealth(output.Data)
		results = append(results, HostHealth{Host: output.Host, Health: h})
	}
	utils.SaveJSON("json", "olt-health", results)
}

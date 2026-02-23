package excesscommands

import (
	"fmt"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

type PowersResult struct {
	Device string
	Site   string
	Host   string
	Data   []extractor.OntPower
	Err    error
}

func PowersLessThan24(user, password string, allPower bool) ([]PowersResult, error) {
	cmd := "show equipment ont optics"
	out := make([]PowersResult, 0)
	for r := range shell.SendCommandNokiaOLTs(user, password, cmd) {
		if r.Err != nil {
			out = append(out, PowersResult{Device: r.Device, Site: r.Site, Host: r.Host, Err: r.Err})
			continue
		}
		if allPower == true {
			out = append(out, PowersResult{
				Device: r.Device,
				Site:   r.Site,
				Host:   r.Host,
				Data:   extractor.ExtractOntPowerBelowOltRx(r.Data, -24.0),
				Err:    nil,
			})
		} else {
			out = append(out, PowersResult{
				Device: r.Device,
				Site:   r.Site,
				Host:   r.Host,
				Data:   extractor.ExtractOntPowerBelowOltRx(r.Data, -24.0),
				Err:    nil,
			})
		}

		fmt.Println(out)
	}
	path, err := utils.SaveJSON("json", "all-onts-powers-less-than-24", out)
	if err != nil {
		return out, err
	}
	_ = path
	return out, nil

}

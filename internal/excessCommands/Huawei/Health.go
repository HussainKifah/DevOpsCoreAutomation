package huawei

import (
	"fmt"
	"log"
	"sync"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

type HuaweiHealthResult struct {
	Device string                `json:"device"`
	Site   string                `json:"site"`
	Host   string                `json:"host"`
	Health extractor.HuaweiHealth `json:"health"`
	Err    string                `json:"error,omitempty"`
}

func CollectHealth(user, pass string) []HuaweiHealthResult {
	olts := shell.GetHuaweiOLTs()
	if len(olts) == 0 {
		return nil
	}

	results := make([]HuaweiHealthResult, len(olts))
	var wg sync.WaitGroup

	for i, olt := range olts {
		i, olt := i, olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = collectHealthForOLT(olt.Ip, user, pass, olt.Name, olt.Site)
		}()
	}
	wg.Wait()

	path, err := utils.SaveJSON("json", "HuaweiHealth", results)
	if err != nil {
		log.Printf("save health JSON: %v", err)
	} else {
		fmt.Println("saved:", path)
	}

	return results
}

func collectHealthForOLT(host, user, pass, device, site string) HuaweiHealthResult {
	res := HuaweiHealthResult{Device: device, Site: site, Host: host}

	sess, err := shell.HwOpenSession(host, user, pass)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer sess.Close()

	if _, err := sess.SendCommands("enable", "scroll 512"); err != nil {
		res.Err = fmt.Sprintf("setup: %v", err)
		return res
	}

	// Uptime
	uptimeOut, err := sess.SendCommands("display sysuptime")
	if err != nil {
		log.Printf("[%s] health uptime: %v", host, err)
	}
	res.Health.Uptime = extractor.ExtractHuaweiUptime(uptimeOut)

	// Temperature and CPU: loop over slots 0-7
	startSlot := SlotForOLT(host)
	for slot := startSlot; slot < maxSlots; slot++ {
		slotStr := fmt.Sprintf("0/%d", slot)

		tempOut, err := sess.SendCommands(fmt.Sprintf("display temperature %s", slotStr))
		if err != nil {
			log.Printf("[%s] health temp slot %s: %v", host, slotStr, err)
			continue
		}
		if t := extractor.ExtractHuaweiTemperature(slotStr, tempOut); t != nil {
			res.Health.Temperatures = append(res.Health.Temperatures, *t)
		}

		cpuOut, err := sess.SendCommands(fmt.Sprintf("display cpu %s", slotStr))
		if err != nil {
			log.Printf("[%s] health cpu slot %s: %v", host, slotStr, err)
			continue
		}
		if c := extractor.ExtractHuaweiCpu(slotStr, cpuOut); c != nil {
			res.Health.CpuLoads = append(res.Health.CpuLoads, *c)
		}
	}

	return res
}

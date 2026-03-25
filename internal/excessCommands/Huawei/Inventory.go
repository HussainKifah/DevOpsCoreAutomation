package huawei

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

type HuaweiInventoryResult struct {
	Device string                       `json:"device"`
	Site   string                       `json:"site"`
	Host   string                       `json:"host"`
	ONTs   []extractor.HuaweiOntVersion `json:"onts"`
	Total  int                          `json:"total"`
	Err    string                       `json:"error,omitempty"`
}

func CollectInventory(user, pass string) []HuaweiInventoryResult {
	olts := shell.GetHuaweiOLTs()
	if len(olts) == 0 {
		return nil
	}

	results := make([]HuaweiInventoryResult, len(olts))
	var wg sync.WaitGroup

	for i, olt := range olts {
		i, olt := i, olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = collectInventoryForOLT(olt.Ip, user, pass, olt.Name, olt.Site)
		}()
	}
	wg.Wait()

	path, err := utils.SaveJSON("json", "HuaweiInventory", results)
	if err != nil {
		log.Printf("save inventory JSON: %v", err)
	} else {
		fmt.Println("saved:", path)
	}

	return results
}

func collectInventoryForOLT(host, user, pass, device, site string) HuaweiInventoryResult {
	res := HuaweiInventoryResult{Device: device, Site: site, Host: host}

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

	raw, err := sess.SendCommands("display ont version 0 all")
	if err != nil {
		res.Err = fmt.Sprintf("display ont version: %v", err)
	}

	// Always try extraction, even on partial error (SendCommands returns partial data + error)
	res.ONTs = extractor.ExtractHuaweiOntVersions(raw)
	res.Total = len(res.ONTs)

	if res.Total == 0 && len(raw) > 0 {
		dir := filepath.Join("logs", "huawei")
		_ = os.MkdirAll(dir, 0o755)
		name := fmt.Sprintf("debug_inventory_%s_%s.txt", sanitizeHost(host), time.Now().Format("20060102_150405"))
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
			log.Printf("[%s] could not write debug file %s: %v", host, p, err)
		} else {
			log.Printf("[%s] debug: saved raw output (%d bytes) to %s", host, len(raw), p)
		}
	}
	return res
}

func sanitizeHost(host string) string {
	s := host
	for _, c := range []string{".", ":", "/", "\\"} {
		s = strings.ReplaceAll(s, c, "_")
	}
	return s
}

package huawei

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"github.com/Flafl/DevOpsCore/utils"
)

type ProtectGroupResult struct {
	Device string                         `json:"device"`
	Site   string                         `json:"site"`
	Host   string                         `json:"host"`
	Groups []extractor.HuaweiProtectGroup `json:"groups"`
	Err    string                         `json:"error,omitempty"`
}

func CollectProtectGroups(user, pass string) []ProtectGroupResult {
	olts := shell.GetHuaweiOLTs()
	if len(olts) == 0 {
		return nil
	}

	results := make([]ProtectGroupResult, len(olts))
	var wg sync.WaitGroup

	for i, olt := range olts {
		i, olt := i, olt
		wg.Add(1)
		go func() {
			defer wg.Done()

			raw, err := shell.HwSendCommandOLT(olt.Ip, user, pass, "enable", "scroll 512", "display protect-group")
			r := ProtectGroupResult{Device: olt.Name, Site: olt.Site, Host: olt.Ip}
			if err != nil {
				r.Err = err.Error()
				log.Printf("[%s] protect-group: %v", olt.Ip, err)
			}
			r.Groups = extractor.ExtractHuaweiProtectGroups(raw)
			results[i] = r

			if len(r.Groups) == 0 && len(raw) > 0 {
				saveRawForDebug(olt.Ip, raw)
			}
		}()
	}
	wg.Wait()

	path, err := utils.SaveJSON("json", "HuaweiProtectGroups", results)
	if err != nil {
		log.Printf("save protect-groups JSON: %v", err)
	} else {
		fmt.Println("saved:", path)
	}

	return results
}

// saveRawForDebug writes raw SSH output to logs when parsing fails (for debugging).
func saveRawForDebug(host, raw string) {
	dir := filepath.Join("logs", "huawei")
	_ = os.MkdirAll(dir, 0o755)
	safe := strings.NewReplacer(".", "_", ":", "_").Replace(host)
	path := filepath.Join(dir, "protect_debug_"+safe+".txt")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		log.Printf("[%s] could not save raw debug: %v", host, err)
	} else {
		log.Printf("[%s] raw output saved for debug: %s (parsing returned no groups)", host, path)
	}
}

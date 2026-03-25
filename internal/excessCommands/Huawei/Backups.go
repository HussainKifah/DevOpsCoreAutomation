package huawei

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/shell"
)

type BackupResult struct {
	Device   string `json:"device"`
	Site     string `json:"site"`
	Host     string `json:"host"`
	FilePath string `json:"file_path"`
	Err      string `json:"error,omitempty"`
}

func Backups(user, pass string) []BackupResult {
	olts := shell.GetHuaweiOLTs()
	if len(olts) == 0 {
		return nil
	}

	results := make([]BackupResult, len(olts))
	var wg sync.WaitGroup
	for i, olt := range olts {
		i, olt := i, olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = backupOLT(olt, user, pass)
		}()
	}
	wg.Wait()
	return results
}

func backupOLT(olt shell.OLT, user, pass string) BackupResult {
	res := BackupResult{Device: olt.Name, Site: olt.Site, Host: olt.Ip}

	raw, err := shell.HwSendCommandOLT(olt.Ip, user, pass, "enable", "scroll 512", "display current-configuration")
	if err != nil {
		res.Err = err.Error()
		log.Printf("[%s] backup: %v", olt.Ip, err)
	}
	if len(strings.TrimSpace(raw)) == 0 {
		if res.Err == "" {
			res.Err = "no data received"
		}
		log.Printf("[%s] backup: no data received", olt.Ip)
		return res
	}

	site := strings.ReplaceAll(olt.Site, "/", "-")
	if site == "" {
		site = "unknown"
	}
	res.Site = site
	folder := filepath.Join("backups", site, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		res.Err = fmt.Sprintf("mkdir: %v", err)
		log.Printf("[%s] backup mkdir: %v", olt.Ip, err)
		return res
	}

	name := strings.ReplaceAll(olt.Name, "/", "-")
	filename := fmt.Sprintf("%s_%s.txt", name, olt.Ip)
	path := filepath.Join(folder, filename)

	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		res.Err = fmt.Sprintf("write: %v", err)
		log.Printf("[%s] backup write: %v", olt.Ip, err)
		return res
	}
	res.FilePath = path
	fmt.Printf("saved: %s\n", path)
	return res
}

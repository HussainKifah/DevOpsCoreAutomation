package excesscommands

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

type backupStep struct {
	label string
	cmds  []string
}

var backupSteps = []backupStep{
	{label: "equipment ont", cmds: []string{"info configure equipment ont interface flat no detail"}},
	{label: "interface port", cmds: []string{"info configure interface port flat no detail"}},
	{label: "qos interface", cmds: []string{"info configure qos interface flat no detail"}},
	{label: "bridge", cmds: []string{"info configure bridge flat no detail"}},
	{label: "service config", cmds: []string{"info configure service flat no detail"}},
	{label: "admin display config", cmds: []string{"admin display-config"}},
}

const backupCmdPause = 2 * time.Minute

// backupParallelOLTs limits concurrent OLT backup sessions (15 at a time to reduce SSH load).
const backupParallelOLTs = 15

type BackupResult struct {
	Device   string
	Site     string
	Host     string
	Data     string
	FilePath string // set when the backup file was written successfully
	Err      error
}

// CollectBackup runs the Nokia backup command sequence for one OLT (new SSH session per step).
// If verbose is true, raw CLI output per step is written to w. The on-disk file is written once
// after all steps succeed, using extractor.FinalizeNokiaMultistepBackup.
func CollectBackup(host, user, pass, device, site string, verbose bool, w io.Writer) BackupResult {
	res := BackupResult{Device: device, Site: site, Host: host}
	filePath := backupFilePath(device, site, host)

	var rawSteps []string
	for i, step := range backupSteps {
		if i > 0 {
			log.Printf("[backup] %s: waiting %s before next step", host, backupCmdPause)
			time.Sleep(backupCmdPause)
		}

		log.Printf("[backup] %s: step %d/%d [%s]: %v", host, i+1, len(backupSteps), step.label, step.cmds)
		out, err := shell.NkSendCommandOLT(host, user, pass, step.cmds...)
		if err != nil {
			log.Printf("[backup] %s: step %d [%s] failed: %v", host, i+1, step.label, err)
			res.Err = fmt.Errorf("step %d (%s): %w", i+1, step.label, err)
			if len(rawSteps) > 0 {
				break
			}
			return res
		}
		log.Printf("[backup] %s: step %d [%s] done (%d bytes)", host, i+1, step.label, len(out))
		if verbose && w != nil {
			_, _ = fmt.Fprintf(w, "\n========== %s | step %d/%d [%s] — %d bytes ==========\n%s\n========== end step ==========\n",
				host, i+1, len(backupSteps), step.label, len(out), out)
		}
		rawSteps = append(rawSteps, out)
	}

	res.Data = extractor.FinalizeNokiaMultistepBackup(rawSteps)
	if res.Err != nil {
		return res
	}

	if err := os.WriteFile(filePath, []byte(res.Data), 0o644); err != nil {
		log.Printf("[backup] write %s: %v", filePath, err)
	} else {
		res.FilePath = filePath
		log.Printf("[backup] %s: wrote %s (%d bytes)", host, filePath, len(res.Data))
	}
	return res
}

// CollectBackupPooled runs all backup steps on one SSH session per OLT. Transient errors use pool retry/backoff.
// The file is written once after full success; content is cleaned via extractor.FinalizeNokiaMultistepBackup.
func CollectBackupPooled(pool *shell.ConnectionPool, host, device, site string, verbose bool, w io.Writer) BackupResult {
	res := BackupResult{Device: device, Site: site, Host: host}
	filePath := backupFilePath(device, site, host)

	var rawSteps []string
	for i, step := range backupSteps {
		if i > 0 {
			log.Printf("[backup] %s: waiting %s before next step", host, backupCmdPause)
			time.Sleep(backupCmdPause)
		}

		log.Printf("[backup] %s: step %d/%d [%s]: %v", host, i+1, len(backupSteps), step.label, step.cmds)
		out, err := pool.SendCommand(host, step.cmds...)
		if err != nil {
			log.Printf("[backup] %s: step %d [%s] failed: %v", host, i+1, step.label, err)
			res.Err = fmt.Errorf("step %d (%s): %w", i+1, step.label, err)
			if len(rawSteps) > 0 {
				break
			}
			return res
		}
		log.Printf("[backup] %s: step %d [%s] done (%d bytes)", host, i+1, step.label, len(out))
		if verbose && w != nil {
			_, _ = fmt.Fprintf(w, "\n========== %s | step %d/%d [%s] — %d bytes ==========\n%s\n========== end step ==========\n",
				host, i+1, len(backupSteps), step.label, len(out), out)
		}
		rawSteps = append(rawSteps, out)
	}

	res.Data = extractor.FinalizeNokiaMultistepBackup(rawSteps)
	if res.Err != nil {
		return res
	}

	if err := os.WriteFile(filePath, []byte(res.Data), 0o644); err != nil {
		log.Printf("[backup] write %s: %v", filePath, err)
	} else {
		res.FilePath = filePath
		log.Printf("[backup] %s: wrote %s (%d bytes)", host, filePath, len(res.Data))
	}
	return res
}

// BackupsWithPool runs CollectBackupPooled for every Nokia OLT using pool (bounded by backupParallelOLTs).
// The caller owns pool lifecycle; do not Close a shared scheduler pool from here.
func BackupsWithPool(pool *shell.ConnectionPool, verbose bool, w io.Writer) []BackupResult {
	nokia, _, err := shell.OLTsData()
	if err != nil {
		log.Printf("[backup] failed to fetch OLT data: %v", err)
		return nil
	}
	if len(nokia) == 0 {
		return nil
	}

	results := make([]BackupResult, len(nokia))
	sem := make(chan struct{}, backupParallelOLTs)
	var wg sync.WaitGroup

	for i, olt := range nokia {
		i, olt := i, olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = CollectBackupPooled(pool, olt.Ip, olt.Name, olt.Site, verbose, w)
		}()
	}
	wg.Wait()
	return results
}

// Backups runs CollectBackupPooled for every Nokia OLT in parallel (bounded by backupParallelOLTs).
func Backups(username, password string, verbose bool, w io.Writer) []BackupResult {
	pool := shell.NewConnectionPool(username, password)
	defer pool.Close()
	return BackupsWithPool(pool, verbose, w)
}

func backupFilePath(device, site, host string) string {
	s := strings.ReplaceAll(site, "/", "-")
	if s == "" {
		s = "unknown"
	}
	folder := filepath.Join("backups", s, time.Now().Format("2006-01-02"))
	if err := os.MkdirAll(folder, 0o755); err != nil {
		log.Printf("[backup] mkdir %s: %v", folder, err)
	}
	name := strings.ReplaceAll(device, "/", "-")
	return filepath.Join(folder, fmt.Sprintf("%s_%s.txt", name, host))
}
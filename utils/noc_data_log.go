package utils

import (
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const defaultNocDataDebugLogPath = "backups/logs/noc-data/noc-data-debug.log"

var nocDataDebugLogWriteMu sync.Mutex
var nocDataTraceHosts sync.Map
var nocDataTraceHostsLoaded sync.Once

func NocDataLogEnabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("NOC_DATA_LOG_ENABLED")))
	switch raw {
	case "", "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

func NocDataLogPath() string {
	if path := strings.TrimSpace(os.Getenv("NOC_DATA_LOG_FILE")); path != "" {
		return path
	}
	return defaultNocDataDebugLogPath
}

func NocDataLogf(format string, v ...any) {
	msg := fmt.Sprintf(format, v...)
	_ = stdlog.Output(2, strings.TrimRight(msg, "\n"))
	appendNocDataDebugLog(msg)
}

func Printf(format string, v ...any) {
	NocDataLogf(format, v...)
}

func NocDataLogln(v ...any) {
	msg := strings.TrimRight(fmt.Sprintln(v...), "\n")
	_ = stdlog.Output(2, msg)
	appendNocDataDebugLog(msg)
}

func Println(v ...any) {
	NocDataLogln(v...)
}

func NocDataFileOnlyf(format string, v ...any) {
	appendNocDataDebugLog(fmt.Sprintf(format, v...))
}

func FileOnlyf(format string, v ...any) {
	NocDataFileOnlyf(format, v...)
}

func NocDataTraceEnabled(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	nocDataTraceHostsLoaded.Do(loadNocDataTraceHosts)
	_, ok := nocDataTraceHosts.Load(host)
	return ok
}

func NocDataTracef(host, format string, v ...any) {
	if !NocDataTraceEnabled(host) {
		return
	}
	NocDataLogf(format, v...)
}

func NocDataTraceFileOnlyf(host, format string, v ...any) {
	if !NocDataTraceEnabled(host) {
		return
	}
	NocDataFileOnlyf(format, v...)
}

func loadNocDataTraceHosts() {
	raw := strings.TrimSpace(os.Getenv("NOC_DATA_TRACE_HOSTS"))
	if raw == "" {
		return
	}
	for _, host := range strings.Split(raw, ",") {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		nocDataTraceHosts.Store(host, struct{}{})
	}
}

func appendNocDataDebugLog(msg string) {
	msg = strings.TrimRight(msg, "\n")
	if msg == "" {
		return
	}
	if !NocDataLogEnabled() {
		return
	}

	nocDataDebugLogWriteMu.Lock()
	defer nocDataDebugLogWriteMu.Unlock()

	path := NocDataLogPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		_ = stdlog.Output(2, fmt.Sprintf("[noc-data-log] mkdir %s: %v", filepath.Dir(path), err))
		return
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		_ = stdlog.Output(2, fmt.Sprintf("[noc-data-log] open %s: %v", path, err))
		return
	}
	defer f.Close()

	line := time.Now().Format("2006/01/02 15:04:05 ") + msg + "\n"
	_, _ = f.WriteString(line)
}

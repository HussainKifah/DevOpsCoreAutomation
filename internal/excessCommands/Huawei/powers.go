package huawei

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/extractor"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

const (
	maxSlots = 16
	maxPons  = 8
	maxOnts  = 128
)

const HafriyaHost = "10.80.2.161"

// SlotForOLT returns the GPON board slot. Hafriya uses slot 0; all others use slot 1.
func SlotForOLT(host string) int {
	if host == HafriyaHost {
		return 0
	}
	return 1
}

type PowerScanConfig struct {
	Slots  []int
	Pons   []int
	OntMax int
	Chunk  int
}

func loadPowerConfig() PowerScanConfig {
	cfg := PowerScanConfig{
		OntMax: maxOnts - 1,
		Chunk:  64,
	}
	if v := os.Getenv("HW_POWER_ONT_CHUNK"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 128 {
			cfg.Chunk = n
		}
	}
	if v := os.Getenv("HW_POWER_ONT_MAX"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 1 && n <= 128 {
			cfg.OntMax = n - 1
		}
	}
	if v := os.Getenv("HW_POWER_SLOTS"); v != "" {
		v = strings.TrimSpace(strings.Split(v, "#")[0])
		if v == "all" {
			cfg.Slots = []int{-1} // sentinel: resolved per-host in slotsFor()
		} else {
			for _, s := range strings.Split(v, ",") {
				if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 && n < maxSlots {
					cfg.Slots = append(cfg.Slots, n)
				}
			}
		}
	}
	if v := os.Getenv("HW_POWER_PONS"); v != "" && strings.TrimSpace(v) != "all" {
		for _, s := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 0 && n < maxPons {
				cfg.Pons = append(cfg.Pons, n)
			}
		}
	}
	return cfg
}

func (c PowerScanConfig) slotsFor(host string) []int {
	if len(c.Slots) == 1 && c.Slots[0] == -1 {
		// "all" — Hafriya starts from slot 0, others from slot 1
		start := SlotForOLT(host)
		slots := make([]int, 0, maxSlots-start)
		for i := start; i < maxSlots; i++ {
			slots = append(slots, i)
		}
		return slots
	}
	if len(c.Slots) > 0 {
		return c.Slots
	}
	return []int{SlotForOLT(host)}
}

func (c PowerScanConfig) ponList() []int {
	if len(c.Pons) > 0 {
		return c.Pons
	}
	p := make([]int, maxPons)
	for i := range p {
		p[i] = i
	}
	return p
}

type OpticalPowerResult struct {
	Device        string
	Site          string
	Host          string
	RawOut        string
	Powers        []extractor.HuaweiOntOptical
	Err           error
	RawLogPath    string // absolute path to *_raw.log (set after save)
	PowersLogPath string // absolute path to *_powers.log
	CommandsRun   int    // display ont optical-info count
	ChunksOK      int
	ChunksFailed  int
}

// logHuaweiPowerSummary logs ONT power scan results to the standard logger (stderr) so they show
// up with API/GORM output; also mirrors to stdout for foreground `go run`.
func logHuaweiPowerSummary(res *OpticalPowerResult) {
	if res == nil {
		return
	}
	cwd, _ := os.Getwd()
	lines := []string{
		"",
		fmt.Sprintf("======== HUAWEI ONT POWER — %s (%s) ========", res.Device, res.Host),
		fmt.Sprintf("  readings parsed:  %d", len(res.Powers)),
		fmt.Sprintf("  raw capture:      %d bytes", len(res.RawOut)),
		fmt.Sprintf("  CLI commands:     %d  (chunks ok: %d, failed: %d)", res.CommandsRun, res.ChunksOK, res.ChunksFailed),
	}
	if res.Err != nil {
		lines = append(lines, fmt.Sprintf("  last error:       %v", res.Err))
	}
	if res.RawLogPath != "" {
		lines = append(lines, fmt.Sprintf("  raw log file:     %s", res.RawLogPath))
	}
	if res.PowersLogPath != "" {
		lines = append(lines, fmt.Sprintf("  powers TSV:       %s", res.PowersLogPath))
	}
	lines = append(lines, fmt.Sprintf("  working dir:      %s", cwd), "==========================================", "")

	for _, ln := range lines {
		log.Printf("[hw-power] %s", ln)
		fmt.Fprintln(os.Stdout, ln)
	}
}

// HuaweiPowerOntJournalFile returns the absolute path to the append-only ONT power scan journal.
func HuaweiPowerOntJournalFile() string {
	p := filepath.Join("logs", "huawei", "power_ont_scans.log")
	abs, _ := filepath.Abs(p)
	return abs
}

// AppendPowerScanJournal appends one line to a durable journal under logs/huawei/.
func AppendPowerScanJournal(line string) {
	dir := filepath.Join("logs", "huawei")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "power_ont_scans.log")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[hw-power] journal append: %v", err)
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
}

// CollectOpticalPowers opens one SSH session, enters config mode once, then
// iterates slot → pon → ONT-chunks collecting optical power data.
func CollectOpticalPowers(host, user, pass, device, site string) (*OpticalPowerResult, error) {
	res := &OpticalPowerResult{Device: device, Site: site, Host: host}
	cfg := loadPowerConfig()
	ts := time.Now().Format("20060102_150405")
	safeHost := strings.NewReplacer(".", "_", ":", "_").Replace(host)

	sess, err := shell.HwOpenSession(host, user, pass)
	if err != nil {
		res.Err = err
		log.Printf("[hw-power] %s (%s): SSH failed: %v", device, host, err)
		fmt.Fprintf(os.Stdout, "[hw-power] %s (%s): SSH failed: %v\n", device, host, err)
		AppendPowerScanJournal(fmt.Sprintf("FAIL ssh host=%s device=%s err=%v", host, device, err))
		return res, err
	}
	defer sess.Close()

	setupOut, err := sess.SendCommands("enable", "scroll 512", "config")
	if err != nil {
		res.Err = fmt.Errorf("setup commands: %w", err)
		log.Printf("[hw-power] %s (%s): setup failed: %v", device, host, res.Err)
		fmt.Fprintf(os.Stdout, "[hw-power] %s (%s): setup failed: %v\n", device, host, res.Err)
		AppendPowerScanJournal(fmt.Sprintf("FAIL setup host=%s device=%s err=%v", host, device, res.Err))
		return res, res.Err
	}

	var (
		powers   []extractor.HuaweiOntOptical
		rawParts = []string{setupOut}
	)

	slots := cfg.slotsFor(host)
	pons := cfg.ponList()
	ontEnd := cfg.OntMax + 1
	chunk := cfg.Chunk

	maxChunksPerSlot := len(pons) * ((ontEnd + chunk - 1) / chunk)
	log.Printf("[hw-power] LIVE %s (%s): start — %d slot(s) × %d PON × ONT 0-%d chunk=%d (up to ~%d chunks per slot)",
		device, host, len(slots), len(pons), cfg.OntMax, chunk, maxChunksPerSlot)

	chunkNum := 0
	for _, slot := range slots {
		log.Printf("[hw-power] LIVE %s: → interface gpon 0/%d", host, slot)
		ifOut, err := sess.SendCommands(fmt.Sprintf("interface gpon 0/%d", slot))
		rawParts = append(rawParts, ifOut)
		if err != nil {
			log.Printf("[hw-power] LIVE %s: slot %d interface FAILED: %v", host, slot, err)
			continue
		}
		if strings.Contains(ifOut, "Failure") || strings.Contains(ifOut, "Unknown command") {
			log.Printf("[hw-power] LIVE %s: slot %d board missing — skip", host, slot)
			continue
		}

		log.Printf("[hw-power] LIVE %s: slot %d OK — scanning PONs", host, slot)

		for _, pon := range pons {
			for ontStart := 0; ontStart < ontEnd; ontStart += chunk {
				end := ontStart + chunk
				if end > ontEnd {
					end = ontEnd
				}

				cmds := make([]string, 0, end-ontStart)
				for ont := ontStart; ont < end; ont++ {
					cmds = append(cmds, fmt.Sprintf("display ont optical-info %d %d", pon, ont))
				}
				res.CommandsRun += len(cmds)
				chunkNum++
				t0 := time.Now()
				log.Printf("[hw-power] LIVE %s: chunk %d slot=%d pon=%d ONT %d-%d (%d cmds) running…",
					host, chunkNum, slot, pon, ontStart, end-1, len(cmds))

				raw, err := sess.SendCommands(cmds...)
				elapsed := time.Since(t0)
				rawParts = append(rawParts, raw)
				if err != nil {
					res.ChunksFailed++
					log.Printf("[hw-power] LIVE %s: chunk %d FAIL after %s — %v", host, chunkNum, elapsed.Round(time.Millisecond), err)
					if res.Err == nil {
						res.Err = err
					}
					continue
				}
				res.ChunksOK++

				indices := make([]struct{ Slot, Pon, Ont int }, 0, end-ontStart)
				for ont := ontStart; ont < end; ont++ {
					indices = append(indices, struct{ Slot, Pon, Ont int }{slot, pon, ont})
				}
				parsed := extractor.ExtractAllHuaweiOntOptical(raw, indices)
				powers = append(powers, parsed...)
				log.Printf("[hw-power] LIVE %s: chunk %d OK in %s — +%d readings this chunk (total %d) raw+%d bytes",
					host, chunkNum, elapsed.Round(time.Millisecond), len(parsed), len(powers), len(raw))
			}
		}

		log.Printf("[hw-power] LIVE %s: slot %d done — leaving interface", host, slot)
		qOut, _ := sess.SendCommands("quit")
		rawParts = append(rawParts, qOut)
	}

	log.Printf("[hw-power] LIVE %s: exiting config", host)
	exitOut, _ := sess.SendCommands("quit")
	rawParts = append(rawParts, exitOut)

	sort.Slice(powers, func(i, j int) bool {
		a, b := powers[i], powers[j]
		if a.Slot != b.Slot {
			return a.Slot < b.Slot
		}
		if a.Pon != b.Pon {
			return a.Pon < b.Pon
		}
		return a.Ont < b.Ont
	})

	res.Powers = powers
	res.RawOut = strings.Join(rawParts, "")

	rawPath, powersPath, saveErr := saveLogs(safeHost, ts, res.RawOut, res.Powers)
	if saveErr != nil {
		log.Printf("[hw-power] %s: writing log files: %v", host, saveErr)
	} else {
		res.RawLogPath = rawPath
		res.PowersLogPath = powersPath
	}

	log.Printf("[hw-power] %s: DONE — readings=%d cmds=%d chunks ok=%d fail=%d raw=%d bytes",
		host, len(res.Powers), res.CommandsRun, res.ChunksOK, res.ChunksFailed, len(res.RawOut))
	logHuaweiPowerSummary(res)
	AppendPowerScanJournal(fmt.Sprintf(
		"OK host=%s device=%s readings=%d cmds=%d chunks_ok=%d chunks_fail=%d raw=%s powers=%s err=%v",
		host, device, len(res.Powers), res.CommandsRun, res.ChunksOK, res.ChunksFailed, res.RawLogPath, res.PowersLogPath, res.Err))

	return res, nil
}

func saveLogs(safeHost, ts, rawOut string, powers []extractor.HuaweiOntOptical) (rawPath, powersPath string, err error) {
	dir := filepath.Join("logs", "huawei")
	if err = os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	rawPath = filepath.Join(dir, fmt.Sprintf("huawei_%s_%s_raw.log", safeHost, ts))
	powersPath = filepath.Join(dir, fmt.Sprintf("huawei_%s_%s_powers.log", safeHost, ts))
	if err = os.WriteFile(rawPath, []byte(rawOut), 0o644); err != nil {
		return "", "", err
	}
	if err = os.WriteFile(powersPath, []byte(formatPowers(powers)), 0o644); err != nil {
		return "", "", err
	}
	rawPath, _ = filepath.Abs(rawPath)
	powersPath, _ = filepath.Abs(powersPath)
	return rawPath, powersPath, nil
}

func formatPowers(p []extractor.HuaweiOntOptical) string {
	var b strings.Builder
	b.WriteString("ont_indx\tont_rx(dBm)\tont_tx(dBm)\tvendor_pn\n")
	for _, o := range p {
		vendor := o.Vendor
		if vendor == "" {
			vendor = "-"
		}
		fmt.Fprintf(&b, "1/1/%d/%d/%d\t%.2f\t%.2f\t%s\n",
			o.Slot, o.Pon, o.Ont, o.RxPower, o.TxPower, vendor)
	}
	return b.String()
}

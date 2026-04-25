package scheduler

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/config"
	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/nocdata"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	noclog "github.com/Flafl/DevOpsCore/utils"
)

type NocDataCollector struct {
	repo      repository.NocDataRepository
	key       []byte
	cfg       *config.Config
	ticker    *time.Ticker
	stop      chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
	runningMu sync.Mutex
	running   bool
}

type nocDataCommandSender func(ctx context.Context, host, user, pass, shellVendor, preferredMethod string, cmds ...string) (string, string, error)

func NewNocDataCollector(repo repository.NocDataRepository, key []byte, cfg *config.Config) *NocDataCollector {
	return &NocDataCollector{
		repo: repo,
		key:  key,
		cfg:  cfg,
		stop: make(chan struct{}),
	}
}

func (c *NocDataCollector) Start() {
	c.ticker = time.NewTicker(7 * 24 * time.Hour)
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.stop:
				return
			case <-c.ticker.C:
				c.RunAllNow()
			}
		}
	}()
	noclog.NocDataLogf(
		"[noc-data] collector started (every 7d) debug_log=%s workers=%d cmd_gap=%s heavy_cmd_gap=%s",
		noclog.NocDataLogPath(),
		c.nocDataWorkers(),
		c.nocDataCommandGap("", ""),
		c.nocDataHeavyCommandGap(),
	)
}

func (c *NocDataCollector) Stop() {
	c.stopOnce.Do(func() {
		if c.ticker != nil {
			c.ticker.Stop()
		}
		close(c.stop)
		c.wg.Wait()
	})
}

func (c *NocDataCollector) RunAllNow() {
	c.runningMu.Lock()
	if c.running {
		c.runningMu.Unlock()
		return
	}
	c.running = true
	c.runningMu.Unlock()

	defer func() {
		c.runningMu.Lock()
		c.running = false
		c.runningMu.Unlock()
	}()

	list, err := c.repo.ListAll()
	if err != nil {
		noclog.NocDataLogf("[noc-data] list devices: %v", err)
		return
	}
	workers := c.nocDataWorkers()
	jobs := make(chan *models.NocDataDevice)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				if err := c.collectOne(d); err != nil {
					noclog.NocDataLogf("[noc-data] collect id=%d host=%s: %v", d.ID, d.Host, err)
				}
			}
		}()
	}

	for i := range list {
		jobs <- &list[i]
	}
	close(jobs)
	wg.Wait()

	c.recoverFailedAfterFullRun()
	c.saveHistorySnapshot(time.Now())
}

func (c *NocDataCollector) CollectDeviceNow(id uint) {
	d, err := c.repo.GetByID(id)
	if err != nil {
		noclog.NocDataLogf("[noc-data] get device id=%d: %v", id, err)
		return
	}
	if err := c.collectOne(d); err != nil {
		noclog.NocDataLogf("[noc-data] collect id=%d host=%s: %v", d.ID, d.Host, err)
	}
}

func (c *NocDataCollector) RecoverFailedDeviceNow(id uint) {
	d, err := c.repo.GetByID(id)
	if err != nil {
		noclog.NocDataLogf("[noc-data] get failed recovery device id=%d: %v", id, err)
		return
	}
	if err := c.collectOneWithSender(d, c.recoveryCommandSender); err != nil {
		noclog.NocDataLogf("[noc-data] failed recovery id=%d host=%s: %v", d.ID, d.Host, err)
	}
}

func (c *NocDataCollector) collectOne(d *models.NocDataDevice) error {
	return c.collectOneWithSender(d, c.standardCommandSender)
}

func (c *NocDataCollector) recoverFailedAfterFullRun() {
	list, err := c.repo.ListAll()
	if err != nil {
		noclog.NocDataLogf("[noc-data] list failed devices for post-run recovery: %v", err)
		return
	}

	failedIDs := make([]uint, 0)
	for i := range list {
		if strings.ToLower(strings.TrimSpace(list[i].LastStatus)) == "fail" {
			failedIDs = append(failedIDs, list[i].ID)
		}
	}
	if len(failedIDs) == 0 {
		noclog.NocDataLogf("[noc-data] post-run failed recovery skipped: no failed devices")
		return
	}

	encUser, err := crypto.Encrypt(c.key, "")
	if err != nil {
		noclog.NocDataLogf("[noc-data] post-run failed recovery encrypt username: %v", err)
		return
	}
	encPass, err := crypto.Encrypt(c.key, "")
	if err != nil {
		noclog.NocDataLogf("[noc-data] post-run failed recovery encrypt password: %v", err)
		return
	}

	noclog.NocDataLogf("[noc-data] post-run failed recovery started failed_count=%d", len(failedIDs))
	for _, id := range failedIDs {
		if err := c.resetDeviceForRecovery(id, encUser, encPass); err != nil {
			noclog.NocDataLogf("[noc-data] post-run failed recovery reset id=%d: %v", id, err)
			continue
		}
		c.RecoverFailedDeviceNow(id)
	}
	noclog.NocDataLogf("[noc-data] post-run failed recovery completed failed_count=%d", len(failedIDs))
}

func (c *NocDataCollector) resetDeviceForRecovery(id uint, encUser, encPass []byte) error {
	return c.repo.UpdateDevice(id, map[string]interface{}{
		"vendor":            "pending",
		"access_method":     "pending",
		"enc_username":      encUser,
		"enc_password":      encPass,
		"last_status":       "pending",
		"last_error":        "",
		"hostname":          "",
		"device_model":      "",
		"version":           "",
		"serial":            "",
		"uptime":            "",
		"if_up":             0,
		"if_down":           0,
		"default_router":    false,
		"layer_mode":        "",
		"user_count":        0,
		"users":             "",
		"ssh_enabled":       false,
		"telnet_enabled":    false,
		"snmp_enabled":      false,
		"ntp_enabled":       false,
		"aaa_enabled":       false,
		"syslog_enabled":    false,
		"last_collected_at": nil,
	})
}

func (c *NocDataCollector) saveHistorySnapshot(runAt time.Time) {
	list, err := c.repo.ListAll()
	if err != nil {
		noclog.NocDataLogf("[noc-data] history snapshot list devices: %v", err)
		return
	}
	if len(list) == 0 {
		noclog.NocDataLogf("[noc-data] history snapshot skipped: no devices")
		return
	}
	if err := c.repo.CreateHistorySnapshot(runAt.UTC(), list); err != nil {
		noclog.NocDataLogf("[noc-data] history snapshot save failed: %v", err)
		return
	}
	noclog.NocDataLogf("[noc-data] history snapshot saved run_at=%s rows=%d", runAt.UTC().Format(time.RFC3339), len(list))
}

func (c *NocDataCollector) collectOneWithSender(d *models.NocDataDevice, sender nocDataCommandSender) error {
	noclog.NocDataTracef(
		strings.TrimSpace(d.Host),
		"[noc-data-trace] host=%s id=%d collect start vendor=%q method=%q status=%q",
		strings.TrimSpace(d.Host),
		d.ID,
		strings.TrimSpace(d.Vendor),
		strings.TrimSpace(d.AccessMethod),
		strings.TrimSpace(d.LastStatus),
	)
	if c.cfg == nil {
		err := fmt.Errorf("collector config is missing")
		c.failDevice(d.ID, err)
		return err
	}
	excluded, err := c.isExcluded(strings.TrimSpace(d.Host))
	if err != nil {
		c.failDevice(d.ID, err)
		return err
	}
	if excluded {
		err := fmt.Errorf("host %s is excluded from NOC data collection", strings.TrimSpace(d.Host))
		c.failDevice(d.ID, err)
		return err
	}
	if err := shell.PingNocDataHost(strings.TrimSpace(d.Host), time.Second); err != nil {
		err = fmt.Errorf("host %s is unreachable by ping: %w", strings.TrimSpace(d.Host), err)
		c.failDevice(d.ID, err)
		return err
	}

	shellVendor, appVendor, user, pass, method, encUser, encPass, err := c.resolveCredentialsWithSender(d, sender)
	if err != nil {
		c.failDevice(d.ID, err)
		return err
	}
	_, cmds, err := nocdata.CommandsForVendor(appVendor)
	if err != nil {
		c.failDevice(d.ID, err)
		return err
	}
	sections := make(map[string]string, len(cmds))
	for _, cmd := range cmds {
		noclog.NocDataTracef(
			strings.TrimSpace(d.Host),
			"[noc-data-trace] host=%s vendor=%s command start method=%s cmd=%q",
			strings.TrimSpace(d.Host),
			appVendor,
			method,
			cmd,
		)
		out, usedMethod, runErr := c.runCommandOnceWithSender(
			strings.TrimSpace(d.Host),
			strings.TrimSpace(user),
			strings.TrimSpace(pass),
			shellVendor,
			appVendor,
			method,
			cmd,
			sender,
		)
		if runErr != nil {
			noclog.NocDataTracef(
				strings.TrimSpace(d.Host),
				"[noc-data-trace] host=%s vendor=%s command fail method=%s cmd=%q err=%v",
				strings.TrimSpace(d.Host),
				appVendor,
				usedMethod,
				cmd,
				runErr,
			)
			runErr = fmt.Errorf("command %q failed during collection: %w", cmd, runErr)
			if isCriticalNocDataCommand(appVendor, cmd) {
				c.failDevice(d.ID, runErr)
				return runErr
			}
			noclog.NocDataLogf(
				"[noc-data] host=%s vendor=%s non-critical command failed cmd=%q err=%v; continuing collection",
				strings.TrimSpace(d.Host),
				appVendor,
				cmd,
				runErr,
			)
			sections[cmd] = ""
			continue
		}
		noclog.NocDataTracef(
			strings.TrimSpace(d.Host),
			"[noc-data-trace] host=%s vendor=%s command ok method=%s cmd=%q output_bytes=%d",
			strings.TrimSpace(d.Host),
			appVendor,
			usedMethod,
			cmd,
			len(strings.TrimSpace(out)),
		)
		if usedMethod == "telnet" {
			method = "telnet"
		} else if method == "" {
			method = usedMethod
		}
		noclog.NocDataLogf(
			"[noc-data] host=%s vendor=%s method=%s cmd=%q output_bytes=%d",
			strings.TrimSpace(d.Host),
			appVendor,
			usedMethod,
			cmd,
			len(strings.TrimSpace(out)),
		)
		noclog.NocDataFileOnlyf(
			"[noc-data] host=%s vendor=%s method=%s cmd=%q full_output_begin\n%s\n[noc-data] host=%s cmd=%q full_output_end",
			strings.TrimSpace(d.Host),
			appVendor,
			usedMethod,
			cmd,
			out,
			strings.TrimSpace(d.Host),
			cmd,
		)
		sections[cmd] = out
		if gap := c.nocDataCommandGap(appVendor, cmd); gap > 0 {
			noclog.NocDataTracef(
				strings.TrimSpace(d.Host),
				"[noc-data-trace] host=%s vendor=%s sleeping_between_commands=%s after cmd=%q",
				strings.TrimSpace(d.Host),
				appVendor,
				gap,
				cmd,
			)
			time.Sleep(gap)
		}
	}
	deviceForSnapshot := *d
	deviceForSnapshot.Vendor = appVendor
	snapshot := nocdata.CollectSnapshot(&deviceForSnapshot, sections)
	appVendor = normalizeNocDataVendor(appVendor, snapshot.Model)
	d.Vendor = appVendor
	snapshot.Method = method
	if snapshot.Status == "fail" && strings.EqualFold(appVendor, "mikrotik") {
		logIncompleteMikrotikSnapshot(strings.TrimSpace(d.Host), sections, snapshot.Error)
	}
	updates := map[string]interface{}{
		"vendor":         appVendor,
		"access_method":  snapshot.Method,
		"enc_username":   encUser,
		"enc_password":   encPass,
		"last_status":    snapshot.Status,
		"last_error":     snapshot.Error,
		"hostname":       snapshot.Hostname,
		"device_model":   snapshot.Model,
		"version":        snapshot.Version,
		"serial":         snapshot.Serial,
		"uptime":         snapshot.Uptime,
		"if_up":          snapshot.IFUp,
		"if_down":        snapshot.IFDown,
		"default_router": snapshot.DefaultRouter,
		"layer_mode":     snapshot.LayerMode,
		"user_count":     snapshot.UserCount,
		"users":          strings.Join(snapshot.Users, ", "),
		"ssh_enabled":    snapshot.SSHEnabled,
		"telnet_enabled": snapshot.TelnetEnabled,
		"snmp_enabled":   snapshot.SNMPEnabled,
		"ntp_enabled":    snapshot.NTPEnabled,
		"aaa_enabled":    snapshot.AAAEnabled,
		"syslog_enabled": snapshot.SyslogEnabled,
	}
	if err := c.repo.UpdateSnapshot(d.ID, updates); err != nil {
		noclog.NocDataLogf(
			"[noc-data] host=%s id=%d: snapshot update failed status=%s err=%q updateErr=%v",
			strings.TrimSpace(d.Host),
			d.ID,
			snapshot.Status,
			snapshot.Error,
			err,
		)
		return err
	}
	if snapshot.Status == "ok" {
		if err := c.removeDuplicateDevices(d.ID, snapshot); err != nil {
			noclog.NocDataLogf(
				"[noc-data] host=%s id=%d: duplicate cleanup failed hostname=%q model=%q version=%q serial=%q err=%v",
				strings.TrimSpace(d.Host),
				d.ID,
				snapshot.Hostname,
				snapshot.Model,
				snapshot.Version,
				snapshot.Serial,
				err,
			)
		}
	}
	noclog.NocDataTracef(
		strings.TrimSpace(d.Host),
		"[noc-data-trace] host=%s id=%d collect complete status=%s error=%q",
		strings.TrimSpace(d.Host),
		d.ID,
		snapshot.Status,
		snapshot.Error,
	)
	return nil
}

func normalizeNocDataVendor(vendor, model string) string {
	normalizedVendor := strings.ToLower(strings.TrimSpace(vendor))
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if (normalizedVendor == "cisco_ios" || normalizedVendor == "cisco_nexus") &&
		(strings.Contains(normalizedModel, "nexus") || strings.Contains(normalizedModel, "n9k") || strings.Contains(normalizedModel, "nexus9000")) {
		return "cisco_nexus"
	}
	return normalizedVendor
}

func (c *NocDataCollector) removeDuplicateDevices(currentID uint, snapshot nocdata.Snapshot) error {
	hostname := strings.TrimSpace(snapshot.Hostname)
	model := strings.TrimSpace(snapshot.Model)
	version := strings.TrimSpace(snapshot.Version)
	serial := strings.TrimSpace(snapshot.Serial)
	if hostname == "" || model == "" || version == "" || serial == "" {
		return nil
	}

	matches, err := c.repo.FindByIdentity(hostname, model, version, serial)
	if err != nil {
		return err
	}
	if len(matches) <= 1 {
		return nil
	}

	keepID := matches[0].ID
	for _, item := range matches[1:] {
		if item.ID == keepID {
			continue
		}
		if err := c.repo.HardDelete(item.ID); err != nil {
			return fmt.Errorf("hard delete duplicate id=%d keep_id=%d: %w", item.ID, keepID, err)
		}
		noclog.NocDataLogf(
			"[noc-data] duplicate removed keep_id=%d removed_id=%d host=%s duplicate_ip=%s hostname=%q model=%q version=%q serial=%q",
			keepID,
			item.ID,
			strings.TrimSpace(item.Host),
			strings.TrimSpace(item.Host),
			hostname,
			model,
			version,
			serial,
		)
	}
	if currentID != keepID {
		noclog.NocDataLogf(
			"[noc-data] current collected row id=%d matched existing device keep_id=%d hostname=%q model=%q version=%q serial=%q",
			currentID,
			keepID,
			hostname,
			model,
			version,
			serial,
		)
	}
	return nil
}

func (c *NocDataCollector) runCommandOnce(host, user, pass, shellVendor, appVendor, preferredMethod, cmd string) (string, string, error) {
	return c.runCommandOnceWithSender(host, user, pass, shellVendor, appVendor, preferredMethod, cmd, c.standardCommandSender)
}

func (c *NocDataCollector) runCommandOnceWithSender(host, user, pass, shellVendor, appVendor, preferredMethod, cmd string, sender nocDataCommandSender) (string, string, error) {
	commandAttemptTimeout := nocDataCommandAttemptTimeout(appVendor, cmd)
	noclog.NocDataTracef(
		host,
		"[noc-data-trace] host=%s vendor=%s cmd=%q preferred_method=%s shell_vendor=%s",
		host,
		appVendor,
		cmd,
		preferredMethod,
		shellVendor,
	)
	ctx, cancel := context.WithTimeout(context.Background(), commandAttemptTimeout)
	out, usedMethod, err := sender(ctx, host, user, pass, shellVendor, preferredMethod, cmd)
	cancel()
	if err == nil && nocdata.OutputLooksUnauthorized(out) {
		noclog.NocDataFileOnlyf(
			"[noc-data] host=%s vendor=%s cmd=%q unauthorized_output_begin\n%s\n[noc-data] host=%s cmd=%q unauthorized_output_end",
			host,
			appVendor,
			cmd,
			out,
			host,
			cmd,
		)
		err = fmt.Errorf("command output indicates authorization failure")
	}
	if err != nil {
		return "", usedMethod, err
	}
	return out, usedMethod, nil
}

func (c *NocDataCollector) standardCommandSender(ctx context.Context, host, user, pass, shellVendor, preferredMethod string, cmds ...string) (string, string, error) {
	return shell.NocDataSendCommandUsingMethodContext(ctx, host, user, pass, shellVendor, preferredMethod, cmds...)
}

func (c *NocDataCollector) recoveryCommandSender(ctx context.Context, host, user, pass, shellVendor, preferredMethod string, cmds ...string) (string, string, error) {
	return shell.NocDataRecoverySendCommandUsingMethodContext(ctx, host, user, pass, shellVendor, preferredMethod, cmds...)
}

func nocDataCommandAttemptTimeout(appVendor, cmd string) time.Duration {
	vendor := strings.ToLower(strings.TrimSpace(appVendor))
	command := strings.TrimSpace(cmd)

	switch vendor {
	case "cisco_ios", "cisco_nexus":
		switch command {
		case "show logging":
			return 45 * time.Second
		case "show version", "show int status":
			return 35 * time.Second
		case "show running-config | include 0.0.0.0",
			"show running-config | include ^username",
			"show running-config | section line vty",
			"show running-config | section aaa":
			return 30 * time.Second
		default:
			return 15 * time.Second
		}
	case "mikrotik":
		switch command {
		case "/system identity print",
			"/system resource print",
			"/system routerboard print",
			"/ip service print detail":
			return 20 * time.Second
		case "/interface print",
			"/user print detail",
			"/system logging print":
			return 15 * time.Second
		default:
			return 12 * time.Second
		}
	case "huawei":
		switch command {
		case "display elabel":
			return 75 * time.Second
		case "display current-configuration":
			return 60 * time.Second
		case "display version",
			"display interface description | include GE",
			"display ip routing-table",
			"display tcp status":
			return 30 * time.Second
		case "screen-length 0 temporary":
			return 15 * time.Second
		default:
			return 20 * time.Second
		}
	default:
		return 15 * time.Second
	}
}

func (c *NocDataCollector) nocDataWorkers() int {
	if c != nil && c.cfg != nil && c.cfg.NocDataWorkers >= 1 {
		return c.cfg.NocDataWorkers
	}
	return 3
}

func (c *NocDataCollector) nocDataHeavyCommandGap() time.Duration {
	if c != nil && c.cfg != nil {
		return c.cfg.NocDataHeavyCmdGap
	}
	return 750 * time.Millisecond
}

func (c *NocDataCollector) nocDataCommandGap(appVendor, cmd string) time.Duration {
	base := 250 * time.Millisecond
	if c != nil && c.cfg != nil {
		base = c.cfg.NocDataCommandGap
	}
	if isHeavyNocDataCommand(appVendor, cmd) {
		return c.nocDataHeavyCommandGap()
	}
	return base
}

func isHeavyNocDataCommand(appVendor, cmd string) bool {
	vendor := strings.ToLower(strings.TrimSpace(appVendor))
	command := strings.TrimSpace(cmd)

	switch vendor {
	case "huawei":
		return command == "display elabel" || command == "display current-configuration"
	case "cisco_ios", "cisco_nexus":
		return command == "show version" || command == "show logging"
	case "mikrotik":
		return command == "/system resource print" || command == "/user print detail" || command == "/system logging print"
	default:
		return false
	}
}

func isCriticalNocDataCommand(appVendor, cmd string) bool {
	vendor := strings.ToLower(strings.TrimSpace(appVendor))
	command := strings.TrimSpace(cmd)

	switch vendor {
	case "mikrotik":
		switch command {
		case "/system identity print",
			"/system resource print",
			"/system routerboard print",
			"/interface print",
			"/ip service print detail":
			return true
		default:
			return false
		}
	case "cisco_ios", "cisco_nexus":
		switch command {
		case "show version", "show int status":
			return true
		default:
			return false
		}
	default:
		return true
	}
}

func logIncompleteMikrotikSnapshot(host string, sections map[string]string, reason string) {
	critical := []string{
		"/system identity print",
		"/system resource print",
		"/system routerboard print",
		"/interface print",
		"/ip service print detail",
	}
	for _, cmd := range critical {
		trimmed := strings.TrimSpace(sections[cmd])
		preview := trimmed
		if len(preview) > 180 {
			preview = preview[:180] + "..."
		}
		noclog.NocDataLogf(
			"[noc-data] host=%s mikrotik incomplete snapshot reason=%q cmd=%q output_bytes=%d preview=%q",
			host,
			reason,
			cmd,
			len(trimmed),
			preview,
		)
	}
}

func (c *NocDataCollector) resolveCredentials(d *models.NocDataDevice) (string, string, string, string, string, []byte, []byte, error) {
	return c.resolveCredentialsWithSender(d, c.standardCommandSender)
}

func (c *NocDataCollector) resolveCredentialsWithSender(d *models.NocDataDevice, sender nocDataCommandSender) (string, string, string, string, string, []byte, []byte, error) {
	host := strings.TrimSpace(d.Host)
	preferredMethod := preferredNocDataMethod(d.AccessMethod)
	ordered := []string{}

	if strings.HasPrefix(host, "10.90.") {
		ordered = append(ordered, "huawei")
	}

	if cachedFamily := credentialVendorFamily(d.Vendor); cachedFamily != "" {
		switch cachedFamily {
		case "cisco":
			ordered = append(ordered, "cisco_ios")
		case "huawei":
			ordered = append(ordered, "huawei")
		case "mikrotik":
			ordered = append(ordered, "mikrotik")
		}
	}
	ordered = append(ordered, "cisco_ios", "huawei", "mikrotik")

	seen := make(map[string]struct{}, len(ordered))
	candidates := make([]string, 0, len(ordered))
	for _, v := range ordered {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		candidates = append(candidates, v)
	}

	cachedVendorFamily := credentialVendorFamily(d.Vendor)
	cachedCred, cachedErr := c.cachedCredentialForDevice(d)
	if cachedErr != nil {
		noclog.NocDataLogf("[noc-data] host=%s: decrypt cached credential failed: %v", host, cachedErr)
	}

	var errs []string
	for _, candidate := range candidates {
		shellVendor, creds, err := c.credentialsForVendor(candidate)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		orderedCreds := prependCachedCredential(cachedCred, cachedVendorFamily, candidate, creds)
		for _, cred := range orderedCreds {
			noclog.NocDataTracef(
				host,
				"[noc-data-trace] host=%s credential probe vendor=%s preferred_method=%s user=%q",
				host,
				candidate,
				preferredMethod,
				cred.Username,
			)
			noclog.NocDataLogf(
				"[noc-data] host=%s: probing vendor=%s preferred_method=%s user=%q",
				host,
				candidate,
				preferredMethod,
				cred.Username,
			)
			method, probeErr := c.probeDeviceWithSender(host, cred.Username, cred.Password, shellVendor, candidate, preferredMethod, sender)
			if probeErr != nil {
				noclog.NocDataTracef(
					host,
					"[noc-data-trace] host=%s credential probe failed vendor=%s user=%q err=%v",
					host,
					candidate,
					cred.Username,
					probeErr,
				)
				noclog.NocDataLogf(
					"[noc-data] host=%s: probe failed vendor=%s method=%s user=%q err=%v",
					host,
					candidate,
					preferredMethod,
					cred.Username,
					probeErr,
				)
				errs = append(errs, candidate+"["+cred.Username+"]: "+probeErr.Error())
				continue
			}

			noclog.NocDataTracef(
				host,
				"[noc-data-trace] host=%s credential probe succeeded vendor=%s method=%s user=%q",
				host,
				candidate,
				method,
				cred.Username,
			)
			noclog.NocDataLogf(
				"[noc-data] host=%s: probe succeeded vendor=%s method=%s user=%q",
				host,
				candidate,
				method,
				cred.Username,
			)
			return shellVendor, candidate, cred.Username, cred.Password, method, cred.EncUsername, cred.EncPassword, nil
		}
	}

	if len(errs) == 0 {
		return "", "", "", "", "", nil, nil, fmt.Errorf("no NOC data credentials available")
	}
	slices.Sort(errs)
	return "", "", "", "", "", nil, nil, fmt.Errorf("unable to determine vendor/login for %s: %s", d.Host, strings.Join(errs, "; "))
}

type nocDataCredential struct {
	Username    string
	Password    string
	EncUsername []byte
	EncPassword []byte
}

func (c *NocDataCollector) credentialsForVendor(vendor string) (string, []nocDataCredential, error) {
	family := credentialVendorFamily(vendor)
	if family == "" {
		return "", nil, fmt.Errorf("unsupported vendor %q", vendor)
	}
	list, err := c.repo.ListCredentials(family)
	if err != nil {
		return "", nil, err
	}
	if len(list) == 0 {
		return "", nil, fmt.Errorf("missing %s credentials in NOC Data users", family)
	}

	out := make([]nocDataCredential, 0, len(list))
	for _, item := range list {
		user, userErr := crypto.Decrypt(c.key, item.EncUsername)
		if userErr != nil {
			return "", nil, fmt.Errorf("decrypt username: %w", userErr)
		}
		pass, passErr := crypto.Decrypt(c.key, item.EncPassword)
		if passErr != nil {
			return "", nil, fmt.Errorf("decrypt password: %w", passErr)
		}
		out = append(out, nocDataCredential{
			Username:    strings.TrimSpace(user),
			Password:    pass,
			EncUsername: append([]byte(nil), item.EncUsername...),
			EncPassword: append([]byte(nil), item.EncPassword...),
		})
	}

	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios", "cisco_nexus":
		shellVendor, _ := nocdata.ShellVendor("cisco_ios")
		return shellVendor, out, nil
	case "huawei":
		shellVendor, _ := nocdata.ShellVendor("huawei")
		return shellVendor, out, nil
	case "mikrotik":
		shellVendor, _ := nocdata.ShellVendor("mikrotik")
		return shellVendor, out, nil
	default:
		return "", nil, fmt.Errorf("unsupported vendor %q", vendor)
	}
}

func (c *NocDataCollector) cachedCredentialForDevice(d *models.NocDataDevice) (*nocDataCredential, error) {
	if len(d.EncUsername) == 0 || len(d.EncPassword) == 0 {
		return nil, nil
	}

	user, err := crypto.Decrypt(c.key, d.EncUsername)
	if err != nil {
		return nil, fmt.Errorf("decrypt cached username: %w", err)
	}
	user = strings.TrimSpace(user)
	if user == "" {
		return nil, nil
	}

	pass, err := crypto.Decrypt(c.key, d.EncPassword)
	if err != nil {
		return nil, fmt.Errorf("decrypt cached password: %w", err)
	}

	return &nocDataCredential{
		Username:    user,
		Password:    pass,
		EncUsername: append([]byte(nil), d.EncUsername...),
		EncPassword: append([]byte(nil), d.EncPassword...),
	}, nil
}

func prependCachedCredential(cached *nocDataCredential, cachedVendorFamily, candidateVendor string, creds []nocDataCredential) []nocDataCredential {
	if cached == nil || strings.TrimSpace(cached.Username) == "" {
		return creds
	}
	if cachedVendorFamily == "" || cachedVendorFamily != credentialVendorFamily(candidateVendor) {
		return creds
	}

	out := make([]nocDataCredential, 0, len(creds)+1)
	out = append(out, *cached)
	for _, cred := range creds {
		if strings.EqualFold(strings.TrimSpace(cred.Username), strings.TrimSpace(cached.Username)) &&
			cred.Password == cached.Password {
			continue
		}
		out = append(out, cred)
	}
	return out
}

func credentialVendorFamily(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios", "cisco_nexus":
		return "cisco"
	case "huawei":
		return "huawei"
	case "mikrotik":
		return "mikrotik"
	default:
		return ""
	}
}

func (c *NocDataCollector) probeDevice(host, user, pass, shellVendor, appVendor, preferredMethod string) (string, error) {
	return c.probeDeviceWithSender(host, user, pass, shellVendor, appVendor, preferredMethod, c.standardCommandSender)
}

func (c *NocDataCollector) probeDeviceWithSender(host, user, pass, shellVendor, appVendor, preferredMethod string, sender nocDataCommandSender) (string, error) {
	cmd := "show version"
	if appVendor == "mikrotik" {
		cmd = "/system identity print"
	} else if appVendor == "huawei" {
		cmd = "display version"
	}

	methods := orderedNocDataMethods(preferredMethod)
	var errs []string
	for _, method := range methods {
		noclog.NocDataTracef(
			host,
			"[noc-data-trace] host=%s probe start vendor=%s requested_method=%s shell_vendor=%s user=%q cmd=%q",
			host,
			appVendor,
			method,
			shellVendor,
			strings.TrimSpace(user),
			cmd,
		)
		out, usedMethod, err := sender(
			context.Background(),
			host,
			strings.TrimSpace(user),
			strings.TrimSpace(pass),
			shellVendor,
			method,
			cmd,
		)
		if err == nil {
			if !nocdata.ProbeOutputLooksValid(appVendor, cmd, out) {
				noclog.NocDataFileOnlyf(
					"[noc-data] host=%s: probe vendor=%s method=%s user=%q invalid_output_begin\n%s\n[noc-data] host=%s: invalid_output_end",
					host,
					appVendor,
					usedMethod,
					strings.TrimSpace(user),
					out,
					host,
				)
				errs = append(errs, method+": probe output did not match expected "+appVendor+" signature")
				continue
			}
			noclog.NocDataFileOnlyf(
				"[noc-data] host=%s: probe vendor=%s method=%s user=%q output_begin\n%s\n[noc-data] host=%s: probe output_end",
				host,
				appVendor,
				usedMethod,
				strings.TrimSpace(user),
				out,
				host,
			)
			noclog.NocDataTracef(
				host,
				"[noc-data-trace] host=%s probe success vendor=%s method=%s output_bytes=%d",
				host,
				appVendor,
				usedMethod,
				len(strings.TrimSpace(out)),
			)
			return usedMethod, nil
		}
		noclog.NocDataTracef(
			host,
			"[noc-data-trace] host=%s probe fail vendor=%s method=%s err=%v",
			host,
			appVendor,
			method,
			err,
		)
		errs = append(errs, method+": "+err.Error())
	}
	return "", fmt.Errorf("%s", strings.Join(errs, "; "))
}

func orderedNocDataMethods(preferred string) []string {
	switch strings.ToLower(strings.TrimSpace(preferred)) {
	case "ssh":
		return []string{"ssh", "telnet"}
	case "telnet":
		return []string{"telnet", "ssh"}
	default:
		return []string{"auto"}
	}
}

func preferredNocDataMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "ssh", "telnet":
		return strings.ToLower(strings.TrimSpace(method))
	default:
		return "auto"
	}
}

func (c *NocDataCollector) isExcluded(host string) (bool, error) {
	list, err := c.repo.ListExclusions()
	if err != nil {
		return false, err
	}
	for _, item := range list {
		match, matchErr := nocdata.HostMatchesIPv4Spec(item.Subnet, item.Target, host)
		if matchErr != nil {
			noclog.NocDataLogf("[noc-data] skip invalid exclusion id=%d subnet=%s target=%s: %v", item.ID, item.Subnet, item.Target, matchErr)
			continue
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

func (c *NocDataCollector) failDevice(id uint, err error) {
	noclog.NocDataLogf("[noc-data] id=%d: failDevice full error: %v", id, err)
	snapshot := nocdata.ParseFailure(err)
	if updateErr := c.repo.UpdateSnapshot(id, map[string]interface{}{
		"last_status": snapshot.Status,
		"last_error":  snapshot.Error,
	}); updateErr != nil {
		noclog.NocDataLogf("[noc-data] id=%d: failDevice snapshot update failed: %v", id, updateErr)
	}
}

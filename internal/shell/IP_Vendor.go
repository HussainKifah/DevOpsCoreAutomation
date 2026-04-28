package shell

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	log "github.com/Flafl/DevOpsCore/utils"
	"github.com/scrapli/scrapligo/driver/generic"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/transport"
	"github.com/scrapli/scrapligo/util"
	"golang.org/x/crypto/ssh"
)

type VendorProfile struct {
	PromptPattern  *regexp.Regexp
	SetupCmds      []string
	BackupCmd      string
	TimeoutOps     time.Duration
	TransportType  string
	UsernameSuffix string
	UseRawSSH      bool
}

var vendorProfiles = map[string]VendorProfile{
	"nokia": {
		PromptPattern: regexp.MustCompile(`(?m)[#>]\s*$`),
		SetupCmds:     []string{"environment no more"},
		BackupCmd:     "admin display-config",
		TimeoutOps:    10 * time.Minute,
		TransportType: "standard",
	},
	// Prompt must match exec (# / >) and config submodes: hostname(config)#, (config-line)#, etc.
	// Parentheses are required — without them scrapligo never sees the prompt after "configure terminal".
	"cisco": {
		PromptPattern: regexp.MustCompile(`(?m)[a-zA-Z0-9._():\/@\-]+[#>]\s*$`),
		SetupCmds:     []string{"terminal length 0", "terminal width 0"},
		BackupCmd:     "show running-config",
		TimeoutOps:    5 * time.Minute,
		TransportType: "standard",
	},
	"nexus": {
		PromptPattern: regexp.MustCompile(`(?m)[a-zA-Z0-9._():\/@\-]+[#>]\s*$`),
		SetupCmds:     []string{"terminal length 0", "terminal width 0"},
		TimeoutOps:    5 * time.Minute,
		TransportType: "standard",
	},
	"mikrotik": {
		BackupCmd:  "/export",
		TimeoutOps: 10 * time.Minute,
		UseRawSSH:  true,
	},
	"huawei": {
		BackupCmd:  "display current-configuration",
		TimeoutOps: 10 * time.Minute,
		UseRawSSH:  true,
	},
}

var legacySystemSSHPasswordPrompt = regexp.MustCompile(`(?im)(?:.*@.*)?password(?: for [^:\r\n]+)?\s*:?\s?$`)
var huaweiWorkflowMoreRe = regexp.MustCompile(`(?i)(----\s*more\s*----|--more--|<---\s*more\s*--->)`)
var huaweiWorkflowContinueRe = regexp.MustCompile(`(?im)(continue\?\s*\[y/n\]\s*:?\s*[a-z]*\s*$|please choose 'yes' or 'no' first before pressing 'enter'.*\[y/n\]\s*:?\s*[a-z]*\s*$)`)

func mikrotikSSH(host, user, pass string, timeout time.Duration, cmds ...string) (string, error) {
	sshUser := user + "+cet511w4098h"
	addr := host + ":22"

	log.Printf("[ip-ssh] %s (mikrotik): raw SSH — user=%q timeout=%s cmds=%v", host, sshUser, timeout, cmds)

	cfg := &ssh.ClientConfig{
		User: sshUser,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pass
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         60 * time.Second,
		Config:          wideSSHConfig(),
	}

	log.Printf("[ip-ssh] %s (mikrotik): dialing %s ...", host, addr)
	conn, err := net.DialTimeout("tcp", addr, cfg.Timeout)
	if err != nil {
		log.Printf("[ip-ssh] %s (mikrotik): ✘ TCP dial failed: %v", host, err)
		return "", fmt.Errorf("tcp dial: %w", err)
	}

	var sshConn ssh.Conn
	var chans <-chan ssh.NewChannel
	var reqs <-chan *ssh.Request
	err = withWeakRSAHostKeySupport(func() error {
		var handshakeErr error
		sshConn, chans, reqs, handshakeErr = ssh.NewClientConn(conn, addr, cfg)
		return handshakeErr
	})
	if err != nil {
		conn.Close()
		log.Printf("[ip-ssh] %s (mikrotik): ✘ SSH handshake failed: %v", host, err)
		return "", fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()
	log.Printf("[ip-ssh] %s (mikrotik): ✔ SSH connected", host)

	fullCmd := strings.Join(cmds, "; ")

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)

	go func() {
		session, sErr := client.NewSession()
		if sErr != nil {
			ch <- result{"", fmt.Errorf("new session: %w", sErr)}
			return
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr

		log.Printf("[ip-ssh] %s (mikrotik): exec command: %q", host, fullCmd)
		runErr := session.Run(fullCmd)
		out := stdout.String()
		if runErr != nil && out == "" {
			errStr := stderr.String()
			if errStr != "" {
				log.Printf("[ip-ssh] %s (mikrotik): stderr: %s", host, errStr)
			}
			ch <- result{"", fmt.Errorf("exec: %w", runErr)}
			return
		}
		log.Printf("[ip-ssh] %s (mikrotik): ✔ command done — %d bytes", host, len(out))
		ch <- result{out, nil}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			log.Printf("[ip-ssh] %s (mikrotik): ✘ command failed: %v", host, res.err)
			return "", res.err
		}
		if len(res.output) > 200 {
			log.Printf("[ip-ssh] %s (mikrotik): output preview: %s...", host, res.output[:200])
		} else if len(res.output) > 0 {
			log.Printf("[ip-ssh] %s (mikrotik): full output: %s", host, res.output)
		} else {
			log.Printf("[ip-ssh] %s (mikrotik): ⚠ empty output!", host)
		}
		return res.output, nil
	case <-ctx.Done():
		log.Printf("[ip-ssh] %s (mikrotik): ✘ TIMEOUT after %s", host, timeout)
		client.Close()
		return "", fmt.Errorf("timeout after %s: command never returned", timeout)
	}
}

// MikrotikNocPassSSH connects with the admin username as entered (no +cet511w4098h IP-workflow suffix).
// Uses a longer TCP dial timeout than IP backups (helps slow links; override with NOCPASS_SSH_DIAL_TIMEOUT_SEC).
func MikrotikNocPassSSH(host, user, pass string, cmds ...string) (string, error) {
	return MikrotikNocPassSSHContext(context.Background(), host, user, pass, cmds...)
}

func MikrotikNocPassSSHContext(ctx context.Context, host, user, pass string, cmds ...string) (string, error) {
	profile, ok := vendorProfiles["mikrotik"]
	if !ok {
		return "", fmt.Errorf("mikrotik profile missing")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	host = strings.TrimSpace(host)
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	addr := JoinSSHAddr(host)
	if addr == "" {
		return "", fmt.Errorf("empty host")
	}

	dialTimeout := 120 * time.Second
	if v := os.Getenv("NOCPASS_SSH_DIAL_TIMEOUT_SEC"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 5 && n <= 600 {
			dialTimeout = time.Duration(n) * time.Second
		}
	}

	log.Printf("[noc-pass-mikrotik] %s: dial %s user=%q dialTimeout=%s cmds=%d", host, addr, user, dialTimeout, len(cmds))

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pass
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         dialTimeout,
		Config:          wideSSHConfig(),
	}

	log.Printf("[noc-pass-mikrotik] %s: TCP connect...", host)
	dialer := &net.Dialer{Timeout: dialTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		log.Printf("[noc-pass-mikrotik] %s: ✘ TCP dial failed: %v", host, err)
		return "", fmt.Errorf("tcp dial: %w", err)
	}
	log.Printf("[noc-pass-mikrotik] %s: ✔ TCP connected to %s — SSH handshake...", host, addr)

	type handshakeResult struct {
		conn  ssh.Conn
		chans <-chan ssh.NewChannel
		reqs  <-chan *ssh.Request
		err   error
	}
	handshakeCh := make(chan handshakeResult, 1)
	go func() {
		var sshConn ssh.Conn
		var chans <-chan ssh.NewChannel
		var reqs <-chan *ssh.Request
		err := withWeakRSAHostKeySupport(func() error {
			var handshakeErr error
			sshConn, chans, reqs, handshakeErr = ssh.NewClientConn(conn, addr, cfg)
			return handshakeErr
		})
		handshakeCh <- handshakeResult{conn: sshConn, chans: chans, reqs: reqs, err: err}
	}()

	var sshConn ssh.Conn
	var chans <-chan ssh.NewChannel
	var reqs <-chan *ssh.Request
	select {
	case res := <-handshakeCh:
		if res.err != nil {
			conn.Close()
			log.Printf("[noc-pass-mikrotik] %s: ✘ SSH handshake failed: %v", host, res.err)
			return "", fmt.Errorf("ssh handshake: %w", res.err)
		}
		sshConn = res.conn
		chans = res.chans
		reqs = res.reqs
	case <-ctx.Done():
		conn.Close()
		return "", ctx.Err()
	}

	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()
	log.Printf("[noc-pass-mikrotik] %s: ✔ SSH connected", host)

	fullCmd := strings.Join(cmds, "; ")
	if len(cmds) > 1 {
		parts := make([]string, 0, len(cmds)*2)
		for _, cmd := range cmds {
			trimmed := strings.TrimSpace(cmd)
			if trimmed == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf(":put %q", trimmed))
			parts = append(parts, trimmed)
		}
		fullCmd = strings.Join(parts, "; ")
	}
	execTimeout := profile.TimeoutOps

	runCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)

	go func() {
		session, sErr := client.NewSession()
		if sErr != nil {
			ch <- result{"", fmt.Errorf("new session: %w", sErr)}
			return
		}
		defer session.Close()

		var stdout, stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr

		log.Printf("[noc-pass-mikrotik] %s: exec: %q", host, fullCmd)
		runErr := session.Run(fullCmd)
		out := stdout.String()
		if runErr != nil && out == "" {
			errStr := stderr.String()
			if errStr != "" {
				log.Printf("[noc-pass-mikrotik] %s: stderr: %s", host, errStr)
			}
			ch <- result{"", fmt.Errorf("exec: %w", runErr)}
			return
		}
		ch <- result{out, nil}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		return res.output, nil
	case <-runCtx.Done():
		client.Close()
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("timeout after %s: command never returned", execTimeout)
	}
}

// NocPassSendCommand uses plain-username Mikrotik SSH for NOC Pass; Cisco IOS/Nexus use scrapligo via IPSendCommand.
func NocPassSendCommand(host, user, pass, vendor string, cmds ...string) (string, error) {
	host = strings.TrimSpace(host)
	user = strings.TrimSpace(user)
	pass = strings.TrimSpace(pass)
	if strings.EqualFold(strings.TrimSpace(vendor), "mikrotik") {
		return MikrotikNocPassSSH(host, user, pass, cmds...)
	}
	return IPSendCommand(host, user, pass, vendor, cmds...)
}

func huaweiWorkflowSSH(host, user, pass string, cmds ...string) (string, error) {
	addr := host + ":22"
	log.Printf("[hw-bng] %s: ═══ START SESSION ═══", host)
	log.Printf("[hw-bng] %s: user=%q cmds=%v", host, user, cmds)

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				log.Printf("[hw-bng] %s: keyboard-interactive challenge: name=%q instruction=%q questions=%v", host, name, instruction, questions)
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = pass
				}
				return answers, nil
			}),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         60 * time.Second,
		Config:          wideSSHConfig(),
	}

	log.Printf("[hw-bng] %s: dialing TCP %s ...", host, addr)
	tcpConn, err := net.DialTimeout("tcp", addr, cfg.Timeout)
	if err != nil {
		log.Printf("[hw-bng] %s: ✘ TCP dial failed: %v", host, err)
		return "", fmt.Errorf("tcp dial: %w", err)
	}
	log.Printf("[hw-bng] %s: ✔ TCP connected", host)

	log.Printf("[hw-bng] %s: SSH handshake starting...", host)
	var sshConn ssh.Conn
	var chans <-chan ssh.NewChannel
	var reqs <-chan *ssh.Request
	err = withWeakRSAHostKeySupport(func() error {
		var handshakeErr error
		sshConn, chans, reqs, handshakeErr = ssh.NewClientConn(tcpConn, addr, cfg)
		return handshakeErr
	})
	if err != nil {
		tcpConn.Close()
		log.Printf("[hw-bng] %s: ✘ SSH handshake FAILED: %v", host, err)
		return "", fmt.Errorf("ssh handshake: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer func() {
		client.Close()
		log.Printf("[hw-bng] %s: SSH connection closed", host)
	}()
	log.Printf("[hw-bng] %s: ✔ SSH authenticated", host)

	session, err := client.NewSession()
	if err != nil {
		log.Printf("[hw-bng] %s: ✘ new session failed: %v", host, err)
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	log.Printf("[hw-bng] %s: requesting PTY (xterm 512x40)...", host)
	if err := session.RequestPty("xterm", 40, 512, modes); err != nil {
		log.Printf("[hw-bng] %s: ✘ PTY request failed: %v", host, err)
		return "", fmt.Errorf("request pty: %w", err)
	}
	log.Printf("[hw-bng] %s: ✔ PTY allocated", host)

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		log.Printf("[hw-bng] %s: ✘ stdin pipe failed: %v", host, err)
		return "", fmt.Errorf("stdin pipe: %w", err)
	}
	var outBuf safeBuffer
	session.Stdout = &outBuf
	session.Stderr = &outBuf

	log.Printf("[hw-bng] %s: starting shell...", host)
	if err := session.Shell(); err != nil {
		log.Printf("[hw-bng] %s: ✘ shell start failed: %v", host, err)
		return "", fmt.Errorf("shell: %w", err)
	}
	log.Printf("[hw-bng] %s: ✔ shell started — waiting for initial prompt...", host)

	// Huawei BNG prompt is <HOSTNAME> — never match standalone # (config separator)
	initialPromptRe := regexp.MustCompile(`<[A-Za-z0-9._-]+>\s*$`)
	waitForBNGPrompt := func(label string, promptRe *regexp.Regexp, timeout time.Duration) error {
		deadline := time.After(timeout)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		lastLen := 0
		stableCount := 0
		lastHandledRawLen := 0
		continueAnswered := false
		for {
			select {
			case <-deadline:
				snippet := outBuf.String()
				if len(snippet) > 500 {
					snippet = snippet[len(snippet)-500:]
				}
				log.Printf("[hw-bng] %s: ✘ TIMEOUT waiting for prompt (%s) — last 500 chars:\n%s", host, label, snippet)
				return fmt.Errorf("timeout waiting for prompt (%s)", label)
			case <-tick.C:
				raw := outBuf.Bytes()
				curLen := len(raw)
				if curLen == 0 {
					continue
				}

				if curLen > lastHandledRawLen {
					windowStart := lastHandledRawLen
					if windowStart > 128 {
						windowStart -= 128
					} else {
						windowStart = 0
					}
					fresh := raw[windowStart:]
					cleanFresh := ansiRe.ReplaceAll(fresh, nil)

					if huaweiWorkflowMoreRe.Match(cleanFresh) {
						log.Printf("[hw-bng] %s: flushing pager (%s)", host, label)
						stdinPipe.Write([]byte(" "))
						lastHandledRawLen = curLen
						lastLen = 0
						stableCount = 0
						time.Sleep(300 * time.Millisecond)
						continue
					}

					if !continueAnswered && huaweiWorkflowContinueRe.Match(cleanFresh) {
						log.Printf("[hw-bng] %s: answering continue prompt with y (%s)", host, label)
						stdinPipe.Write([]byte("y\r\n"))
						continueAnswered = true
						lastHandledRawLen = curLen
						lastLen = 0
						stableCount = 0
						time.Sleep(500 * time.Millisecond)
						continue
					}
				}

				// Handle pager seen in the full buffer too, in case ANSI/control bytes split the match.
				clean := ansiRe.ReplaceAll(raw, nil)
				if huaweiWorkflowMoreRe.Match(clean) {
					log.Printf("[hw-bng] %s: flushing pager (%s)", host, label)
					stdinPipe.Write([]byte(" "))
					lastHandledRawLen = curLen
					lastLen = 0
					stableCount = 0
					time.Sleep(300 * time.Millisecond)
					continue
				}

				// Check if output is still growing
				if curLen != lastLen {
					lastLen = curLen
					stableCount = 0
					continue
				}
				stableCount++

				// Only check prompt after output has been stable for ~400ms (2 ticks)
				if stableCount < 2 {
					continue
				}

				if promptRe.Match(clean) {
					log.Printf("[hw-bng] %s: ✔ prompt detected (%s) after %d bytes stable", host, label, curLen)
					return nil
				}
			}
		}
	}

	if err := waitForBNGPrompt("initial", initialPromptRe, 60*time.Second); err != nil {
		return "", err
	}
	initialOut := outBuf.String()
	log.Printf("[hw-bng] %s: initial output:\n%s", host, initialOut)

	// Extract hostname from prompt <HOSTNAME> for precise matching
	hostPromptRe := initialPromptRe
	hnRe := regexp.MustCompile(`<([A-Za-z0-9._-]+)>`)
	if m := hnRe.FindStringSubmatch(initialOut); len(m) > 1 {
		escaped := regexp.QuoteMeta(m[1])
		hostPromptRe = regexp.MustCompile(`<` + escaped + `>\s*$`)
		log.Printf("[hw-bng] %s: captured hostname=%q — prompt regex: %s", host, m[1], hostPromptRe.String())
	}
	outBuf.Reset()

	// Disable paging completely for long Huawei outputs such as elabel/current-configuration.
	log.Printf("[hw-bng] %s: >>> screen-length 0 temporary", host)
	stdinPipe.Write([]byte("screen-length 0 temporary\r\n"))
	time.Sleep(500 * time.Millisecond)
	if err := waitForBNGPrompt("screen-length", hostPromptRe, 30*time.Second); err != nil {
		log.Printf("[hw-bng] %s: screen-length response: %s", host, outBuf.String())
	} else {
		log.Printf("[hw-bng] %s: ✔ paging disabled", host)
	}
	outBuf.Reset()

	var fullOutput strings.Builder
	for i, cmd := range cmds {
		outBuf.Reset()
		log.Printf("[hw-bng] %s: >>> [%d/%d] %s", host, i+1, len(cmds), cmd)
		if _, err := stdinPipe.Write([]byte(cmd + "\r\n")); err != nil {
			log.Printf("[hw-bng] %s: ✘ write failed: %v", host, err)
			return fullOutput.String(), fmt.Errorf("write cmd: %w", err)
		}

		if err := waitForBNGPrompt(fmt.Sprintf("cmd-%d", i+1), hostPromptRe, 10*time.Minute); err != nil {
			partial := outBuf.String()
			log.Printf("[hw-bng] %s: ✘ cmd %d timed out — got %d bytes", host, i+1, len(partial))
			fullOutput.WriteString(partial)
			return fullOutput.String(), err
		}

		result := outBuf.String()
		log.Printf("[hw-bng] %s: ✔ cmd %d done — %d bytes", host, i+1, len(result))
		if len(result) > 300 {
			log.Printf("[hw-bng] %s: output preview:\n%s\n...\n%s", host, result[:150], result[len(result)-150:])
		} else if len(result) > 0 {
			log.Printf("[hw-bng] %s: output:\n%s", host, result)
		}
		fullOutput.WriteString(result)
	}

	total := fullOutput.String()
	log.Printf("[hw-bng] %s: ═══ SESSION COMPLETE — total %d bytes ═══", host, len(total))
	return total, nil
}

func IPSendCommand(host, user, pass, vendor string, cmds ...string) (string, error) {
	profile, ok := vendorProfiles[strings.ToLower(vendor)]
	if !ok {
		return "", fmt.Errorf("unknown vendor %q", vendor)
	}

	if profile.UseRawSSH {
		v := strings.ToLower(vendor)
		if v == "huawei" {
			return huaweiWorkflowSSH(host, user, pass, cmds...)
		}
		return mikrotikSSH(host, user, pass, profile.TimeoutOps, cmds...)
	}

	sshUser := user + profile.UsernameSuffix

	var tp string
	switch profile.TransportType {
	case "system":
		tp = transport.SystemTransport
	default:
		tp = transport.StandardTransport
	}

	rawLegacyFallbackAllowed := strings.EqualFold(vendor, "cisco") || strings.EqualFold(vendor, "nexus")

	buildDriver := func(transportType string) (*generic.Driver, error) {
		log.Printf("[ip-ssh] %s (%s): creating driver — user=%q transport=%s timeout=%s prompt=%s",
			host, vendor, sshUser, transportType, profile.TimeoutOps, profile.PromptPattern)

		opts := []util.Option{
			options.WithAuthNoStrictKey(),
			options.WithAuthUsername(sshUser),
			options.WithAuthPassword(pass),
			options.WithPasswordPattern(legacySystemSSHPasswordPrompt),
			options.WithPromptPattern(profile.PromptPattern),
			options.WithTransportType(transportType),
			options.WithTimeoutSocket(60 * time.Second),
			options.WithStandardTransportExtraKexs(scrapligoWideKEX),
			options.WithStandardTransportExtraCiphers(scrapligoWideCiphers),
			options.WithTimeoutOps(profile.TimeoutOps),
			options.WithTermWidth(511),
		}
		return generic.NewDriver(host, opts...)
	}

	runSession := func(transportType string) (string, error) {
		d, err := buildDriver(transportType)
		if err != nil {
			log.Printf("[ip-ssh] %s (%s): ✘ driver init failed: %v", host, vendor, err)
			return "", fmt.Errorf("driver init: %w", err)
		}
		defer d.Close()

		log.Printf("[ip-ssh] %s (%s): opening SSH connection...", host, vendor)
		if err := d.Open(); err != nil {
			log.Printf("[ip-ssh] %s (%s): ✘ SSH open failed: %v", host, vendor, err)
			return "", fmt.Errorf("ssh open: %w", err)
		}
		log.Printf("[ip-ssh] %s (%s): ✔ SSH connected", host, vendor)

		for _, setup := range profile.SetupCmds {
			log.Printf("[ip-ssh] %s (%s): running setup cmd: %q", host, vendor, setup)
			_, setupErr := d.SendCommand(setup)
			if setupErr != nil {
				log.Printf("[ip-ssh] %s (%s): setup cmd %q error: %v", host, vendor, setup, setupErr)
			} else {
				log.Printf("[ip-ssh] %s (%s): setup cmd %q OK", host, vendor, setup)
			}
		}

		out, err := sendIPCommandsWithTimeout(d, host, vendor, profile.TimeoutOps+30*time.Second, cmds...)
		if err != nil {
			return "", err
		}
		logIPOutputPreview(host, vendor, out)
		return out, nil
	}

	out, err := runSession(tp)
	if err == nil || !rawLegacyFallbackAllowed || tp == transport.SystemTransport {
		return out, err
	}
	if !shouldRetryCiscoWithLegacyFallback(err) {
		return "", err
	}

	log.Printf("[ip-ssh] %s (%s): retrying through raw SSH legacy fallback after standard transport error: %v", host, vendor, err)
	fallbackOut, fallbackErr := ciscoLegacyRawSSH(host, sshUser, pass, vendor, profile, cmds...)
	if fallbackErr != nil {
		return "", fmt.Errorf("standard transport failed: %v; raw SSH legacy fallback failed: %w", err, fallbackErr)
	}
	return fallbackOut, nil
}

func sendIPCommandsWithTimeout(d *generic.Driver, host, vendor string, timeout time.Duration, cmds ...string) (string, error) {
	type result struct {
		output string
		err    error
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ch := make(chan result, 1)
	go func() {
		if len(cmds) == 1 {
			log.Printf("[ip-ssh] %s (%s): sending command: %q", host, vendor, cmds[0])
			r, cmdErr := d.SendCommand(cmds[0])
			if cmdErr != nil {
				log.Printf("[ip-ssh] %s (%s): ✘ command failed: %v", host, vendor, cmdErr)
				ch <- result{"", cmdErr}
				return
			}
			log.Printf("[ip-ssh] %s (%s): ✔ command done — %d bytes", host, vendor, len(r.Result))
			ch <- result{r.Result, nil}
			return
		}

		// One SendCommand per line: batched SendCommands can hit "channel timeout sending input"
		// on IOS when write memory or config mode is slow — each call waits for prompt before the next.
		log.Printf("[ip-ssh] %s (%s): sending %d commands sequentially: %v", host, vendor, len(cmds), cmds)
		var out strings.Builder
		for i, cmd := range cmds {
			log.Printf("[ip-ssh] %s (%s): cmd %d/%d: %q", host, vendor, i+1, len(cmds), cmd)
			r, cmdErr := d.SendCommand(cmd)
			if cmdErr != nil {
				log.Printf("[ip-ssh] %s (%s): ✘ cmd %d/%d failed: %v", host, vendor, i+1, len(cmds), cmdErr)
				ch <- result{out.String(), cmdErr}
				return
			}
			log.Printf("[ip-ssh] %s (%s): ✔ cmd %d/%d OK — %d bytes", host, vendor, i+1, len(cmds), len(r.Result))
			out.WriteString(r.Result)
		}
		joined := out.String()
		log.Printf("[ip-ssh] %s (%s): ✔ all commands done — %d bytes", host, vendor, len(joined))
		ch <- result{joined, nil}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		return res.output, nil
	case <-ctx.Done():
		log.Printf("[ip-ssh] %s (%s): ✘ TIMEOUT after %s — command hung, closing connection", host, vendor, timeout)
		return "", fmt.Errorf("timeout after %s: command never returned", timeout)
	}
}

func logIPOutputPreview(host, vendor, output string) {
	log.FileOnlyf(
		"[ip-ssh] %s (%s): full_output_begin\n%s\n[ip-ssh] %s (%s): full_output_end",
		host,
		vendor,
		output,
		host,
		vendor,
	)
	if len(output) > 200 {
		log.Printf("[ip-ssh] %s (%s): output preview: %s...", host, vendor, output[:200])
		return
	}
	if len(output) > 0 {
		log.Printf("[ip-ssh] %s (%s): full output: %s", host, vendor, output)
		return
	}
	log.Printf("[ip-ssh] %s (%s): ⚠ empty output!", host, vendor)
}

func shouldRetryCiscoWithLegacyFallback(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "ssh open") ||
		strings.Contains(lower, "handshake") ||
		strings.Contains(lower, "unable to authenticate") ||
		strings.Contains(lower, "no supported methods remain") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "password prompt seen multiple times") ||
		strings.Contains(lower, "channel timeout") ||
		strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "errconnectionerror") ||
		strings.Contains(lower, "connection reset") ||
		strings.Contains(lower, "connection refused") ||
		strings.Contains(lower, "connection closed") ||
		strings.Contains(lower, "eof") ||
		strings.Contains(lower, "kex") ||
		strings.Contains(lower, "cipher") ||
		strings.Contains(lower, "host key")
}

func IPBackupCommand(vendor string) (string, error) {
	profile, ok := vendorProfiles[strings.ToLower(vendor)]
	if !ok {
		return "", fmt.Errorf("unknown vendor %q", vendor)
	}
	return profile.BackupCmd, nil
}

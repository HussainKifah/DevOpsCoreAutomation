package shell

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/scrapli/scrapligo/driver/generic"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/transport"
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
	"cisco": {
		PromptPattern: regexp.MustCompile(`(?m)[a-zA-Z0-9._\-]+[#>]\s*$`),
		SetupCmds:     []string{"terminal length 0", "terminal width 0"},
		BackupCmd:     "show running-config",
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
		Timeout:         30 * time.Second,
	}

	log.Printf("[ip-ssh] %s (mikrotik): dialing %s ...", host, addr)
	conn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		log.Printf("[ip-ssh] %s (mikrotik): ✘ TCP dial failed: %v", host, err)
		return "", fmt.Errorf("tcp dial: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
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
		Timeout:         30 * time.Second,
		Config: ssh.Config{
			KeyExchanges: []string{
				"diffie-hellman-group-exchange-sha1",
				"diffie-hellman-group1-sha1",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group14-sha256",
			},
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-cbc", "3des-cbc",
			},
		},
	}

	log.Printf("[hw-bng] %s: dialing TCP %s ...", host, addr)
	tcpConn, err := net.DialTimeout("tcp", addr, 30*time.Second)
	if err != nil {
		log.Printf("[hw-bng] %s: ✘ TCP dial failed: %v", host, err)
		return "", fmt.Errorf("tcp dial: %w", err)
	}
	log.Printf("[hw-bng] %s: ✔ TCP connected", host)

	log.Printf("[hw-bng] %s: SSH handshake starting...", host)
	sshConn, chans, reqs, err := ssh.NewClientConn(tcpConn, addr, cfg)
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
	moreRe := regexp.MustCompile(`(?i)(---- More|--More--|<--- More --->)`)

	waitForBNGPrompt := func(label string, promptRe *regexp.Regexp, timeout time.Duration) error {
		deadline := time.After(timeout)
		tick := time.NewTicker(200 * time.Millisecond)
		defer tick.Stop()
		lastLen := 0
		stableCount := 0
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

				// Handle pager
				if moreRe.Match(raw) {
					log.Printf("[hw-bng] %s: flushing pager (%s)", host, label)
					stdinPipe.Write([]byte(" "))
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

				clean := ansiRe.ReplaceAll(raw, nil)
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

	// Disable paging
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

	log.Printf("[ip-ssh] %s (%s): creating driver — user=%q transport=%s timeout=%s prompt=%s",
		host, vendor, sshUser, tp, profile.TimeoutOps, profile.PromptPattern)

	d, err := generic.NewDriver(
		host,
		options.WithAuthNoStrictKey(),
		options.WithAuthUsername(sshUser),
		options.WithAuthPassword(pass),
		options.WithPromptPattern(profile.PromptPattern),
		options.WithTransportType(tp),
		options.WithTimeoutOps(profile.TimeoutOps),
		options.WithTermWidth(511),
	)
	if err != nil {
		log.Printf("[ip-ssh] %s (%s): ✘ driver init failed: %v", host, vendor, err)
		return "", fmt.Errorf("driver init: %w", err)
	}

	log.Printf("[ip-ssh] %s (%s): opening SSH connection...", host, vendor)
	if err := d.Open(); err != nil {
		log.Printf("[ip-ssh] %s (%s): ✘ SSH open failed: %v", host, vendor, err)
		return "", fmt.Errorf("ssh open: %w", err)
	}
	defer d.Close()
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

	type result struct {
		output string
		err    error
	}
	timeout := profile.TimeoutOps + 30*time.Second
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
		} else {
			log.Printf("[ip-ssh] %s (%s): sending %d commands: %v", host, vendor, len(cmds), cmds)
			rs, cmdErr := d.SendCommands(cmds)
			if cmdErr != nil {
				log.Printf("[ip-ssh] %s (%s): ✘ commands failed: %v", host, vendor, cmdErr)
				ch <- result{"", cmdErr}
				return
			}
			out := rs.JoinedResult()
			log.Printf("[ip-ssh] %s (%s): ✔ commands done — %d bytes", host, vendor, len(out))
			ch <- result{out, nil}
		}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			return "", res.err
		}
		if len(res.output) > 200 {
			log.Printf("[ip-ssh] %s (%s): output preview: %s...", host, vendor, res.output[:200])
		} else if len(res.output) > 0 {
			log.Printf("[ip-ssh] %s (%s): full output: %s", host, vendor, res.output)
		} else {
			log.Printf("[ip-ssh] %s (%s): ⚠ empty output!", host, vendor)
		}
		return res.output, nil
	case <-ctx.Done():
		log.Printf("[ip-ssh] %s (%s): ✘ TIMEOUT after %s — command hung, closing connection", host, vendor, timeout)
		return "", fmt.Errorf("timeout after %s: command never returned", timeout)
	}
}

func IPBackupCommand(vendor string) (string, error) {
	profile, ok := vendorProfiles[strings.ToLower(vendor)]
	if !ok {
		return "", fmt.Errorf("unknown vendor %q", vendor)
	}
	return profile.BackupCmd, nil
}

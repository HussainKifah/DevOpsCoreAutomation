package shell

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	log "github.com/Flafl/DevOpsCore/utils"
	"golang.org/x/crypto/ssh"
)

var legacyCiscoMoreRe = regexp.MustCompile(`(?i)--more--|<--- more --->|\bmore\b`)
var legacyCiscoConfirmRe = regexp.MustCompile(`(?im)\[(confirm|yes/no)\]\s*$`)

func ciscoLegacyRawSSH(host, user, pass, vendor string, profile VendorProfile, cmds ...string) (string, error) {
	addr := JoinSSHAddr(host)
	if addr == "" {
		return "", fmt.Errorf("empty host")
	}

	log.Printf("[ip-ssh] %s (%s): starting raw SSH legacy fallback user=%q cmds=%d", host, vendor, user, len(cmds))

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
		HostKeyCallback:   ssh.InsecureIgnoreHostKey(),
		HostKeyAlgorithms: wideSSHHostKeyAlgorithms(),
		Timeout:           60 * time.Second,
		Config:            wideSSHConfig(),
		BannerCallback: func(message string) error {
			log.FileOnlyf("[ip-ssh] %s (%s): legacy_raw_banner\n%s", host, vendor, strings.TrimSpace(message))
			return nil
		},
	}

	var client *ssh.Client
	err := withWeakRSAHostKeySupport(func() error {
		var dialErr error
		client, dialErr = ssh.Dial("tcp", addr, cfg)
		return dialErr
	})
	if err != nil {
		log.Printf("[ip-ssh] %s (%s): raw SSH legacy fallback dial failed: %v", host, vendor, err)
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()
	log.Printf("[ip-ssh] %s (%s): raw SSH legacy fallback connected", host, vendor)

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          0,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := session.RequestPty("xterm", 40, 512, modes); err != nil {
		return "", fmt.Errorf("request pty: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	var outBuf safeBuffer
	session.Stdout = &outBuf
	session.Stderr = &outBuf

	if err := session.Shell(); err != nil {
		return "", fmt.Errorf("shell: %w", err)
	}

	sessionCtx, cancel := context.WithTimeout(context.Background(), profile.TimeoutOps+30*time.Second)
	defer cancel()

	if err := waitForCiscoLegacyPrompt(sessionCtx, &outBuf, stdin, host, profile.PromptPattern); err != nil {
		return "", fmt.Errorf("wait initial prompt: %w", err)
	}

	allCmds := make([]string, 0, len(profile.SetupCmds)+len(cmds))
	allCmds = append(allCmds, profile.SetupCmds...)
	allCmds = append(allCmds, cmds...)

	var fullOutput strings.Builder
	for i, cmd := range allCmds {
		trimmedCmd := strings.TrimSpace(cmd)
		if trimmedCmd == "" {
			continue
		}

		isSetup := i < len(profile.SetupCmds)
		if isSetup {
			log.Printf("[ip-ssh] %s (%s): raw legacy setup cmd: %q", host, vendor, trimmedCmd)
		} else {
			log.Printf("[ip-ssh] %s (%s): raw legacy cmd %d/%d: %q", host, vendor, i-len(profile.SetupCmds)+1, len(cmds), trimmedCmd)
		}

		outBuf.Reset()
		if _, err := io.WriteString(stdin, trimmedCmd+"\r\n"); err != nil {
			return fullOutput.String(), fmt.Errorf("write %q: %w", trimmedCmd, err)
		}

		cmdCtx, cmdCancel := context.WithTimeout(sessionCtx, legacyCiscoPromptTimeout(trimmedCmd))
		waitErr := waitForCiscoLegacyPrompt(cmdCtx, &outBuf, stdin, host, profile.PromptPattern)
		cmdCancel()

		rawOutput := outBuf.String()
		cleanOutput := cleanupLegacyCiscoShellOutput(rawOutput, trimmedCmd, profile.PromptPattern)
		if isSetup {
			if waitErr != nil {
				return fullOutput.String(), fmt.Errorf("setup %q: %w", trimmedCmd, waitErr)
			}
			log.Printf("[ip-ssh] %s (%s): raw legacy setup cmd %q OK", host, vendor, trimmedCmd)
			continue
		}

		fullOutput.WriteString(cleanOutput)
		if waitErr != nil {
			return fullOutput.String(), fmt.Errorf("command %q: %w", trimmedCmd, waitErr)
		}
	}

	out := fullOutput.String()
	log.Printf("[ip-ssh] %s (%s): raw SSH legacy fallback completed (%d bytes)", host, vendor, len(out))
	logIPOutputPreview(host, vendor, out)
	return out, nil
}

func waitForCiscoLegacyPrompt(ctx context.Context, buf *safeBuffer, stdin io.Writer, host string, promptRe *regexp.Regexp) error {
	poll := time.NewTicker(40 * time.Millisecond)
	defer poll.Stop()

	lastHandledRawLen := 0
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-poll.C:
			raw := buf.Bytes()
			if len(raw) == 0 {
				continue
			}

			data := sanitizeLegacyCiscoShellOutput(raw)
			if promptRe != nil && promptRe.MatchString(data) {
				return nil
			}

			rawLen := len(raw)
			if rawLen <= lastHandledRawLen {
				continue
			}

			fresh := sanitizeLegacyCiscoShellOutput(raw[lastHandledRawLen:])
			if stdin != nil && legacyCiscoMoreRe.MatchString(fresh) {
				if _, err := io.WriteString(stdin, " "); err == nil {
					log.Printf("[ip-ssh] %s: raw legacy fallback flushing pager", host)
				}
				lastHandledRawLen = rawLen
				time.Sleep(140 * time.Millisecond)
				continue
			}
			if stdin != nil && legacyCiscoConfirmRe.MatchString(data) {
				if _, err := io.WriteString(stdin, "\r\n"); err == nil {
					log.Printf("[ip-ssh] %s: raw legacy fallback answering confirm prompt", host)
				}
				lastHandledRawLen = rawLen
				time.Sleep(140 * time.Millisecond)
				continue
			}
		}
	}
}

func cleanupLegacyCiscoShellOutput(out, cmd string, promptRe *regexp.Regexp) string {
	clean := sanitizeLegacyCiscoShellOutput([]byte(out))
	lines := strings.Split(clean, "\n")
	filtered := make([]string, 0, len(lines))
	trimmedCmd := strings.TrimSpace(cmd)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == trimmedCmd {
			continue
		}
		if promptRe != nil && promptRe.MatchString(trimmed) {
			continue
		}
		if legacyCiscoMoreRe.MatchString(trimmed) || legacyCiscoConfirmRe.MatchString(trimmed) {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, "\n") + "\n"
}

func sanitizeLegacyCiscoShellOutput(data []byte) string {
	clean := ansiRe.ReplaceAll(data, nil)
	var builder strings.Builder
	for _, b := range clean {
		if b == '\r' || b == '\n' || b == '\t' || (b >= 32 && b <= 126) {
			builder.WriteByte(b)
		}
	}
	return strings.TrimSpace(builder.String())
}

func legacyCiscoPromptTimeout(cmd string) time.Duration {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	switch {
	case strings.Contains(lower, "show logging"),
		strings.Contains(lower, "show running-config"),
		strings.Contains(lower, "write memory"),
		strings.Contains(lower, "copy running-config startup-config"):
		return 90 * time.Second
	case strings.Contains(lower, "show version"),
		strings.Contains(lower, "show int status"):
		return 45 * time.Second
	default:
		return 30 * time.Second
	}
}

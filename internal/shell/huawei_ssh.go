package shell

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// safeBuffer is a thread-safe bytes.Buffer. The SSH session's stdout goroutine
// writes to it concurrently while we read/reset from the main goroutine.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *safeBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]byte, b.buf.Len())
	copy(cp, b.buf.Bytes())
	return cp
}

func (b *safeBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

// Prompt matches only at end-of-buffer (no (?m) — prevents false match on
// intermediate "MA5600T(config)#interface gpon..." echoed lines).
var hwPromptRe = regexp.MustCompile(`(\)#|\w#|>)\s*$`)
var hwMoreRe = regexp.MustCompile(`---- More`)
var hwParamPromptRe = regexp.MustCompile(`\}\s*:\s*$`)
var ansiRe = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

// hwCommandSleep is the delay after each command completes (prompt seen). Prompt
// detection already waits for full output; this is only device/line pacing.
func hwCommandSleep() time.Duration {
	ms := 80
	if v := os.Getenv("HW_POWER_SLEEP_MS"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 && n <= 2000 {
			ms = n
		}
	}
	return time.Duration(ms) * time.Millisecond
}

// hwCommandPreGap is a short pause before sending each command (stdin/echo settle).
func hwCommandPreGap() time.Duration {
	ms := 12
	if v := os.Getenv("HW_POWER_CMD_GAP_MS"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n >= 0 && n <= 500 {
			ms = n
		}
	}
	return time.Duration(ms) * time.Millisecond
}

var hwSSHConfig = ssh.Config{
	KeyExchanges: []string{
		ssh.InsecureKeyExchangeDHGEXSHA1,
		ssh.InsecureKeyExchangeDH1SHA1,
		ssh.InsecureKeyExchangeDH14SHA1,
		ssh.KeyExchangeDH14SHA256,
	},
	Ciphers: []string{
		ssh.CipherAES128CTR, ssh.CipherAES192CTR, ssh.CipherAES256CTR,
		ssh.InsecureCipherAES128CBC, ssh.InsecureCipherTripleDESCBC,
	},
	MACs: []string{
		ssh.HMACSHA256, ssh.HMACSHA512, ssh.HMACSHA1, ssh.InsecureHMACSHA196,
	},
}

type HwSession struct {
	host    string
	client  *ssh.Client
	session *ssh.Session
	stdin   io.Writer
	outBuf  *safeBuffer
	ctx     context.Context
	cancel  context.CancelFunc
}

func HwOpenSession(host, user, pass string) (*HwSession, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
		Config:          hwSSHConfig,
	}
	addr := host
	if !strings.Contains(host, ":") {
		addr = net.JoinHostPort(host, "22")
	}
	log.Printf("[%s] SSH connecting...", host)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("ssh dial: %w", err)
	}
	session, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("new session: %w", err)
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := session.RequestPty("xterm", 120, 40, modes); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("request pty: %w", err)
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	var outBuf safeBuffer
	session.Stdout = &outBuf
	session.Stderr = &outBuf
	if err := session.Shell(); err != nil {
		session.Close()
		client.Close()
		return nil, fmt.Errorf("shell: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Minute)
	initCtx, initCancel := context.WithTimeout(ctx, 90*time.Second)
	if err := waitForPrompt(initCtx, &outBuf); err != nil {
		initCancel()
		session.Close()
		client.Close()
		cancel()
		return nil, fmt.Errorf("wait initial prompt: %w", err)
	}
	initCancel()
	outBuf.Reset()
	log.Printf("[%s] session opened", host)
	return &HwSession{
		host: host, client: client, session: session,
		stdin: stdin, outBuf: &outBuf, ctx: ctx, cancel: cancel,
	}, nil
}

func (s *HwSession) SendCommands(cmds ...string) (string, error) {
	sleep := hwCommandSleep()
	gap := hwCommandPreGap()
	var fullOut strings.Builder
	for _, cmd := range cmds {
		if gap > 0 {
			time.Sleep(gap)
		}
		s.outBuf.Reset()

		log.Printf("[%s] >>> %s", s.host, cmd)
		if _, err := s.stdin.Write([]byte(cmd + "\r\n")); err != nil {
			return fullOut.String(), fmt.Errorf("write %q: %w", cmd, err)
		}
		cmdCtx, cmdCancel := context.WithTimeout(s.ctx, 5*time.Minute)
		if err := waitForPromptWithMore(cmdCtx, s.outBuf, s.stdin, s.host); err != nil {
			cmdCancel()
			resp := s.outBuf.String()
			log.Printf("[%s] <<< TIMEOUT\n%s", s.host, resp)
			return fullOut.String() + resp, fmt.Errorf("prompt after %q: %w", cmd, err)
		}
		cmdCancel()
		resp := s.outBuf.String()
		if strings.HasPrefix(cmd, "display ont optical-info") {
			log.Printf("[%s] <<< %s (%d bytes)", s.host, cmd, len(resp))
		} else {
			preview := strings.TrimRight(resp, "\r\n ")
			if len(preview) > 200 {
				preview = preview[:200] + "…"
			}
			log.Printf("[%s] <<< %s", s.host, preview)
		}
		fullOut.WriteString(resp)
		s.outBuf.Reset()
		time.Sleep(sleep)
	}
	return fullOut.String(), nil
}

func (s *HwSession) Close() {
	s.cancel()
	if s.session != nil {
		_ = s.session.Close()
		s.session = nil
	}
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	log.Printf("[%s] session closed", s.host)
}

// HwSendCommandOLT opens a one-shot SSH session, runs commands, and closes.
func HwSendCommandOLT(host, user, pass string, cmds ...string) (string, error) {
	cfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{ssh.Password(pass)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         30 * time.Second,
		Config:          hwSSHConfig,
	}
	addr := host
	if !strings.Contains(host, ":") {
		addr = net.JoinHostPort(host, "22")
	}
	log.Printf("[%s] SSH connecting (one-shot)...", host)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return "", fmt.Errorf("ssh dial: %w", err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("new session: %w", err)
	}
	defer session.Close()

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 115200,
		ssh.TTY_OP_OSPEED: 115200,
	}
	if err := session.RequestPty("xterm", 120, 40, modes); err != nil {
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

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Minute)
	defer cancel()

	initCtx, initCancel := context.WithTimeout(ctx, 90*time.Second)
	if err := waitForPrompt(initCtx, &outBuf); err != nil {
		initCancel()
		return "", fmt.Errorf("wait initial prompt: %w", err)
	}
	initCancel()

	sleep := hwCommandSleep()
	gap := hwCommandPreGap()
	var fullOut strings.Builder
	for _, cmd := range cmds {
		if gap > 0 {
			time.Sleep(gap)
		}
		outBuf.Reset()
		if _, err := stdin.Write([]byte(cmd + "\r\n")); err != nil {
			return fullOut.String(), fmt.Errorf("write %q: %w", cmd, err)
		}
		cmdCtx, cmdCancel := context.WithTimeout(ctx, 5*time.Minute)
		if err := waitForPromptWithMore(cmdCtx, &outBuf, stdin, host); err != nil {
			cmdCancel()
			return fullOut.String() + outBuf.String(), fmt.Errorf("prompt after %q: %w", cmd, err)
		}
		cmdCancel()
		fullOut.WriteString(outBuf.String())
		outBuf.Reset()
		time.Sleep(sleep)
	}
	return fullOut.String(), nil
}

func waitForPrompt(ctx context.Context, buf *safeBuffer) error {
	return waitForPromptWithMore(ctx, buf, nil, "")
}

func waitForPromptWithMore(ctx context.Context, buf *safeBuffer, stdin interface{ Write([]byte) (int, error) }, host string) error {
	poll := time.NewTicker(40 * time.Millisecond)
	defer poll.Stop()
	// Track position in the RAW buffer to avoid re-matching old pager/param text.
	// We use raw length (not ANSI-stripped) because stripping partial ANSI codes
	// at the boundary can produce non-monotonic lengths.
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
			// Prompt check uses ANSI-stripped full data (prompt is always at the very end)
			data := ansiRe.ReplaceAll(raw, nil)
			if hwPromptRe.Match(data) {
				check := data
				if len(check) > 200 {
					check = check[len(check)-200:]
				}
				// Guard against false ">" from { <cr>|pgid<U><0,511> }:
				lastCurlyOpen := bytes.LastIndex(check, []byte("{"))
				lastCurlyClose := bytes.LastIndex(check, []byte("}"))
				if lastCurlyOpen != -1 && (lastCurlyClose == -1 || lastCurlyClose < lastCurlyOpen) {
					goto notPrompt
				}
				// Guard against false ">" from config tags like <global-config> or <gpon-0/4>
				lastAngleOpen := bytes.LastIndex(check, []byte("<"))
				lastAngleClose := bytes.LastIndex(check, []byte(">"))
				if lastAngleOpen != -1 && lastAngleClose > lastAngleOpen {
					// ">" closes a "<" — not a real prompt unless there's a "#" match
					if !bytes.HasSuffix(bytes.TrimRight(data, " \t\r\n"), []byte("#")) {
						goto notPrompt
					}
				}
				return nil
			}
		notPrompt:
			// Pager and param-prompt checks use raw bytes after last handled position
			rawLen := len(raw)
			if rawLen <= lastHandledRawLen {
				continue
			}
			fresh := raw[lastHandledRawLen:]

			if stdin != nil && hwMoreRe.Match(fresh) {
				if _, err := stdin.Write([]byte(" ")); err == nil && host != "" {
					log.Printf("[%s] flushing More pager", host)
				}
				lastHandledRawLen = rawLen
				time.Sleep(140 * time.Millisecond)
				continue
			}
			if stdin != nil && hwParamPromptRe.Match(fresh) {
				if _, err := stdin.Write([]byte("\r\n")); err == nil && host != "" {
					log.Printf("[%s] answering parameter prompt with Enter", host)
				}
				lastHandledRawLen = rawLen
				time.Sleep(140 * time.Millisecond)
				continue
			}
		}
	}
}

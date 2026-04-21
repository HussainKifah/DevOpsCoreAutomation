package shell

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"

	log "github.com/Flafl/DevOpsCore/utils"
)

func NocDataSendCommand(host, user, pass, vendor string, cmds ...string) (string, string, error) {
	return NocDataSendCommandUsingMethodContext(context.Background(), host, user, pass, vendor, "", cmds...)
}

func NocDataSendCommandUsingMethod(host, user, pass, vendor, method string, cmds ...string) (string, string, error) {
	return NocDataSendCommandUsingMethodContext(context.Background(), host, user, pass, vendor, method, cmds...)
}

func NocDataSendCommandUsingMethodContext(ctx context.Context, host, user, pass, vendor, method string, cmds ...string) (string, string, error) {
	host = strings.TrimSpace(host)
	vendor = strings.TrimSpace(vendor)
	if ctx == nil {
		ctx = context.Background()
	}
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "", "auto":
		log.Printf("[noc-data-transport] %s: trying SSH first vendor=%s cmds=%d", host, vendor, len(cmds))
		out, err := nocDataSSHSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err == nil {
			log.Printf("[noc-data-transport] %s: SSH succeeded", host)
			return out, "ssh", nil
		}
		if ctx.Err() != nil {
			return "", "", ctx.Err()
		}
		log.Printf("[noc-data-transport] %s: SSH failed: %v; trying telnet", host, err)

		telnetOut, telnetErr := NocDataTelnetSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if telnetErr == nil {
			log.Printf("[noc-data-transport] %s: telnet succeeded", host)
			return telnetOut, "telnet", nil
		}

		log.Printf("[noc-data-transport] %s: telnet failed: %v", host, telnetErr)
		return "", "", fmt.Errorf("ssh failed: %v; telnet failed: %w", err, telnetErr)
	case "ssh":
		log.Printf("[noc-data-transport] %s: forcing SSH vendor=%s cmds=%d", host, vendor, len(cmds))
		out, err := nocDataSSHSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err != nil {
			log.Printf("[noc-data-transport] %s: forced SSH failed: %v", host, err)
			return "", "", err
		}
		log.Printf("[noc-data-transport] %s: forced SSH succeeded", host)
		return out, "ssh", nil
	case "telnet":
		log.Printf("[noc-data-transport] %s: forcing telnet vendor=%s cmds=%d", host, vendor, len(cmds))
		out, err := NocDataTelnetSendCommandContext(ctx, host, user, pass, vendor, cmds...)
		if err != nil {
			log.Printf("[noc-data-transport] %s: forced telnet failed: %v", host, err)
			return "", "", err
		}
		log.Printf("[noc-data-transport] %s: forced telnet succeeded", host)
		return out, "telnet", nil
	default:
		return "", "", fmt.Errorf("unsupported NOC data transport method %q", method)
	}
}

func NocDataTelnetSendCommand(host, user, pass, vendor string, cmds ...string) (string, error) {
	return NocDataTelnetSendCommandContext(context.Background(), host, user, pass, vendor, cmds...)
}

func NocDataTelnetSendCommandContext(ctx context.Context, host, user, pass, vendor string, cmds ...string) (string, error) {
	host = strings.TrimSpace(host)
	vendor = strings.ToLower(strings.TrimSpace(vendor))
	if ctx == nil {
		ctx = context.Background()
	}
	log.Printf("[noc-data-telnet] %s: dialing telnet vendor=%s cmds=%d", host, vendor, len(cmds))
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(strings.TrimSpace(host), "23"))
	if err != nil {
		log.Printf("[noc-data-telnet] %s: dial failed: %v", host, err)
		return "", fmt.Errorf("dial telnet: %w", err)
	}
	defer conn.Close()
	log.Printf("[noc-data-telnet] %s: dial connected", host)

	s := &nocDataTelnetSession{
		conn:   conn,
		reader: bufio.NewReader(conn),
		vendor: vendor,
		host:   host,
		ctx:    ctx,
	}
	if err := s.login(strings.TrimSpace(user), strings.TrimSpace(pass)); err != nil {
		log.Printf("[noc-data-telnet] %s: login failed: %v", host, err)
		return "", err
	}
	log.Printf("[noc-data-telnet] %s: login succeeded", host)

	switch s.vendor {
	case "cisco", "nexus":
		log.Printf("[noc-data-telnet] %s: sending session setup commands", host)
		if _, err := s.run("terminal length 0"); err != nil {
			return "", err
		}
		if _, err := s.run("terminal width 0"); err != nil {
			return "", err
		}
	}

	parts := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		out, err := s.run(cmd)
		if err != nil {
			return "", err
		}
		parts = append(parts, strings.TrimSpace(cmd)+"\n"+strings.TrimSpace(out))
	}

	return strings.TrimSpace(strings.Join(parts, "\n\n")), nil
}

type nocDataTelnetSession struct {
	conn   net.Conn
	reader *bufio.Reader
	vendor string
	host   string
	ctx    context.Context
}

const (
	telnetIAC  = 255
	telnetDONT = 254
	telnetDO   = 253
	telnetWONT = 252
	telnetWILL = 251
	telnetSB   = 250
	telnetSE   = 240

	telnetOptBinary = 0
	telnetOptEcho   = 1
	telnetOptSGA    = 3
)

var (
	telnetUsernameRe = regexp.MustCompile(`(?im)(login|username)\s*[:>]?\s*$`)
	telnetPasswordRe = regexp.MustCompile(`(?im)password\s*[:>]?\s*$`)
	telnetCiscoRe    = regexp.MustCompile(`(?m)[A-Za-z0-9._():/@\-\[\]]+[>#]\s*$`)
	telnetMikroTikRe = regexp.MustCompile(`(?m)\[[^\]\r\n]+@[^\]\r\n]+\]\s*[>#]\s*$|[A-Za-z0-9._():/@\-\[\]]+[>#]\s*$`)
)

func (s *nocDataTelnetSession) login(user, pass string) error {
	log.Printf("[noc-data-telnet] %s: waiting for initial login prompt", s.host)
	initial, err := s.readUntil(20*time.Second, s.promptRe(), telnetUsernameRe, telnetPasswordRe)
	if err != nil {
		return err
	}
	log.FileOnlyf("[noc-data-telnet] %s: initial_login_output\n%s", s.host, initial)
	switch {
	case s.promptRe().MatchString(initial):
		log.Printf("[noc-data-telnet] %s: prompt already available without login", s.host)
		return nil
	case telnetUsernameRe.MatchString(initial):
		log.Printf("[noc-data-telnet] %s: username prompt received", s.host)
		if err := s.writeLine(user); err != nil {
			return err
		}
		log.Printf("[noc-data-telnet] %s: username sent, waiting for password prompt", s.host)
		next, err := s.readUntil(20*time.Second, s.promptRe(), telnetPasswordRe, telnetUsernameRe)
		if err != nil {
			return err
		}
		log.FileOnlyf("[noc-data-telnet] %s: post_username_output\n%s", s.host, next)
		if s.promptRe().MatchString(next) {
			log.Printf("[noc-data-telnet] %s: logged in after username step", s.host)
			return nil
		}
		if !telnetPasswordRe.MatchString(next) {
			return fmt.Errorf("unexpected telnet login prompt")
		}
	case telnetPasswordRe.MatchString(initial):
		log.Printf("[noc-data-telnet] %s: password prompt received immediately", s.host)
	default:
		return fmt.Errorf("unable to determine telnet login prompt")
	}

	if err := s.writeLine(pass); err != nil {
		return err
	}
	log.Printf("[noc-data-telnet] %s: password sent, waiting for shell prompt", s.host)
	loggedIn, err := s.readUntil(30*time.Second, s.promptRe(), telnetUsernameRe, telnetPasswordRe)
	if err != nil {
		return err
	}
	log.FileOnlyf("[noc-data-telnet] %s: post_password_output\n%s", s.host, loggedIn)
	if !s.promptRe().MatchString(loggedIn) {
		return fmt.Errorf("telnet login failed: output=%q", truncateTelnetPreview(loggedIn, 220))
	}
	return nil
}

func (s *nocDataTelnetSession) run(cmd string) (string, error) {
	log.Printf("[noc-data-telnet] %s: running command %q", s.host, strings.TrimSpace(cmd))
	if err := s.writeLine(cmd); err != nil {
		return "", err
	}
	out, err := s.readUntil(20*time.Second, s.promptRe())
	if err != nil {
		log.Printf("[noc-data-telnet] %s: command %q failed: %v", s.host, strings.TrimSpace(cmd), err)
		return "", err
	}
	log.Printf("[noc-data-telnet] %s: command %q completed (%d bytes)", s.host, strings.TrimSpace(cmd), len(out))
	log.FileOnlyf("[noc-data-telnet] %s: command %q full_output\n%s", s.host, strings.TrimSpace(cmd), out)
	return cleanupTelnetCommandOutput(out, cmd), nil
}

func (s *nocDataTelnetSession) writeLine(line string) error {
	if err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	_, err := io.WriteString(s.conn, line+"\r\n")
	return err
}

func (s *nocDataTelnetSession) readUntil(timeout time.Duration, patterns ...*regexp.Regexp) (string, error) {
	var buf bytes.Buffer
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if s.ctx != nil {
			select {
			case <-s.ctx.Done():
				_ = s.conn.Close()
				return sanitizeTelnetText(buf.Bytes()), s.ctx.Err()
			default:
			}
		}
		if err := s.conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return "", fmt.Errorf("set read deadline: %w", err)
		}
		chunk := make([]byte, 2048)
		n, err := s.reader.Read(chunk)
		if n > 0 {
			buf.Write(s.processTelnetChunk(chunk[:n]))
			clean := sanitizeTelnetText(buf.Bytes())
			for _, pattern := range patterns {
				if pattern != nil && pattern.MatchString(clean) {
					return clean, nil
				}
			}
		}
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				clean := sanitizeTelnetText(buf.Bytes())
				for _, pattern := range patterns {
					if pattern != nil && pattern.MatchString(clean) {
						return clean, nil
					}
				}
				continue
			}
			return "", fmt.Errorf("read telnet: %w", err)
		}
	}
	log.Printf("[noc-data-telnet] %s: timeout waiting for prompt after %s", s.host, timeout)
	return sanitizeTelnetText(buf.Bytes()), fmt.Errorf("timeout waiting for telnet prompt")
}

func (s *nocDataTelnetSession) processTelnetChunk(chunk []byte) []byte {
	if len(chunk) == 0 {
		return nil
	}

	payload := make([]byte, 0, len(chunk))
	for i := 0; i < len(chunk); i++ {
		if chunk[i] != telnetIAC {
			payload = append(payload, chunk[i])
			continue
		}

		if i+1 >= len(chunk) {
			break
		}
		cmd := chunk[i+1]
		switch cmd {
		case telnetIAC:
			payload = append(payload, telnetIAC)
			i++
		case telnetDO, telnetDONT, telnetWILL, telnetWONT:
			if i+2 >= len(chunk) {
				i = len(chunk)
				break
			}
			opt := chunk[i+2]
			s.replyTelnetNegotiation(cmd, opt)
			i += 2
		case telnetSB:
			i += 2
			for i < len(chunk) {
				if chunk[i] == telnetIAC && i+1 < len(chunk) && chunk[i+1] == telnetSE {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}

	return payload
}

func (s *nocDataTelnetSession) replyTelnetNegotiation(cmd, opt byte) {
	var reply []byte

	switch cmd {
	case telnetDO:
		switch opt {
		case telnetOptBinary, telnetOptSGA:
			reply = []byte{telnetIAC, telnetWILL, opt}
		default:
			reply = []byte{telnetIAC, telnetWONT, opt}
		}
	case telnetDONT:
		reply = []byte{telnetIAC, telnetWONT, opt}
	case telnetWILL:
		switch opt {
		case telnetOptBinary, telnetOptEcho, telnetOptSGA:
			reply = []byte{telnetIAC, telnetDO, opt}
		default:
			reply = []byte{telnetIAC, telnetDONT, opt}
		}
	case telnetWONT:
		reply = []byte{telnetIAC, telnetDONT, opt}
	default:
		return
	}

	if len(reply) == 0 {
		return
	}

	if err := s.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return
	}
	if _, err := s.conn.Write(reply); err == nil {
		log.Printf("[noc-data-telnet] %s: negotiated telnet option cmd=%d opt=%d", s.host, cmd, opt)
	}
}

func nocDataSSHSendCommandContext(ctx context.Context, host, user, pass, vendor string, cmds ...string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(vendor), "mikrotik") {
		return MikrotikNocPassSSHContext(ctx, host, user, pass, cmds...)
	}

	type result struct {
		output string
		err    error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := NocPassSendCommand(host, user, pass, vendor, cmds...)
		ch <- result{output: out, err: err}
	}()

	select {
	case res := <-ch:
		return res.output, res.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (s *nocDataTelnetSession) promptRe() *regexp.Regexp {
	if s.vendor == "mikrotik" {
		return telnetMikroTikRe
	}
	return telnetCiscoRe
}

func sanitizeTelnetText(data []byte) string {
	clean := ansiRe.ReplaceAll(data, nil)
	var builder strings.Builder
	for _, b := range clean {
		if b == '\r' || b == '\n' || b == '\t' || (b >= 32 && b <= 126) {
			builder.WriteByte(b)
		}
	}
	return strings.TrimSpace(builder.String())
}

func cleanupTelnetCommandOutput(out, cmd string) string {
	lines := strings.Split(out, "\n")
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
		if telnetCiscoRe.MatchString(trimmed) || telnetMikroTikRe.MatchString(trimmed) {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func truncateTelnetPreview(out string, limit int) string {
	trimmed := strings.TrimSpace(out)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "..."
}

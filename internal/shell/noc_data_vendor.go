package shell

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	log "github.com/Flafl/DevOpsCore/utils"
)

func DetectNocDataVendor(ip string, timeout time.Duration) (string, string, string, error) {
	log.Printf("[noc-data-detect] %s: start timeout=%s", ip, timeout)
	vendor, banner, err := detectNocDataSSHVendor(ip, timeout)
	if err == nil {
		log.Printf("[noc-data-detect] %s: SSH classified vendor=%s", ip, vendor)
		log.FileOnlyf("[noc-data-detect] %s: ssh_banner\n%s", ip, banner)
		return vendor, "ssh", banner, nil
	}

	sshErr := err
	log.Printf("[noc-data-detect] %s: SSH detect failed: %v", ip, err)
	vendor, banner, err = detectNocDataTelnetVendor(ip, timeout)
	if err == nil {
		log.Printf("[noc-data-detect] %s: telnet classified vendor=%s", ip, vendor)
		log.FileOnlyf("[noc-data-detect] %s: telnet_banner\n%s", ip, banner)
		return vendor, "telnet", banner, nil
	}

	log.Printf("[noc-data-detect] %s: telnet detect failed: %v", ip, err)
	return "unknown", "", "", fmt.Errorf("ssh failed: %v; telnet failed: %w", sshErr, err)
}

func detectNocDataSSHVendor(ip string, timeout time.Duration) (string, string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "22"), timeout)
	if err != nil {
		return "unknown", "", fmt.Errorf("dial ssh: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "unknown", "", fmt.Errorf("set read deadline: %w", err)
	}

	banner, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "unknown", "", fmt.Errorf("read ssh banner: %w", err)
	}

	banner = strings.TrimSpace(banner)
	log.FileOnlyf("[noc-data-detect] %s: raw_ssh_banner\n%s", ip, banner)
	lowerBanner := strings.ToLower(banner)

	switch {
	case strings.Contains(lowerBanner, "rosssh"), strings.Contains(lowerBanner, "mikrotik"):
		return "mikrotik", banner, nil
	case strings.Contains(lowerBanner, "cisco"):
		return "cisco", banner, nil
	default:
		return "unknown", banner, nil
	}
}

func detectNocDataTelnetVendor(ip string, timeout time.Duration) (string, string, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, "23"), timeout)
	if err != nil {
		return "unknown", "", fmt.Errorf("dial telnet: %w", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return "unknown", "", fmt.Errorf("set read deadline: %w", err)
	}

	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil && err != io.EOF {
		return "unknown", "", fmt.Errorf("read telnet banner: %w", err)
	}

	banner := sanitizeNocDataTelnetBanner(buffer[:n])
	if _, err := conn.Write([]byte("\r\n")); err != nil {
		return "unknown", banner, fmt.Errorf("write telnet newline: %w", err)
	}

	n, err = conn.Read(buffer)
	if err != nil && err != io.EOF {
		return "unknown", banner, fmt.Errorf("read telnet follow-up: %w", err)
	}

	followUp := sanitizeNocDataTelnetBanner(buffer[:n])
	combinedBanner := strings.TrimSpace(strings.Join([]string{banner, followUp}, " "))
	log.FileOnlyf("[noc-data-detect] %s: raw_telnet_banner_initial\n%s", ip, banner)
	log.FileOnlyf("[noc-data-detect] %s: raw_telnet_banner_followup\n%s", ip, followUp)
	if combinedBanner == "" {
		return "unknown", "", fmt.Errorf("read telnet banner: non-printable response")
	}

	return classifyNocDataVendor(combinedBanner), combinedBanner, nil
}

func sanitizeNocDataTelnetBanner(data []byte) string {
	var builder strings.Builder
	for _, b := range data {
		if b == '\r' || b == '\n' || b == '\t' || (b >= 32 && b <= 126) {
			builder.WriteByte(b)
		}
	}
	return strings.TrimSpace(builder.String())
}

func classifyNocDataVendor(banner string) string {
	lowerBanner := strings.ToLower(banner)

	switch {
	case strings.Contains(lowerBanner, "rosssh"),
		strings.Contains(lowerBanner, "mikrotik"),
		strings.Contains(lowerBanner, "routeros"):
		return "mikrotik"
	case strings.Contains(lowerBanner, "cisco"),
		strings.Contains(lowerBanner, "tacacs"),
		strings.Contains(lowerBanner, "user access verification"),
		strings.Contains(lowerBanner, "ios"):
		return "cisco"
	default:
		return "unknown"
	}
}

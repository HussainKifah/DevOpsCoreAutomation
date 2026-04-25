package nocpass

import (
	"fmt"
	"net"
	"strings"
)

func NormalizeExclusionTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	if strings.Contains(target, "-") {
		return target
	}
	return target + "-" + target
}

func HostMatchesIPv4Spec(cidr, rawSpec, host string) (bool, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return false, fmt.Errorf("parse subnet: %w", err)
	}
	hostIP := net.ParseIP(strings.TrimSpace(host)).To4()
	if hostIP == nil {
		return false, fmt.Errorf("invalid IPv4 host %q", host)
	}
	if !ipNet.Contains(hostIP) {
		return false, nil
	}

	spec := strings.TrimSpace(rawSpec)
	if spec == "" {
		return false, fmt.Errorf("empty target")
	}
	if !strings.Contains(spec, "-") {
		ip, err := parseIPv4SpecEndpoint(ipNet, spec)
		if err != nil {
			return false, err
		}
		return compareIPv4(hostIP, ip) == 0, nil
	}

	start, end, err := parseIPv4SpecBounds(ipNet, spec)
	if err != nil {
		return false, err
	}
	return compareIPv4(hostIP, start) >= 0 && compareIPv4(hostIP, end) <= 0, nil
}

func ValidateIPv4Spec(cidr, rawSpec string) error {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return fmt.Errorf("parse subnet: %w", err)
	}

	spec := strings.TrimSpace(rawSpec)
	if spec == "" {
		return fmt.Errorf("empty target")
	}
	if !strings.Contains(spec, "-") {
		_, err := parseIPv4SpecEndpoint(ipNet, spec)
		return err
	}

	_, _, err = parseIPv4SpecBounds(ipNet, spec)
	return err
}

func parseIPv4SpecBounds(ipNet *net.IPNet, raw string) (net.IP, net.IP, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("range must look like start-end, for example 10-25 or 10.130.100.0-10.130.240.0")
	}
	start, err := parseIPv4SpecEndpoint(ipNet, parts[0])
	if err != nil {
		return nil, nil, err
	}
	end, err := parseIPv4SpecEndpoint(ipNet, parts[1])
	if err != nil {
		return nil, nil, err
	}
	if compareIPv4(start, end) > 0 {
		return nil, nil, fmt.Errorf("range start must be <= range end")
	}
	return start, end, nil
}

func parseIPv4SpecEndpoint(ipNet *net.IPNet, raw string) (net.IP, error) {
	raw = strings.TrimSpace(raw)
	ip := net.ParseIP(raw).To4()
	if ip != nil {
		if !ipNet.Contains(ip) {
			return nil, fmt.Errorf("ip %s falls outside subnet %s", ip.String(), ipNet.String())
		}
		return append(net.IP(nil), ip...), nil
	}
	var value int
	_, err := fmt.Sscanf(raw, "%d", &value)
	if err != nil || value < 0 || value > 255 {
		return nil, fmt.Errorf("invalid range endpoint %q", raw)
	}
	ip = append(net.IP(nil), ipNet.IP.Mask(ipNet.Mask).To4()...)
	hostOctet := shorthandHostOctetIndex(ipNet.Mask)
	ip[hostOctet] = byte(value)
	for i := hostOctet + 1; i < len(ip); i++ {
		ip[i] = 0
	}
	if !ipNet.Contains(ip) {
		return nil, fmt.Errorf("ip %s falls outside subnet %s", ip.String(), ipNet.String())
	}
	return ip, nil
}

func shorthandHostOctetIndex(mask net.IPMask) int {
	ones, bits := mask.Size()
	if bits != 32 {
		return 3
	}
	switch ones {
	case 8:
		return 1
	case 16:
		return 2
	case 24:
		return 3
	default:
		return 3
	}
}

func compareIPv4(a, b net.IP) int {
	for i := 0; i < 4; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

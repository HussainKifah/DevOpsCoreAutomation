package nocdata

import (
	"fmt"
	"net"
	"strings"
)

func ExpandIPv4Range(cidr, rawRange string) ([]string, error) {
	count, err := CountIPv4Range(cidr, rawRange)
	if err != nil {
		return nil, err
	}
	hosts := make([]string, 0, count)
	err = WalkIPv4Range(cidr, rawRange, func(host string) error {
		hosts = append(hosts, host)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hosts, nil
}

func CountIPv4Range(cidr, rawRange string) (uint64, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return 0, fmt.Errorf("parse subnet: %w", err)
	}

	network := ipNet.IP.Mask(ipNet.Mask).To4()
	if network == nil {
		return 0, fmt.Errorf("only IPv4 subnets are supported")
	}

	start, end, err := parseRangeBounds(ipNet, strings.TrimSpace(rawRange))
	if err != nil {
		return 0, err
	}

	startInt := ipv4ToUint32(start)
	endInt := ipv4ToUint32(end)
	return uint64(endInt) - uint64(startInt) + 1, nil
}

func WalkIPv4Range(cidr, rawRange string, fn func(host string) error) error {
	if fn == nil {
		return fmt.Errorf("walk callback is required")
	}

	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return fmt.Errorf("parse subnet: %w", err)
	}

	network := ipNet.IP.Mask(ipNet.Mask).To4()
	if network == nil {
		return fmt.Errorf("only IPv4 subnets are supported")
	}

	start, end, err := parseRangeBounds(ipNet, strings.TrimSpace(rawRange))
	if err != nil {
		return err
	}

	startInt := ipv4ToUint32(start)
	endInt := ipv4ToUint32(end)
	for current := startInt; current <= endInt; current++ {
		ip := uint32ToIPv4(current)
		if err := fn(ip.String()); err != nil {
			return err
		}
	}
	return nil
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
		ip, err := parseRangeEndpoint(ipNet, spec)
		if err != nil {
			return false, err
		}
		return compareIPv4(hostIP, ip) == 0, nil
	}

	start, end, err := parseRangeBounds(ipNet, spec)
	if err != nil {
		return false, err
	}
	return compareIPv4(hostIP, start) >= 0 && compareIPv4(hostIP, end) <= 0, nil
}

func FormatIPv4Range(cidr, rawRange string) (string, error) {
	clean := strings.TrimSpace(rawRange)
	if clean == "" {
		return "", fmt.Errorf("empty range")
	}
	if strings.Contains(clean, ".") && strings.Contains(clean, "-") {
		return clean, nil
	}

	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(cidr))
	if err != nil {
		return "", fmt.Errorf("parse subnet: %w", err)
	}
	start, end, err := parseRangeBounds(ipNet, clean)
	if err != nil {
		return "", err
	}
	return start.String() + "-" + end.String(), nil
}

func parseRangeBounds(ipNet *net.IPNet, raw string) (net.IP, net.IP, error) {
	parts := strings.Split(raw, "-")
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("range must look like start-end, for example 10-25 or 10.130.100.0-10.130.240.0")
	}
	start, err := parseRangeEndpoint(ipNet, parts[0])
	if err != nil {
		return nil, nil, err
	}
	end, err := parseRangeEndpoint(ipNet, parts[1])
	if err != nil {
		return nil, nil, err
	}
	if compareIPv4(start, end) > 0 {
		return nil, nil, fmt.Errorf("range start must be <= range end")
	}
	return start, end, nil
}

func parseRangeEndpoint(ipNet *net.IPNet, raw string) (net.IP, error) {
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

func ipv4ToUint32(ip net.IP) uint32 {
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

func uint32ToIPv4(v uint32) net.IP {
	return net.IPv4(byte(v>>24), byte(v>>16), byte(v>>8), byte(v)).To4()
}

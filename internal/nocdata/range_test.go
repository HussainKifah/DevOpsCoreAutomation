package nocdata

import "testing"

func TestExpandIPv4RangeSupportsFullIPBounds(t *testing.T) {
	hosts, err := ExpandIPv4Range("10.130.0.0/16", "10.130.100.0-10.130.240.0")
	if err != nil {
		t.Fatalf("ExpandIPv4Range returned error: %v", err)
	}
	if len(hosts) != 35841 {
		t.Fatalf("unexpected host count: got %d want %d", len(hosts), 35841)
	}
	if hosts[0] != "10.130.100.0" {
		t.Fatalf("unexpected first host: %s", hosts[0])
	}
	if hosts[len(hosts)-1] != "10.130.240.0" {
		t.Fatalf("unexpected last host: %s", hosts[len(hosts)-1])
	}
}

func TestExpandIPv4RangeIncludesNetworkAndBroadcastAddresses(t *testing.T) {
	hosts, err := ExpandIPv4Range("10.130.0.0/24", "10.130.0.0-10.130.0.255")
	if err != nil {
		t.Fatalf("ExpandIPv4Range returned error: %v", err)
	}
	if len(hosts) != 256 {
		t.Fatalf("unexpected host count: got %d want %d", len(hosts), 256)
	}
	if hosts[0] != "10.130.0.0" {
		t.Fatalf("unexpected first host: %s", hosts[0])
	}
	if hosts[len(hosts)-1] != "10.130.0.255" {
		t.Fatalf("unexpected last host: %s", hosts[len(hosts)-1])
	}
}

func TestExpandIPv4RangeUsesThirdOctetForShortRangeInSlash16(t *testing.T) {
	hosts, err := ExpandIPv4Range("10.130.0.0/16", "0-255")
	if err != nil {
		t.Fatalf("ExpandIPv4Range returned error: %v", err)
	}
	if len(hosts) != 65281 {
		t.Fatalf("unexpected host count: got %d want %d", len(hosts), 65281)
	}
	if hosts[0] != "10.130.0.0" {
		t.Fatalf("unexpected first host: %s", hosts[0])
	}
	if hosts[len(hosts)-1] != "10.130.255.0" {
		t.Fatalf("unexpected last host: %s", hosts[len(hosts)-1])
	}
}

func TestExpandIPv4RangeUsesLastOctetForShortRangeInSlash24(t *testing.T) {
	hosts, err := ExpandIPv4Range("10.130.0.0/24", "0-255")
	if err != nil {
		t.Fatalf("ExpandIPv4Range returned error: %v", err)
	}
	if len(hosts) != 256 {
		t.Fatalf("unexpected host count: got %d want %d", len(hosts), 256)
	}
	if hosts[0] != "10.130.0.0" {
		t.Fatalf("unexpected first host: %s", hosts[0])
	}
	if hosts[len(hosts)-1] != "10.130.0.255" {
		t.Fatalf("unexpected last host: %s", hosts[len(hosts)-1])
	}
}

func TestCountIPv4RangeSupportsFullSlash8(t *testing.T) {
	count, err := CountIPv4Range("10.0.0.0/8", "10.0.0.0-10.255.255.255")
	if err != nil {
		t.Fatalf("CountIPv4Range returned error: %v", err)
	}
	if count != 16777216 {
		t.Fatalf("unexpected host count: got %d want %d", count, 16777216)
	}
}

func TestWalkIPv4RangeStreamsAddressesInOrder(t *testing.T) {
	var hosts []string
	err := WalkIPv4Range("10.130.0.0/24", "10.130.0.1-10.130.0.3", func(host string) error {
		hosts = append(hosts, host)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkIPv4Range returned error: %v", err)
	}
	if len(hosts) != 3 {
		t.Fatalf("unexpected host count: got %d want %d", len(hosts), 3)
	}
	if hosts[0] != "10.130.0.1" || hosts[1] != "10.130.0.2" || hosts[2] != "10.130.0.3" {
		t.Fatalf("unexpected walk order: %#v", hosts)
	}
}

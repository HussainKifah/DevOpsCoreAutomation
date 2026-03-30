package nocpass

import (
	"fmt"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/models"
)

// ShellVendor returns the IPSendCommand vendor key for this device.
func ShellVendor(d *models.NocPassDevice) (string, error) {
	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios":
		return "cisco", nil
	case "cisco_nexus":
		return "nexus", nil
	case "mikrotik":
		return "mikrotik", nil
	default:
		return "", fmt.Errorf("unknown vendor %q", d.Vendor)
	}
}

// BuildCommandList configures fiberx (15 / full admin) and readOnly (13 / read-only) with the same password.
// initialMikrotik: when true, create both RouterOS users; otherwise update passwords only.
func BuildCommandList(d *models.NocPassDevice, plainPassword string, initialMikrotik bool) ([]string, error) {
	p := plainPassword
	if p == "" {
		return nil, fmt.Errorf("password required")
	}

	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios":
		return []string{
			"configure terminal",
			fmt.Sprintf("username %s privilege 15 secret 0 %s", UserFiberx, p),
			fmt.Sprintf("username %s privilege 13 secret 0 %s", UserReadOnly, p),
			"end",
			"write memory",
		}, nil

	case "cisco_nexus":
		// readOnly: network-operator is the usual read-oriented role (analogous to IOS 13).
		return []string{
			"configure terminal",
			fmt.Sprintf("username %s password %s role priv-15", UserFiberx, p),
			fmt.Sprintf("username %s password %s role network-operator", UserReadOnly, p),
			"end",
			"copy running-config startup-config",
		}, nil

	case "mikrotik":
		if initialMikrotik {
			return []string{
				fmt.Sprintf("user add group=full name=%s password=%s", UserFiberx, p),
				fmt.Sprintf("user add group=read name=%s password=%s", UserReadOnly, p),
			}, nil
		}
		return []string{
			fmt.Sprintf(`/user set [find name=%s] password=%s`, UserFiberx, p),
			fmt.Sprintf(`/user set [find name=%s] password=%s`, UserReadOnly, p),
		}, nil

	default:
		return nil, fmt.Errorf("unknown vendor %q", d.Vendor)
	}
}

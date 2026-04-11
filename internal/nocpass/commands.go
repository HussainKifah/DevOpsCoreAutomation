package nocpass

import (
	"fmt"
	"slices"
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

func NormalizeUsernames(names ...string) []string {
	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, name := range names {
		n := NormalizeUsername(name)
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

func NormalizeUsername(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func ProtectedUsernames(extra ...string) []string {
	base := []string{UserFiberx, UserReadOnly, UserDev}
	base = append(base, extra...)
	return NormalizeUsernames(base...)
}

func containsUsername(list []string, username string) bool {
	needle := NormalizeUsername(username)
	for _, item := range list {
		if item == needle {
			return true
		}
	}
	return false
}

func CiscoUserDiscoveryCommand(d *models.NocPassDevice) (string, error) {
	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios", "cisco_nexus":
		return "show running-config | include ^username", nil
	default:
		return "", fmt.Errorf("user discovery not supported for vendor %q", d.Vendor)
	}
}

func ExtractCiscoUsernames(output string) []string {
	lines := strings.Split(output, "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || !strings.EqualFold(fields[0], "username") {
			continue
		}
		names = append(names, strings.Trim(fields[1], `"`))
	}
	return names
}

func BuildCommandList(d *models.NocPassDevice, plainPassword string, initialMikrotik bool, existingUsers, keepUsers []string) ([]string, error) {
	p := plainPassword
	if p == "" {
		return nil, fmt.Errorf("password required")
	}

	protected := ProtectedUsernames(keepUsers...)

	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios":
		cmds := []string{"configure terminal"}
		for _, user := range existingUsers {
			if containsUsername(protected, user) {
				continue
			}
			cmds = append(cmds, fmt.Sprintf("no username %s", user))
		}
		cmds = append(cmds,
			fmt.Sprintf("username %s privilege 15 secret 0 %s", UserFiberx, p),
			fmt.Sprintf("username %s privilege 13 secret 0 %s", UserReadOnly, p),
			"end",
			"write memory",
		)
		return cmds, nil

	case "cisco_nexus":
		cmds := []string{"configure terminal"}
		for _, user := range existingUsers {
			if containsUsername(protected, user) {
				continue
			}
			cmds = append(cmds, fmt.Sprintf("no username %s", user))
		}
		cmds = append(cmds,
			fmt.Sprintf("username %s password %s role priv-15", UserFiberx, p),
			fmt.Sprintf("username %s password %s role network-operator", UserReadOnly, p),
			"end",
			"copy running-config startup-config",
		)
		return cmds, nil

	case "mikrotik":
		removeCmd := buildMikrotikRemoveCommand(append([]string{UserFiberx, UserReadOnly, UserDev}, keepUsers...))
		if initialMikrotik {
			return []string{
				removeCmd,
				buildMikrotikEnsureUserCommand(UserFiberx, "full", p),
				buildMikrotikEnsureUserCommand(UserReadOnly, "read", p),
				fmt.Sprintf("/user set [find name=%s] password=%s", UserFiberx, p),
				fmt.Sprintf("/user set [find name=%s] password=%s", UserReadOnly, p),
			}, nil
		}
		return []string{
			removeCmd,
			fmt.Sprintf("/user set [find name=%s] password=%s", UserFiberx, p),
			fmt.Sprintf("/user set [find name=%s] password=%s", UserReadOnly, p),
		}, nil

	default:
		return nil, fmt.Errorf("unknown vendor %q", d.Vendor)
	}
}

func buildMikrotikRemoveCommand(keepUsers []string) string {
	conds := make([]string, 0, len(keepUsers))
	for _, user := range keepUsers {
		u := strings.TrimSpace(user)
		if u == "" {
			continue
		}
		conds = append(conds, fmt.Sprintf(`name!="%s"`, u))
	}
	return "/user remove [find " + strings.Join(conds, " && ") + "]"
}

func buildMikrotikEnsureUserCommand(username, group, password string) string {
	return fmt.Sprintf(`:if ([:len [/user find where name="%s"]] = 0) do={ /user add group=%s name=%s password=%s }`, username, group, username, password)
}

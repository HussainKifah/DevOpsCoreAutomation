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
	case "huawei":
		return "huawei", nil
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
	base := []string{UserFiberx, UserSupport, UserDev}
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

func UserDiscoveryCommands(d *models.NocPassDevice) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios", "cisco_nexus":
		return []string{"show running-config | include ^username"}, nil
	case "huawei":
		return []string{"display users all"}, nil
	default:
		return nil, fmt.Errorf("user discovery not supported for vendor %q", d.Vendor)
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

func ExtractHuaweiUsernames(output string) []string {
	lines := strings.Split(output, "\n")
	names := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if idx := strings.Index(strings.ToLower(trimmed), "username"); idx >= 0 {
			parts := strings.SplitN(trimmed[idx:], ":", 2)
			if len(parts) == 2 {
				name := strings.TrimSpace(parts[1])
				name = strings.Trim(name, `"`)
				if strings.EqualFold(name, "unspecified") {
					continue
				}
				key := NormalizeUsername(name)
				if key == "" {
					continue
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				names = append(names, name)
			}
		}
	}
	return names
}

func ExtractUsernamesForVendor(vendor, output string) []string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "huawei":
		return ExtractHuaweiUsernames(output)
	default:
		return ExtractCiscoUsernames(output)
	}
}

func BuildCommandList(d *models.NocPassDevice, fiberxPassword, supportPassword string, initialMikrotik bool, existingUsers, keepUsers []string) ([]string, error) {
	if fiberxPassword == "" || supportPassword == "" {
		return nil, fmt.Errorf("fiberx and support passwords are required")
	}

	protected := ProtectedUsernames(keepUsers...)

	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios":
		cmds := []string{"configure terminal"}
		cmds = append(cmds, buildCiscoDeleteCommands(existingUsers, protected)...)
		cmds = append(cmds,
			fmt.Sprintf("username %s privilege 15 secret 0 %s", UserFiberx, fiberxPassword),
			// IOS has no portable built-in write-only role, so keep SUPPORT at a standard admin-capable privilege.
			fmt.Sprintf("username %s privilege 15 secret 0 %s", UserSupport, supportPassword),
			"end",
			"write memory",
		)
		return cmds, nil

	case "cisco_nexus":
		cmds := []string{"configure terminal"}
		cmds = append(cmds, buildCiscoDeleteCommands(existingUsers, protected)...)
		cmds = append(cmds,
			fmt.Sprintf("username %s password %s role priv-15", UserFiberx, fiberxPassword),
			fmt.Sprintf("username %s password %s role network-admin", UserSupport, supportPassword),
			"end",
			"copy running-config startup-config",
		)
		return cmds, nil

	case "mikrotik":
		removeCmd := buildMikrotikRemoveCommand(append([]string{UserFiberx, UserSupport, UserDev}, keepUsers...))
		if initialMikrotik {
			return []string{
				removeCmd,
				buildMikrotikEnsureUserCommand(UserFiberx, "full", fiberxPassword),
				buildMikrotikEnsureUserCommand(UserSupport, "write", supportPassword),
				fmt.Sprintf("/user set [find name=%s] password=%s", UserFiberx, fiberxPassword),
				fmt.Sprintf("/user set [find name=%s] password=%s", UserSupport, supportPassword),
			}, nil
		}
		return []string{
			removeCmd,
			fmt.Sprintf("/user set [find name=%s] password=%s", UserFiberx, fiberxPassword),
			fmt.Sprintf("/user set [find name=%s] password=%s", UserSupport, supportPassword),
		}, nil

	case "huawei":
		cmds := make([]string, 0, len(existingUsers)+8)
		cmds = append(cmds, buildHuaweiDeleteCommands(existingUsers, protected)...)
		cmds = append(cmds,
			fmt.Sprintf("local-user %s password irreversible-cipher %s", UserFiberx, fiberxPassword),
			fmt.Sprintf("local-user %s privilege level 15", UserFiberx),
			fmt.Sprintf("local-user %s service-type ssh terminal", UserFiberx),
			fmt.Sprintf("local-user %s password irreversible-cipher %s", UserSupport, supportPassword),
			fmt.Sprintf("local-user %s privilege level 3", UserSupport),
			fmt.Sprintf("local-user %s service-type ssh terminal", UserSupport),
			"save",
			"y",
		)
		return cmds, nil

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

func buildCiscoDeleteCommands(existingUsers, protected []string) []string {
	cmds := make([]string, 0, len(existingUsers))
	seen := map[string]struct{}{}
	for _, user := range existingUsers {
		raw := strings.TrimSpace(user)
		key := NormalizeUsername(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if containsUsername(protected, user) {
			continue
		}
		cmds = append(cmds, fmt.Sprintf("no username %s", raw))
	}
	return cmds
}

func buildHuaweiDeleteCommands(existingUsers, protected []string) []string {
	cmds := make([]string, 0, len(existingUsers))
	seen := map[string]struct{}{}
	for _, user := range existingUsers {
		raw := strings.TrimSpace(user)
		key := NormalizeUsername(raw)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if containsUsername(protected, user) {
			continue
		}
		cmds = append(cmds, fmt.Sprintf("undo local-user %s", raw))
	}
	return cmds
}

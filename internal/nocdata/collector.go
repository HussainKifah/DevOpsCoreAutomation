package nocdata

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/nocpass"
)

type Snapshot struct {
	Method        string
	Profile       string
	Status        string
	Error         string
	Hostname      string
	Model         string
	Version       string
	Serial        string
	Uptime        string
	IFUp          int
	IFDown        int
	DefaultRouter bool
	LayerMode     string
	UserCount     int
	Users         []string
	SSHEnabled    bool
	TelnetEnabled bool
	SNMPEnabled   bool
	NTPEnabled    bool
	AAAEnabled    bool
	SyslogEnabled bool
}

type CommandSet struct {
	ShowVersion string
	HardwareID  string
	IntStatus   string
	DefaultGW   string
	Users       string
	VTY         string
	SNMP        string
	NTP         string
	AAA         string
	Syslog      string
	LayerMode   string
	Identity    string
	Routerboard string
}

func ShellVendor(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "cisco_ios":
		return "cisco", nil
	case "cisco_nexus":
		return "nexus", nil
	case "huawei":
		return "huawei", nil
	case "mikrotik":
		return "mikrotik", nil
	default:
		return "", fmt.Errorf("unsupported vendor %q", v)
	}
}

func CommandsForVendor(v string) (CommandSet, []string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "cisco_ios", "cisco_nexus":
		set := CommandSet{
			ShowVersion: "show version",
			IntStatus:   "show int status",
			DefaultGW:   "show running-config | include 0.0.0.0",
			Users:       "show running-config | include ^username",
			VTY:         "show running-config | section line vty",
			SNMP:        "show snmp",
			NTP:         "show ntp status",
			AAA:         "show running-config | section aaa",
			Syslog:      "show logging",
			LayerMode:   "show ip route",
		}
		return set, []string{
			set.ShowVersion,
			set.IntStatus,
			set.DefaultGW,
			set.Users,
			set.VTY,
			set.SNMP,
			set.NTP,
			set.AAA,
			set.Syslog,
			set.LayerMode,
		}, nil
	case "mikrotik":
		set := CommandSet{
			ShowVersion: "/system resource print",
			IntStatus:   "/interface print",
			DefaultGW:   "/ip route print where dst-address=0.0.0.0/0",
			Users:       "/user print detail",
			VTY:         "/ip service print detail",
			SNMP:        "/snmp print detail",
			NTP:         "/system ntp client print detail",
			AAA:         "/radius print detail",
			Syslog:      "/system logging print",
			Identity:    "/system identity print",
			Routerboard: "/system routerboard print",
		}
		return set, []string{
			set.ShowVersion,
			set.IntStatus,
			set.DefaultGW,
			set.Users,
			set.VTY,
			set.SNMP,
			set.NTP,
			set.AAA,
			set.Syslog,
			set.Identity,
			set.Routerboard,
		}, nil
	case "huawei":
		set := CommandSet{
			ShowVersion: "display version",
			HardwareID:  "display elabel",
			IntStatus:   "display interface description | include GE",
			DefaultGW:   "display ip routing-table",
			Users:       "display current-configuration",
			VTY:         "display current-configuration",
			SNMP:        "display current-configuration",
			NTP:         "display current-configuration",
			AAA:         "display current-configuration",
			Syslog:      "display current-configuration",
			LayerMode:   "display ip routing-table",
		}
		return set, []string{
			"screen-length 0 temporary",
			set.ShowVersion,
			set.HardwareID,
			set.IntStatus,
			set.DefaultGW,
			set.Users,
			set.VTY,
			set.SNMP,
			set.NTP,
			set.AAA,
			set.Syslog,
			set.LayerMode,
		}, nil
	default:
		return CommandSet{}, nil, fmt.Errorf("unsupported vendor %q", v)
	}
}

func SplitOutputs(output string, cmds []string) map[string]string {
	if len(cmds) == 0 {
		return map[string]string{}
	}
	sections := make(map[string]string, len(cmds))
	remaining := output
	for i, cmd := range cmds {
		if idx := strings.Index(remaining, cmd); idx >= 0 {
			remaining = remaining[idx+len(cmd):]
		}
		nextIdx := len(remaining)
		for _, nextCmd := range cmds[i+1:] {
			if j := strings.Index(remaining, nextCmd); j >= 0 && j < nextIdx {
				nextIdx = j
			}
		}
		sections[cmd] = strings.TrimSpace(remaining[:nextIdx])
		if nextIdx < len(remaining) {
			remaining = remaining[nextIdx:]
		} else {
			remaining = ""
		}
	}
	return sections
}

func CollectSnapshot(d *models.NocDataDevice, sections map[string]string) Snapshot {
	s := Snapshot{Method: "ssh", Status: "ok"}
	switch strings.ToLower(strings.TrimSpace(d.Vendor)) {
	case "cisco_ios", "cisco_nexus":
		s.Hostname, s.Model, s.Version, s.Serial, s.Uptime = parseCiscoVersion(sections)
		s.IFUp, s.IFDown = parseCiscoInterfaces(sections)
		s.DefaultRouter = hasSignal(sections["show running-config | include 0.0.0.0"])
		s.LayerMode = parseCiscoLayerMode(sections["show ip route"])
		s.Users = uniqueSorted(nocpass.ExtractCiscoUsernames(sections["show running-config | include ^username"]))
		s.UserCount = len(s.Users)
		s.SSHEnabled, s.TelnetEnabled = parseCiscoVTY(sections["show running-config | section line vty"])
		s.SNMPEnabled = hasSignal(sections["show snmp"])
		s.NTPEnabled = hasSignal(sections["show ntp status"])
		s.AAAEnabled = hasSignal(sections["show running-config | section aaa"])
		s.SyslogEnabled = hasSignal(sections["show logging"])
	case "mikrotik":
		s.Hostname = parseMikrotikIdentity(sections["/system identity print"])
		s.Model, s.Version, s.Serial, s.Uptime = parseMikrotikVersion(sections["/system resource print"], sections["/system routerboard print"])
		s.IFUp, s.IFDown = parseMikrotikInterfaces(sections["/interface print"])
		s.DefaultRouter = hasSignal(sections["/ip route print where dst-address=0.0.0.0/0"])
		s.LayerMode = parseMikrotikLayerMode(sections["/ip route print where dst-address=0.0.0.0/0"])
		s.Users = parseMikrotikUsers(sections["/user print detail"])
		s.UserCount = len(s.Users)
		s.SSHEnabled, s.TelnetEnabled = parseMikrotikServices(sections["/ip service print detail"])
		s.SNMPEnabled = parseMikrotikEnabled(sections["/snmp print detail"])
		s.NTPEnabled = hasSignal(sections["/system ntp client print detail"])
		s.AAAEnabled = parseMikrotikAAA(sections["/radius print detail"])
		s.SyslogEnabled = hasSignal(sections["/system logging print"])
	case "huawei":
		s.Hostname, s.Uptime = parseHuaweiDisplayVersion(sections["display version"])
		s.Model, s.Version, s.Serial = parseHuaweiElabel(sections["display elabel"])
		s.IFUp, s.IFDown = parseHuaweiInterfaces(sections["display interface description | include GE"])
		s.DefaultRouter = parseHuaweiDefaultRouter(sections["display ip routing-table"])
		s.LayerMode = parseHuaweiLayerMode(sections["display ip routing-table"])
		s.Users = parseHuaweiUsers(sections["display current-configuration"])
		s.UserCount = len(s.Users)
		s.SSHEnabled, s.TelnetEnabled = parseHuaweiServices(sections["display current-configuration"])
		s.SNMPEnabled = parseHuaweiSNMP(sections["display current-configuration"])
		s.NTPEnabled = parseHuaweiNTP(sections["display current-configuration"])
		s.AAAEnabled = parseHuaweiAAA(sections["display current-configuration"])
		s.SyslogEnabled = parseHuaweiSyslog(sections["display current-configuration"])
	}
	validateSnapshot(d, sections, &s)
	return s
}

func ProbeOutputLooksValid(vendor, cmd, out string) bool {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return false
	}
	if OutputLooksUnauthorized(trimmed) {
		return false
	}

	lower := strings.ToLower(trimmed)
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios", "cisco_nexus":
		if strings.Contains(lower, "invalid input") ||
			strings.Contains(lower, "unknown command") ||
			strings.Contains(lower, "command not found") ||
			strings.Contains(lower, "not recognized as an internal or external command") {
			return false
		}

		hostname, model, version, serial, uptime := parseCiscoVersion(map[string]string{
			"show version": trimmed,
		})
		if hostname != "" || model != "" || version != "" || serial != "" || uptime != "" {
			return true
		}

		return strings.Contains(lower, "cisco ios") ||
			strings.Contains(lower, "cisco nexus") ||
			strings.Contains(lower, "processor board id") ||
			strings.Contains(lower, "system serial number") ||
			strings.Contains(lower, " uptime is ")
	case "mikrotik":
		if strings.Contains(lower, "bad command name") ||
			strings.Contains(lower, "syntax error") ||
			strings.Contains(lower, "expected end of command") ||
			strings.Contains(lower, "no such item") {
			return false
		}

		if parseMikrotikIdentity(trimmed) != "" {
			return true
		}

		return strings.Contains(lower, "routeros") ||
			strings.Contains(lower, "name:") ||
			strings.Contains(lower, "name=")
	case "huawei":
		if strings.Contains(lower, "unrecognized command") ||
			strings.Contains(lower, "incomplete command") ||
			strings.Contains(lower, "too many parameters") ||
			strings.Contains(lower, "error:") {
			return false
		}

		hostname, uptime := parseHuaweiDisplayVersion(trimmed)
		if hostname != "" || uptime != "" {
			return true
		}

		return strings.Contains(lower, "huawei versatile routing platform software") ||
			strings.Contains(lower, "vrp (r) software") ||
			strings.Contains(lower, "routing switch uptime is")
	default:
		return hasSignal(trimmed)
	}
}

func ParseFailure(err error) Snapshot {
	return Snapshot{Method: "ssh", Status: "fail", Error: strings.TrimSpace(err.Error())}
}

func validateSnapshot(d *models.NocDataDevice, sections map[string]string, s *Snapshot) {
	if s == nil {
		return
	}

	vendor := strings.ToLower(strings.TrimSpace(d.Vendor))
	var problems []string

	switch vendor {
	case "cisco_ios", "cisco_nexus":
		problems = append(problems, missingSectionProblem(sections, "show version", "show version")...)
		problems = append(problems, missingSectionProblem(sections, "show int status", "show int status")...)
		if strings.TrimSpace(s.Hostname) == "" {
			problems = append(problems, "missing hostname")
		}
		if strings.TrimSpace(s.Model) == "" {
			problems = append(problems, "missing model")
		}
		if strings.TrimSpace(s.Version) == "" {
			problems = append(problems, "missing version")
		}
		if s.IFUp == 0 && s.IFDown == 0 {
			problems = append(problems, "missing interface counts")
		}
	case "mikrotik":
		problems = append(problems, missingSectionProblem(sections, "/system identity print", "identity output")...)
		problems = append(problems, missingSectionProblem(sections, "/system resource print", "resource output")...)
		problems = append(problems, missingSectionProblem(sections, "/ip service print detail", "service output")...)
		if strings.TrimSpace(s.Hostname) == "" {
			problems = append(problems, "missing hostname")
		}
		if strings.TrimSpace(s.Model) == "" {
			problems = append(problems, "missing model")
		}
		if strings.TrimSpace(s.Version) == "" {
			problems = append(problems, "missing version")
		}
		if strings.TrimSpace(s.Serial) == "" {
			problems = append(problems, "missing serial")
		}
		if strings.TrimSpace(s.Uptime) == "" {
			problems = append(problems, "missing uptime")
		}
		if !hasNonEmptySection(sections, "/interface print") {
			problems = append(problems, "missing interface output")
		}
	case "huawei":
		problems = append(problems, missingSectionProblem(sections, "display version", "display version")...)
		problems = append(problems, missingSectionProblem(sections, "display elabel", "display elabel")...)
		problems = append(problems, missingSectionProblem(sections, "display interface description | include GE", "interface output")...)
		problems = append(problems, missingSectionProblem(sections, "display current-configuration", "current configuration output")...)
		if strings.TrimSpace(s.Hostname) == "" {
			problems = append(problems, "missing hostname")
		}
		if strings.TrimSpace(s.Model) == "" {
			problems = append(problems, "missing model")
		}
		if strings.TrimSpace(s.Version) == "" {
			problems = append(problems, "missing version")
		}
		if strings.TrimSpace(s.Serial) == "" {
			problems = append(problems, "missing serial")
		}
		if strings.TrimSpace(s.Uptime) == "" {
			problems = append(problems, "missing uptime")
		}
		if s.IFUp == 0 && s.IFDown == 0 {
			problems = append(problems, "missing interface counts")
		}
	}

	if len(problems) == 0 {
		s.Status = "ok"
		s.Error = ""
		return
	}

	s.Status = "fail"
	s.Error = "collection completed but parsed data incomplete: " + strings.Join(uniqueSorted(problems), ", ")
}

func missingSectionProblem(sections map[string]string, key, label string) []string {
	if hasNonEmptySection(sections, key) {
		return nil
	}
	return []string{"missing " + label}
}

func hasNonEmptySection(sections map[string]string, key string) bool {
	value, ok := sections[key]
	if !ok {
		return false
	}
	return strings.TrimSpace(value) != ""
}

func parseCiscoVersion(sections map[string]string) (string, string, string, string, string) {
	out := sections["show version"]
	lines := strings.Split(out, "\n")
	var hostname, model, version, serial, uptime string
	iosVersionRe := regexp.MustCompile(`(?i)^cisco ios software,.*?\bversion\s+([^,\n]+)`)
	nxosVersionRe := regexp.MustCompile(`(?i)^nxos:\s+version\s+([^\r\n]+)$`)
	genericCiscoVersionRe := regexp.MustCompile(`(?i)\bversion\s+([^,\r\n]+)`)
	serialRe := regexp.MustCompile(`(?i)(processor board id|system serial number|serial number)\s*[:#]?\s*(\S+)`)
	iosModelRe := regexp.MustCompile(`(?i)^cisco\s+(\S+)\s+\([^)]+\)\s+processor`)
	nexusModelRe := regexp.MustCompile(`(?i)^cisco\s+(.+?)\s+chassis\s*$`)
	deviceNameRe := regexp.MustCompile(`(?i)^device name:\s*(.+)$`)
	kernelUptimeRe := regexp.MustCompile(`(?i)^kernel uptime is\s+(.+)$`)
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if trimmed == "" {
			continue
		}
		if hostname == "" {
			if m := deviceNameRe.FindStringSubmatch(trimmed); len(m) > 1 {
				hostname = strings.TrimSpace(m[1])
			}
		}
		if hostname == "" && strings.Contains(strings.ToLower(trimmed), " uptime is ") {
			parts := strings.SplitN(trimmed, " uptime is ", 2)
			if len(parts) == 2 {
				hostname = strings.TrimSpace(parts[0])
				uptime = strings.TrimSpace(parts[1])
			}
		}
		if uptime == "" {
			if m := kernelUptimeRe.FindStringSubmatch(trimmed); len(m) > 1 {
				uptime = strings.TrimSpace(m[1])
			}
		}
		if version == "" {
			if m := iosVersionRe.FindStringSubmatch(trimmed); len(m) > 1 {
				version = strings.TrimSpace(m[1])
			} else if m := nxosVersionRe.FindStringSubmatch(trimmed); len(m) > 1 {
				version = strings.TrimSpace(m[1])
			} else if strings.Contains(lower, "software") &&
				!strings.Contains(lower, "gpl") &&
				!strings.Contains(lower, "lgpl") &&
				!strings.Contains(lower, "license") &&
				!strings.HasPrefix(lower, "bios:") &&
				!strings.HasPrefix(lower, "rom:") {
				if m := genericCiscoVersionRe.FindStringSubmatch(trimmed); len(m) > 1 {
					version = strings.TrimSpace(m[1])
				}
			}
		}
		if serial == "" {
			if m := serialRe.FindStringSubmatch(trimmed); len(m) > 2 {
				serial = strings.TrimSpace(m[2])
			}
		}
		if model == "" {
			if m := iosModelRe.FindStringSubmatch(trimmed); len(m) > 1 {
				model = strings.Trim(m[1], `"`)
			}
		}
		if model == "" {
			if m := nexusModelRe.FindStringSubmatch(trimmed); len(m) > 1 {
				model = strings.Trim(m[1], `" `)
			}
		}
		if hostname != "" && strings.EqualFold(hostname, "kernel") {
			hostname = ""
		}
		if version != "" && (strings.Contains(lower, "gpl") || strings.Contains(version, " or ")) {
			version = ""
		}
	}
	return hostname, model, version, serial, uptime
}

func parseCiscoInterfaces(sections map[string]string) (int, int) {
	out := sections["show int status"]
	var up, down int
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(strings.ToLower(trimmed), "port ") || strings.HasPrefix(strings.ToLower(trimmed), "name ") || strings.HasPrefix(trimmed, "--") {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.Contains(lower, " connected ") || strings.HasSuffix(lower, " connected") {
			up++
			continue
		}
		if strings.Contains(lower, " notconnect ") || strings.Contains(lower, " notconnected ") || strings.Contains(lower, " disabled ") || strings.Contains(lower, " inactive ") || strings.Contains(lower, " err-disabled ") || strings.Contains(lower, " down ") {
			down++
		}
	}
	return up, down
}

func parseCiscoLayerMode(out string) string {
	if hasSignal(out) {
		return "3"
	}
	return "2"
}

func parseCiscoVTY(out string) (bool, bool) {
	lower := strings.ToLower(out)
	ssh := strings.Contains(lower, "transport input ssh") || strings.Contains(lower, "transport input all")
	telnet := strings.Contains(lower, "transport input telnet") || strings.Contains(lower, "transport input all")
	if strings.Contains(lower, "transport input ssh telnet") || strings.Contains(lower, "transport input telnet ssh") {
		ssh = true
		telnet = true
	}
	return ssh, telnet
}

func parseHuaweiDisplayVersion(out string) (string, string) {
	lines := strings.Split(out, "\n")
	var hostname, uptime string
	promptRe := regexp.MustCompile(`<([A-Za-z0-9._:-]+)>`)
	uptimeRe := regexp.MustCompile(`(?i)(?:^|.*\s)HUAWEI\s+(\S+)\s+.+?\s+uptime is\s+(.+)$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if hostname == "" {
			if m := promptRe.FindStringSubmatch(trimmed); len(m) > 1 {
				hostname = strings.TrimSpace(m[1])
			}
		}
		if uptime == "" {
			if m := uptimeRe.FindStringSubmatch(trimmed); len(m) > 2 {
				uptime = strings.TrimSpace(m[2])
			}
		}
	}

	return hostname, uptime
}

func parseHuaweiElabel(out string) (string, string, string) {
	var model, version, serial string
	lines := strings.Split(out, "\n")

	inMainBoard := false
	inBoardProperties := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			switch {
			case strings.EqualFold(section, "Main_Board"):
				inMainBoard = true
				inBoardProperties = false
			case strings.HasPrefix(strings.ToLower(section), "port_"):
				if model != "" || version != "" || serial != "" {
					return strings.TrimSpace(model), strings.TrimSpace(version), strings.TrimSpace(serial)
				}
				inMainBoard = false
				inBoardProperties = false
			case strings.EqualFold(section, "Board Properties"):
				inBoardProperties = inMainBoard
			default:
				inBoardProperties = false
			}
			continue
		}

		if !inMainBoard || !inBoardProperties {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "Model="):
			model = strings.TrimSpace(strings.TrimPrefix(trimmed, "Model="))
		case strings.HasPrefix(trimmed, "BoardType=") && model == "":
			model = strings.TrimSpace(strings.TrimPrefix(trimmed, "BoardType="))
		case strings.HasPrefix(trimmed, "BarCode="):
			serial = strings.TrimSpace(strings.TrimPrefix(trimmed, "BarCode="))
		case strings.HasPrefix(trimmed, "/$ElabelVersion="):
			version = strings.TrimSpace(strings.TrimPrefix(trimmed, "/$ElabelVersion="))
		}
	}

	return strings.TrimSpace(model), strings.TrimSpace(version), strings.TrimSpace(serial)
}

func parseHuaweiInterfaces(out string) (int, int) {
	var up, down int
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "phy:") ||
			strings.HasPrefix(lower, "*down") ||
			strings.HasPrefix(lower, "#down") ||
			strings.HasPrefix(lower, "-down") ||
			strings.HasPrefix(lower, "(l):") ||
			strings.HasPrefix(lower, "(s):") ||
			strings.HasPrefix(lower, "(e):") ||
			strings.HasPrefix(lower, "(b):") ||
			strings.HasPrefix(lower, "(dl):") ||
			strings.HasPrefix(lower, "(lb):") ||
			strings.HasPrefix(lower, "(lp):") ||
			strings.HasPrefix(lower, "(o):") ||
			strings.HasPrefix(lower, "interface ") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		if !strings.Contains(strings.ToUpper(fields[0]), "GE") {
			continue
		}

		phy := strings.ToLower(fields[1])
		if phy == "up" {
			up++
			continue
		}
		if strings.Contains(phy, "down") {
			down++
		}
	}
	return up, down
}

func parseHuaweiDefaultRouter(out string) bool {
	return strings.Contains(strings.ToLower(out), "0.0.0.0/0")
}

func parseHuaweiLayerMode(out string) string {
	if strings.Contains(strings.ToUpper(out), "ISIS") {
		return "3"
	}
	if hasSignal(out) {
		return "3"
	}
	return "2"
}

func parseHuaweiUsers(out string) []string {
	userRe := regexp.MustCompile(`(?im)^\s*local-user\s+"?([^"\s]+)"?\b`)
	var users []string
	for _, m := range userRe.FindAllStringSubmatch(out, -1) {
		if len(m) > 1 {
			users = append(users, m[1])
		}
	}
	return uniqueSorted(users)
}

func parseHuaweiServices(out string) (bool, bool) {
	lower := strings.ToLower(out)
	ssh := strings.Contains(lower, "stelnet server enable") ||
		strings.Contains(lower, "ssh server enable") ||
		strings.Contains(lower, "protocol inbound ssh") ||
		strings.Contains(lower, "protocol inbound all")
	telnet := strings.Contains(lower, "telnet server enable") ||
		strings.Contains(lower, "protocol inbound telnet") ||
		strings.Contains(lower, "protocol inbound all")
	return ssh, telnet
}

func parseHuaweiSNMP(out string) bool {
	return strings.Contains(strings.ToLower(out), "snmp-agent")
}

func parseHuaweiNTP(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "\nntp-service") || strings.HasPrefix(lower, "ntp-service")
}

func parseHuaweiAAA(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "\naaa") || strings.HasPrefix(lower, "aaa")
}

func parseHuaweiSyslog(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "info-center loghost") ||
		strings.Contains(lower, "info-center enable")
}

func parseMikrotikVersion(resourceOut, routerboardOut string) (string, string, string, string) {
	model := firstKV(routerboardOut, "model")
	if model == "" {
		model = firstKV(resourceOut, "board-name")
	}
	return model, firstKV(resourceOut, "version"), firstKV(routerboardOut, "serial-number"), firstKV(resourceOut, "uptime")
}

func parseMikrotikIdentity(out string) string {
	if name := firstKV(out, "name"); name != "" {
		return name
	}

	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if OutputLooksUnauthorized(trimmed) {
			continue
		}
		if strings.Contains(trimmed, "#") || strings.Contains(trimmed, ">") {
			continue
		}
		if !strings.ContainsAny(trimmed, "=:") {
			return trimmed
		}
	}
	return ""
}

func parseMikrotikInterfaces(out string) (int, int) {
	var up, down int
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if len(fields) < 3 {
			continue
		}
		if _, err := strconv.Atoi(fields[0]); err != nil {
			continue
		}
		flags := ""
		if isMikrotikFlags(fields[1]) {
			flags = fields[1]
		}
		if !containsMikrotikEtherType(fields[1:]) {
			continue
		}
		if strings.Contains(flags, "R") || strings.Contains(strings.ToUpper(trimmed), " RUNNING") {
			up++
		} else {
			down++
		}
	}
	return up, down
}

func containsMikrotikEtherType(fields []string) bool {
	for _, field := range fields {
		key := strings.ToLower(strings.TrimSpace(field))
		if key == "ether" {
			return true
		}
	}
	return false
}

func parseMikrotikUsers(out string) []string {
	nameRe := regexp.MustCompile(`(?i)\bname="?([^"\s]+)`)
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if m := nameRe.FindStringSubmatch(line); len(m) > 1 {
			names = append(names, m[1])
		}
	}
	return uniqueSorted(names)
}

func parseMikrotikServices(out string) (bool, bool) {
	return serviceEnabled(out, "ssh"), serviceEnabled(out, "telnet")
}

func parseMikrotikEnabled(out string) bool {
	lower := strings.ToLower(out)
	if strings.Contains(lower, "enabled: yes") || strings.Contains(lower, "enabled=yes") {
		return true
	}
	return hasSignal(out)
}

func parseMikrotikAAA(out string) bool {
	return strings.Contains(strings.ToLower(out), "service=")
}

func parseMikrotikLayerMode(out string) string {
	if hasSignal(out) {
		return "3"
	}
	return "2"
}

func serviceEnabled(out, name string) bool {
	for _, line := range strings.Split(strings.ToLower(out), "\n") {
		if !strings.Contains(line, name) {
			continue
		}
		if strings.Contains(line, "disabled: yes") || strings.Contains(line, "disabled=yes") || strings.HasPrefix(strings.TrimSpace(line), "x ") {
			return false
		}
		return true
	}
	return false
}

func firstKV(out, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	prefix := strings.ToLower(key) + ":"
	altPrefix := strings.ToLower(key) + "="
	quotedPattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `\b\s*[:=]\s*"([^"\r\n]+)"`)
	plainPattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(key) + `\b\s*[:=]\s*([^\r\n]+)`)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, prefix) {
			return strings.TrimSpace(trimmed[len(prefix):])
		}
		if strings.HasPrefix(lower, altPrefix) {
			return strings.TrimSpace(trimmed[len(altPrefix):])
		}
		if m := quotedPattern.FindStringSubmatch(trimmed); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
		if m := plainPattern.FindStringSubmatch(trimmed); len(m) > 1 {
			return strings.Trim(strings.TrimSpace(m[1]), `"`)
		}
	}
	return ""
}

func hasSignal(out string) bool {
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return false
	}
	if OutputLooksUnauthorized(trimmed) {
		return false
	}
	lower := strings.ToLower(trimmed)
	return !strings.Contains(lower, "invalid input") && !strings.Contains(lower, "unknown command")
}

func OutputLooksUnauthorized(out string) bool {
	lower := strings.ToLower(strings.TrimSpace(out))
	if lower == "" {
		return false
	}

	return strings.Contains(lower, "% authorization failed") ||
		strings.Contains(lower, "authorization failed") ||
		strings.Contains(lower, "% authentication failed") ||
		strings.Contains(lower, "authentication failed") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "access denied") ||
		strings.Contains(lower, "not authorized") ||
		strings.Contains(lower, "not enough permissions")
}

func isMikrotikFlags(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func uniqueSorted(list []string) []string {
	seen := map[string]string{}
	for _, item := range list {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; !ok {
			seen[key] = trimmed
		}
	}
	out := make([]string, 0, len(seen))
	for _, item := range seen {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

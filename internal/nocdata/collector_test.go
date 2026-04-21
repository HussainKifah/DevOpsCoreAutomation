package nocdata

import (
	"strings"
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
)

func TestCollectSnapshotCisco(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "cisco_ios"}
	sections := map[string]string{
		"show version":                            "EDGE-SW uptime is 3 weeks, 2 days\nCisco IOS Software, Version 15.2(7)E\ncisco WS-C2960X-48FPS-L (APM86XXX) processor\nProcessor board ID FOC12345",
		"show int status":                         "Port Name Status Vlan Duplex Speed Type\nGi1/0/1 x connected 1 a-full a-100 10/100/1000-TX\nGi1/0/2 x notconnect 1 auto auto 10/100/1000-TX",
		"show running-config | include 0.0.0.0":   "ip route 0.0.0.0 0.0.0.0 10.0.0.1",
		"show ip route":                           "Gateway of last resort is 10.0.0.1 to network 0.0.0.0",
		"show running-config | include ^username": "username fiberx privilege 15 secret 0 x\nusername readOnly privilege 13 secret 0 y",
		"show running-config | section line vty":  "line vty 0 4\n transport input ssh",
		"show snmp":                               "SNMP packets input",
		"show ntp status":                         "Clock is synchronized",
		"show running-config | section aaa":       "aaa new-model",
		"show logging":                            "Syslog logging: enabled",
	}
	s := CollectSnapshot(d, sections)
	if s.Status != "ok" || s.Hostname != "EDGE-SW" || s.IFUp != 1 || s.IFDown != 1 {
		t.Fatalf("unexpected snapshot: %+v", s)
	}
	if !s.DefaultRouter || s.LayerMode != "3" || !s.SSHEnabled || s.TelnetEnabled {
		t.Fatalf("unexpected capabilities: %+v", s)
	}
	if s.UserCount != 2 || !strings.Contains(strings.Join(s.Users, ","), "fiberx") {
		t.Fatalf("unexpected users: %+v", s)
	}
}

func TestParseCiscoVersionNexus(t *testing.T) {
	host, model, version, serial, uptime := parseCiscoVersion(map[string]string{
		"show version": `Cisco Nexus Operating System (NX-OS) Software
TAC support: http://www.cisco.com/tac
Copyright (C) 2002-2019, Cisco and/or its affiliates.
Certain components of this software are licensed under
the GNU General Public License (GPL) version 2.0 or
GNU General Public License (GPL) version 3.0  or the GNU

Software
  BIOS: version 07.68
 NXOS: version 9.3(3)

Hardware
  cisco Nexus9000 C9396PX Chassis
  Processor Board ID SAL1820SDQY

  Device name: HILQSIM-REP
Kernel uptime is 82 day(s), 3 hour(s), 44 minute(s), 29 second(s)`,
	})

	if host != "HILQSIM-REP" || model != "Nexus9000 C9396PX" || version != "9.3(3)" || serial != "SAL1820SDQY" || uptime != "82 day(s), 3 hour(s), 44 minute(s), 29 second(s)" {
		t.Fatalf("unexpected nexus parsed fields host=%q model=%q version=%q serial=%q uptime=%q", host, model, version, serial, uptime)
	}
}

func TestParseCiscoVersionCatalyst4500(t *testing.T) {
	host, model, version, serial, uptime := parseCiscoVersion(map[string]string{
		"show version": `Cisco IOS Software, Catalyst 4500 L3 Switch Software (cat4500e-IPBASEK9-M), Version 15.1(1)SG2, RELEASE SOFTWARE (fc1)
Technical Support: http://www.cisco.com/techsupport
OSAMA-HOME uptime is 34 weeks, 5 days, 14 hours, 7 minutes
System image file is "bootflash:cat4500e-ipbasek9-mz.151-1.SG2.bin"
cisco WS-C4948E-F (MPC8548) processor (revision 8) with 1048576K bytes of memory.
Processor board ID CAT1711S3UL`,
	})

	if host != "OSAMA-HOME" || model != "WS-C4948E-F" || version != "15.1(1)SG2" || serial != "CAT1711S3UL" || uptime != "34 weeks, 5 days, 14 hours, 7 minutes" {
		t.Fatalf("unexpected ios parsed fields host=%q model=%q version=%q serial=%q uptime=%q", host, model, version, serial, uptime)
	}
}

func TestParseCiscoVersionLegacyIOSFormat(t *testing.T) {
	host, model, version, serial, uptime := parseCiscoVersion(map[string]string{
		"show version": `Cisco Internetwork Operating System Software
IOS (tm) C2600 Software (C2600-IK9O3S-M), Version 12.3(26), RELEASE SOFTWARE (fc2)
Copyright (c) 1986-2007 by cisco Systems, Inc.
HILLA-EDGE uptime is 12 weeks, 1 day, 2 hours, 9 minutes
cisco 2621XM (MPC860P) processor (revision 0x500) with 61440K/4096K bytes of memory.
Processor board ID JAE064512AB`,
	})

	if host != "HILLA-EDGE" || model != "2621XM" || version != "12.3(26)" || serial != "JAE064512AB" || uptime != "12 weeks, 1 day, 2 hours, 9 minutes" {
		t.Fatalf("unexpected legacy ios parsed fields host=%q model=%q version=%q serial=%q uptime=%q", host, model, version, serial, uptime)
	}
}

func TestCollectSnapshotMikrotik(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "mikrotik"}
	sections := map[string]string{
		"/system identity print":                      "name: MT-EDGE-1",
		"/system resource print":                      "uptime: 5d2h\nversion: 7.15\nboard-name: RB5009",
		"/system routerboard print":                   "model: RB5009UG+S+\nserial-number: ABC123",
		"/interface print":                            " 0 R ether1 ether 1500\n 1   ether2 ether 1500\n 2 RS ether3 ether 1500\n 3   vlan10 vlan 1500",
		"/ip route print where dst-address=0.0.0.0/0": "0 ADS dst-address=0.0.0.0/0 gateway=10.0.0.1",
		"/user print detail":                          "0 name=\"admin\" group=full\n1 name=\"fiberx\" group=full",
		"/ip service print detail":                    "0 name=telnet disabled=yes\n1 name=ssh disabled=no",
		"/snmp print detail":                          "enabled: yes",
		"/system ntp client print detail":             "enabled: yes",
		"/radius print detail":                        "0 service=login address=1.1.1.1 secret=x",
		"/system logging print":                       "0 topics=info action=memory",
	}
	s := CollectSnapshot(d, sections)
	if s.Hostname != "MT-EDGE-1" || s.Model != "RB5009UG+S+" || s.Serial != "ABC123" {
		t.Fatalf("unexpected identity fields: %+v", s)
	}
	if s.IFUp != 2 || s.IFDown != 1 || !s.DefaultRouter {
		t.Fatalf("unexpected interface/default route data: %+v", s)
	}
	if s.LayerMode != "3" || !s.SSHEnabled || s.TelnetEnabled || !s.SNMPEnabled || !s.NTPEnabled || !s.AAAEnabled || !s.SyslogEnabled {
		t.Fatalf("unexpected service data: %+v", s)
	}
}

func TestCollectSnapshotMikrotikEqualsStyleOutput(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "mikrotik"}
	sections := map[string]string{
		"/system identity print":                      "name=MT-EDGE-2",
		"/system resource print":                      "uptime=6d4h\nversion=7.18.2\nboard-name=CCR2004",
		"/system routerboard print":                   "model=CCR2004-16G-2S+\nserial-number=XYZ987",
		"/interface print":                            " 0 RS ether1 ether 1500\n 1    ether2 ether 1500\n 2    vlan10 vlan 1500",
		"/ip route print where dst-address=0.0.0.0/0": "dst-address=0.0.0.0/0 gateway=10.0.0.1",
		"/user print detail":                          "0 name=NOC group=full",
		"/ip service print detail":                    "0 name=telnet disabled=yes\n1 name=ssh disabled=no",
		"/snmp print detail":                          "enabled=yes",
		"/system ntp client print detail":             "enabled=yes",
		"/radius print detail":                        "0 service=login address=1.1.1.1 secret=x",
		"/system logging print":                       "0 topics=info action=memory",
	}

	s := CollectSnapshot(d, sections)
	if s.Status != "ok" {
		t.Fatalf("expected ok status, got %+v", s)
	}
	if s.Hostname != "MT-EDGE-2" || s.Model != "CCR2004-16G-2S+" || s.Version != "7.18.2" || s.Serial != "XYZ987" || s.Uptime != "6d4h" {
		t.Fatalf("unexpected parsed identity/version fields: %+v", s)
	}
}

func TestCollectSnapshotMikrotikDoesNotFailWhenNoEthernetInterfacesExist(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "mikrotik"}
	sections := map[string]string{
		"/system identity print":                      "name=KRB-EOIP1-DC",
		"/system resource print":                      "uptime=12w9h50m55s\nversion=7.19.6\nboard-name=CCR2116-12G-4S+",
		"/system routerboard print":                   "model=CCR2116-12G-4S+\nserial-number=HF10994CYQY",
		"/interface print":                            " 0 R eoip-tunnel eoip 1500\n 1 R vlan10 vlan 1500\n 2 R bridge1 bridge 1500",
		"/ip route print where dst-address=0.0.0.0/0": "dst-address=0.0.0.0/0 gateway=10.0.0.1",
		"/user print detail":                          "0 name=NOC group=full",
		"/ip service print detail":                    "0 name=telnet disabled=no\n1 name=ssh disabled=no",
		"/snmp print detail":                          "enabled=yes",
		"/system ntp client print detail":             "enabled=yes",
		"/radius print detail":                        "0 service=login address=1.1.1.1 secret=x",
		"/system logging print":                       "0 topics=info action=memory",
	}

	s := CollectSnapshot(d, sections)
	if s.Status != "ok" {
		t.Fatalf("expected ok status when interface output exists but has no ethernet ports, got %+v", s)
	}
	if s.IFUp != 0 || s.IFDown != 0 {
		t.Fatalf("expected zero ethernet counts, got %+v", s)
	}
}

func TestProbeOutputLooksValid(t *testing.T) {
	if !ProbeOutputLooksValid("cisco_ios", "show version", "EDGE-SW uptime is 3 weeks\nCisco IOS Software, Version 15.2(7)E") {
		t.Fatal("expected Cisco probe output to be accepted")
	}
	if ProbeOutputLooksValid("cisco_ios", "show version", "bash: show: command not found") {
		t.Fatal("expected shell error output to be rejected for Cisco probe")
	}
	if !ProbeOutputLooksValid("mikrotik", "/system identity print", "name=MT-EDGE-2") {
		t.Fatal("expected MikroTik probe output to be accepted")
	}
	if ProbeOutputLooksValid("mikrotik", "/system identity print", "bad command name identity") {
		t.Fatal("expected invalid MikroTik probe output to be rejected")
	}
	if ProbeOutputLooksValid("mikrotik", "/system identity print", "% Authorization failed.") {
		t.Fatal("expected MikroTik authorization failure to be rejected")
	}
	if ProbeOutputLooksValid("cisco_ios", "show version", "% Authorization failed.") {
		t.Fatal("expected Cisco authorization failure to be rejected")
	}
	if !ProbeOutputLooksValid("huawei", "display version", "Huawei Versatile Routing Platform Software\nVRP (R) software, Version 5.170 (S6730 V200R024C00SPC500)\nHUAWEI S6730-H48X6C Routing Switch uptime is 18 weeks, 3 days") {
		t.Fatal("expected Huawei probe output to be accepted")
	}
	if ProbeOutputLooksValid("huawei", "display version", "Error: Unrecognized command found at '^' position.") {
		t.Fatal("expected invalid Huawei probe output to be rejected")
	}
}

func TestCollectSnapshotHuawei(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "huawei"}
	sections := map[string]string{
		"display version": `<AMAL-CUST>display version
Huawei Versatile Routing Platform Software
VRP (R) software, Version 5.170 (S6730 V200R024C00SPC500)
Copyright (C) 2000-2024 HUAWEI TECH Co., Ltd.
HUAWEI S6730-H48X6C Routing Switch uptime is 18 weeks, 3 days, 23 hours, 33 minutes

ES6D2S54S003 0(Master)  : uptime is 18 weeks, 3 days, 23 hours, 31 minutes`,
		"display elabel": `<AMAL-CUST>display elabel
Info: It is executing, please wait...
/$[System Integration Version]
/$SystemIntegrationVersion=3.0

[Slot_0]
/$[Board Integration Version]
/$BoardIntegrationVersion=3.0

[Main_Board]

/$[ArchivesInfo Version]
/$ArchivesInfoVersion=3.0

[Board Properties]
BoardType=S6730-H48X6C
BarCode=6R24C0010836
Item=02352FSF-009
Description=Assembling Components,S6730-H48X6C
Manufactured=2024-12-12
VendorName=Huawei
IssueNumber=00
Model=S6730-H48X6C
/$ElabelVersion=4.0

[Port_XGigabitEthernet0/0/1]
/$[ArchivesInfo Version]
/$ArchivesInfoVersion=3.0

[Board Properties]
BoardType=SFP-10G-LR
BarCode=FNS27230AL3
Description=10300Mbps-1310nm-LC-
Manufactured=2024-03-21
/$VendorName=CISCO-FINISAR`,
		"display interface description | include GE": `PHY: Physical
Interface                     PHY     Protocol Description
XGE0/0/1                      up      up       A
XGE0/0/2                      down    down
XGE0/0/3                      up      up       B`,
		"display ip routing-table": `Route Flags: R - relay, D - download to fib, T - to vpn-instance
Destination/Mask    Proto   Pre  Cost      Flags NextHop         Interface
0.0.0.0/0   ISIS-L2 15   100         D   10.90.20.49     XGigabitEthernet0/0/25`,
		"display current-configuration": `stelnet server enable
telnet server enable
snmp-agent
ntp-service enable
aaa
info-center loghost 10.1.1.1
local-user noc password irreversible-cipher x
local-user read password irreversible-cipher y
user-interface vty 0 4
 protocol inbound all`,
	}

	s := CollectSnapshot(d, sections)
	if s.Status != "ok" {
		t.Fatalf("expected ok Huawei snapshot, got %+v", s)
	}
	if s.Hostname != "AMAL-CUST" {
		t.Fatalf("unexpected Huawei hostname: %+v", s)
	}
	if s.Model != "S6730-H48X6C" || s.Serial != "6R24C0010836" {
		t.Fatalf("unexpected Huawei identity fields: %+v", s)
	}
	if s.Version != "4.0" || s.Uptime != "18 weeks, 3 days, 23 hours, 33 minutes" {
		t.Fatalf("unexpected Huawei version/uptime fields: %+v", s)
	}
	if s.IFUp != 2 || s.IFDown != 1 || !s.DefaultRouter || s.LayerMode != "3" {
		t.Fatalf("unexpected Huawei interface/layer data: %+v", s)
	}
	if !s.SSHEnabled || !s.TelnetEnabled || !s.SNMPEnabled || !s.NTPEnabled || !s.AAAEnabled || !s.SyslogEnabled {
		t.Fatalf("unexpected Huawei service flags: %+v", s)
	}
	if s.UserCount != 2 {
		t.Fatalf("unexpected Huawei users: %+v", s)
	}
}

func TestCommandsForVendorHuaweiUsesElabelAndDisablesPaging(t *testing.T) {
	set, cmds, err := CommandsForVendor("huawei")
	if err != nil {
		t.Fatalf("CommandsForVendor returned error: %v", err)
	}
	if set.HardwareID != "display elabel" {
		t.Fatalf("expected Huawei hardware command to be display elabel, got %q", set.HardwareID)
	}
	if len(cmds) == 0 || cmds[0] != "screen-length 0 temporary" {
		t.Fatalf("expected first Huawei command to disable paging, got %v", cmds)
	}
}

func TestCollectSnapshotMikrotikDoesNotTreatAuthorizationFailureAsHostname(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "mikrotik"}
	sections := map[string]string{
		"/system identity print":                      "% Authorization failed.",
		"/system resource print":                      "uptime=6d4h\nversion=7.18.2\nboard-name=CCR2004",
		"/system routerboard print":                   "model=CCR2004-16G-2S+\nserial-number=XYZ987",
		"/interface print":                            "0 RS ether1 ether 1500",
		"/ip service print detail":                    "% Authorization failed.",
		"/system ntp client print detail":             "% Authorization failed.",
		"/ip route print where dst-address=0.0.0.0/0": "% Authorization failed.",
	}

	s := CollectSnapshot(d, sections)
	if s.Hostname != "" {
		t.Fatalf("expected empty hostname when identity output is authorization failure, got %+v", s)
	}
}

func TestCollectSnapshotMikrotikFailsWhenCoreDataMissing(t *testing.T) {
	d := &models.NocDataDevice{Vendor: "mikrotik"}
	sections := map[string]string{
		"/system identity print":          "",
		"/system resource print":          "",
		"/system routerboard print":       "",
		"/interface print":                "",
		"/ip service print detail":        "",
		"/snmp print detail":              "",
		"/system ntp client print detail": "",
		"/radius print detail":            "",
		"/system logging print":           "",
	}

	s := CollectSnapshot(d, sections)
	if s.Status != "fail" {
		t.Fatalf("expected fail status, got %+v", s)
	}
	if !strings.Contains(s.Error, "collection completed but parsed data incomplete:") {
		t.Fatalf("expected incomplete data error, got %+v", s)
	}
	if !strings.Contains(s.Error, "missing hostname") || !strings.Contains(s.Error, "missing resource output") {
		t.Fatalf("expected core missing details, got %+v", s)
	}
}

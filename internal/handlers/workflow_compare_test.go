package handlers

import "testing"

func TestCompareBackupLinesIgnoresVolatileBackupMetadata(t *testing.T) {
	base := `# 2026-04-02 00:35:39 by RouterOS 7.21.2
! Last configuration change at 11:38:36 UTC Sun Apr 5 2026 by mustafa.m
! NVRAM config last updated at 11:38:42 UTC Sun Apr 5 2026 by mustafa.m
Current configuration : 5657 bytes
ntp clock-period 36028985
transport input ss
switchport access vlan 475`

	compare := `# 2026-04-03 00:35:39 by RouterOS 7.21.2
! No configuration change since last restart
Current configuration : 7039 bytes
ntp clock-period 36029609
transport input ssh
switchport access vlan 463`

	added, removed := compareBackupLines(base, compare)
	if len(added) != 1 || added[0].Text != "switchport access vlan 463" {
		t.Fatalf("added = %#v, want only vlan 463", added)
	}
	if len(removed) != 1 || removed[0].Text != "switchport access vlan 475" {
		t.Fatalf("removed = %#v, want only vlan 475", removed)
	}
}

func TestCompareBackupLinesReturnsNoChangesForOnlyVolatileMetadata(t *testing.T) {
	base := `# 2026-04-02 00:35:39 by RouterOS 7.21.2
! Last configuration change at 11:38:36 UTC Sun Apr 5 2026 by mustafa.m
Current configuration : 5657 bytes
ntp clock-period 36028985
transport input ss`

	compare := `# 2026-04-03 00:35:39 by RouterOS 7.21.2
! No configuration change since last restart
Current configuration : 7039 bytes
ntp clock-period 36029609
transport input ssh`

	added, removed := compareBackupLines(base, compare)
	if len(added) != 0 || len(removed) != 0 {
		t.Fatalf("expected no line-level changes, added=%#v removed=%#v", added, removed)
	}
}

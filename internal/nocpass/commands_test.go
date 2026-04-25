package nocpass

import (
	"strings"
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
)

func TestExtractCiscoUsernames(t *testing.T) {
	out := `
username fiberx privilege 15 secret 0 x
username support privilege 15 secret 0 x
username AdminUser privilege 15 secret 0 x
`
	got := ExtractCiscoUsernames(out)
	if len(got) != 3 {
		t.Fatalf("expected 3 usernames, got %d (%v)", len(got), got)
	}
	if got[2] != "AdminUser" {
		t.Fatalf("expected original username case to be preserved, got %q", got[2])
	}
}

func TestExtractHuaweiUsernames(t *testing.T) {
	out := `
  0   CON 0                                                                       Username : Unspecified
+ 34  VTY 0   00:00:00  SSH    188.72.40.218             pass           no        Username : NOC
  35  VTY 1                                                                       Username : Unspecified
  36  VTY 2                                                                       Username : support
  37  VTY 3                                                                       Username : AdminUser
`
	got := ExtractHuaweiUsernames(out)
	if len(got) != 3 {
		t.Fatalf("expected 3 usernames, got %d (%v)", len(got), got)
	}
	if got[0] != "NOC" || got[2] != "AdminUser" {
		t.Fatalf("expected Huawei usernames to preserve case, got %v", got)
	}
}

func TestUserDiscoveryCommandsHuawei(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "huawei"}
	cmds, err := UserDiscoveryCommands(device)
	if err != nil {
		t.Fatalf("UserDiscoveryCommands error: %v", err)
	}
	if len(cmds) != 1 {
		t.Fatalf("expected 1 Huawei discovery command, got %d (%v)", len(cmds), cmds)
	}
	if cmds[0] != "display users all" {
		t.Fatalf("unexpected Huawei discovery commands: %v", cmds)
	}
}

func TestBuildCommandListCiscoProtectsSupportDevAndKeepUsers(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "cisco_ios"}
	cmds, err := BuildCommandList(device, "pw123", "pw456", false, []string{"fiberx", "support", "dev", "TempUser", "AdminUser", "tempuser"}, []string{"AdminUser"})
	if err != nil {
		t.Fatalf("BuildCommandList error: %v", err)
	}
	joined := strings.Join(cmds, "\n")
	if strings.Contains(joined, "no username dev") {
		t.Fatalf("dev user should be protected from deletion: %s", joined)
	}
	if strings.Contains(joined, "no username support") {
		t.Fatalf("support user should be protected from deletion: %s", joined)
	}
	if strings.Contains(joined, "no username AdminUser") {
		t.Fatalf("keep user should be protected from deletion: %s", joined)
	}
	if !strings.Contains(joined, "no username TempUser") {
		t.Fatalf("unexpected cleanup commands: %s", joined)
	}
	if strings.Count(joined, "no username TempUser") != 1 {
		t.Fatalf("expected one delete command per stale username, got: %s", joined)
	}
	if !strings.Contains(joined, "username support privilege 15 secret 0 pw456") {
		t.Fatalf("support account was not created as expected: %s", joined)
	}
}

func TestBuildCommandListMikrotikKeepsDevWithoutRotatingIt(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "mikrotik"}
	cmds, err := BuildCommandList(device, "pw123", "pw456", false, nil, []string{"nocadmin"})
	if err != nil {
		t.Fatalf("BuildCommandList error: %v", err)
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, `name!="dev"`) {
		t.Fatalf("dev should be protected in MikroTik cleanup: %s", joined)
	}
	if !strings.Contains(joined, `name!="support"`) {
		t.Fatalf("support should be protected in MikroTik cleanup: %s", joined)
	}
	if strings.Contains(joined, `/user set [find name=dev]`) {
		t.Fatalf("dev password must stay unchanged: %s", joined)
	}
}

func TestBuildCommandListHuaweiCreatesSupportUser(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "huawei"}
	cmds, err := BuildCommandList(device, "pw123", "pw456", false, []string{"fiberx", "support", "TempUser", "tempuser"}, []string{"AdminUser"})
	if err != nil {
		t.Fatalf("BuildCommandList error: %v", err)
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, "undo local-user TempUser") {
		t.Fatalf("expected stale Huawei user cleanup, got: %s", joined)
	}
	if strings.Count(joined, "undo local-user TempUser") != 1 {
		t.Fatalf("expected one Huawei delete command per stale username, got: %s", joined)
	}
	if strings.Contains(joined, "undo local-user support") {
		t.Fatalf("support should be protected on Huawei: %s", joined)
	}
	if strings.Contains(joined, "system-view") {
		t.Fatalf("Huawei command list should not require system-view anymore: %s", joined)
	}
	if !strings.Contains(joined, "local-user support password irreversible-cipher pw456") {
		t.Fatalf("support Huawei account missing: %s", joined)
	}
	if !strings.Contains(joined, "local-user support privilege level 3") {
		t.Fatalf("support Huawei privilege missing: %s", joined)
	}
}

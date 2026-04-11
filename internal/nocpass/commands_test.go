package nocpass

import (
	"strings"
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
)

func TestExtractCiscoUsernames(t *testing.T) {
	out := `
username fiberx privilege 15 secret 0 x
username readOnly privilege 13 secret 0 x
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

func TestBuildCommandListCiscoProtectsDevAndKeepUsers(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "cisco_ios"}
	cmds, err := BuildCommandList(device, "pw123", false, []string{"fiberx", "readOnly", "dev", "temp1", "AdminUser"}, []string{"AdminUser"})
	if err != nil {
		t.Fatalf("BuildCommandList error: %v", err)
	}
	joined := strings.Join(cmds, "\n")
	if strings.Contains(joined, "no username dev") {
		t.Fatalf("dev user should be protected from deletion: %s", joined)
	}
	if strings.Contains(joined, "no username AdminUser") {
		t.Fatalf("keep user should be protected from deletion: %s", joined)
	}
	if !strings.Contains(joined, "no username temp1") {
		t.Fatalf("unexpected cleanup commands: %s", joined)
	}
	if strings.Contains(joined, "username dev") {
		t.Fatalf("dev user should not be rotated or recreated: %s", joined)
	}
}

func TestBuildCommandListMikrotikKeepsDevWithoutRotatingIt(t *testing.T) {
	device := &models.NocPassDevice{Vendor: "mikrotik"}
	cmds, err := BuildCommandList(device, "pw123", false, nil, []string{"nocadmin"})
	if err != nil {
		t.Fatalf("BuildCommandList error: %v", err)
	}
	joined := strings.Join(cmds, "\n")
	if !strings.Contains(joined, `name!="dev"`) {
		t.Fatalf("dev should be protected in MikroTik cleanup: %s", joined)
	}
	if strings.Contains(joined, `/user set [find name=dev]`) {
		t.Fatalf("dev password must stay unchanged: %s", joined)
	}
}

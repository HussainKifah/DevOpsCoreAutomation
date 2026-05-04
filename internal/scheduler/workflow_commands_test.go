package scheduler

import (
	"reflect"
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
)

func TestWorkflowCommandLines(t *testing.T) {
	t.Parallel()

	got := workflowCommandLines(" conf t \r\n\r\nip access-list VTY_MGMT_ACCESS\n  10 permit 10.130.30.0 0.0.0.255  \nline vty 0 15\nend\n")
	want := []string{
		"conf t",
		"ip access-list VTY_MGMT_ACCESS",
		"10 permit 10.130.30.0 0.0.0.255",
		"line vty 0 15",
		"end",
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("workflowCommandLines() = %#v, want %#v", got, want)
	}
}

func TestWorkflowSchedulerTransportMethodForIPBackups(t *testing.T) {
	t.Parallel()

	ws := &WorkflowScheduler{scope: "ip"}

	if got := ws.transportMethod(&models.WorkflowJob{JobType: "backup"}); got != "ssh" {
		t.Fatalf("transportMethod(ip backup) = %q, want ssh", got)
	}
	if got := ws.transportMethod(&models.WorkflowJob{JobType: "command"}); got != "" {
		t.Fatalf("transportMethod(ip command) = %q, want empty auto method", got)
	}
}

func TestWorkflowSchedulerTransportMethodKeepsNOCBackupsAuto(t *testing.T) {
	t.Parallel()

	ws := &WorkflowScheduler{scope: "noc"}

	if got := ws.transportMethod(&models.WorkflowJob{JobType: "backup"}); got != "" {
		t.Fatalf("transportMethod(noc backup) = %q, want empty auto method", got)
	}
}

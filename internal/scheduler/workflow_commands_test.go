package scheduler

import (
	"reflect"
	"testing"
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

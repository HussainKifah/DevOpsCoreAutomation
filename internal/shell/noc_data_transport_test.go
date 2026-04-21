package shell

import (
	"net"
	"testing"
	"time"
)

func TestProcessTelnetChunkNegotiatesEchoAndPreservesPayload(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	s := &nocDataTelnetSession{
		conn: client,
		host: "test-host",
	}

	replyCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		if err := server.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			errCh <- err
			return
		}
		reply := make([]byte, 3)
		if _, err := server.Read(reply); err != nil {
			errCh <- err
			return
		}
		replyCh <- reply
	}()

	chunk := []byte{
		telnetIAC, telnetWILL, telnetOptEcho,
		'U', 's', 'e', 'r', 'n', 'a', 'm', 'e', ':',
	}
	payload := s.processTelnetChunk(chunk)
	if got := string(payload); got != "Username:" {
		t.Fatalf("unexpected payload %q", got)
	}

	var reply []byte
	select {
	case err := <-errCh:
		t.Fatalf("read reply: %v", err)
	case reply = <-replyCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for telnet negotiation reply")
	}
	expected := []byte{telnetIAC, telnetDO, telnetOptEcho}
	for i := range expected {
		if reply[i] != expected[i] {
			t.Fatalf("unexpected telnet reply %v", reply)
		}
	}
}

func TestHuaweiWorkflowPromptRegexes(t *testing.T) {
	if !huaweiWorkflowContinueRe.MatchString("Warning: It may take a long time to execute this command. Continue? [Y/N]:") {
		t.Fatal("expected Huawei continue prompt to be detected")
	}
	if !huaweiWorkflowContinueRe.MatchString("Error: Please choose 'YES' or 'NO' first before pressing 'Enter'. [Y/N]:y") {
		t.Fatal("expected Huawei retry continue prompt to be detected")
	}
	if !huaweiWorkflowMoreRe.MatchString("---- More ----") {
		t.Fatal("expected Huawei pager prompt to be detected")
	}
}

package handlers

import "testing"

func TestIsPauseReaction(t *testing.T) {
	for _, reaction := range []string{"hourglass", "hourglass_flowing_sand", "hourglass_done", "sand_clock"} {
		if !isPauseReaction(reaction) {
			t.Fatalf("expected %q to pause alerts", reaction)
		}
	}

	for _, reaction := range []string{"white_check_mark", "alarm_clock", "", "thumbsup"} {
		if isPauseReaction(reaction) {
			t.Fatalf("did not expect %q to pause alerts", reaction)
		}
	}
}

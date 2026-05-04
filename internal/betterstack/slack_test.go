package betterstack

import (
	"strings"
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
)

func TestBuildIncidentAttachmentIncludesIncidentDetails(t *testing.T) {
	att := BuildIncidentAttachment(Incident{
		ID:        "123",
		Name:      "OLT status check",
		Cause:     "Status 500",
		URL:       "https://example.test/health",
		Status:    "Started",
		TeamName:  "NOC",
		StartedAt: time.Date(2026, 5, 4, 1, 2, 3, 0, time.UTC),
	})

	if att.Color != colorAlert {
		t.Fatalf("color = %q", att.Color)
	}
	if !strings.Contains(att.Fallback, "OLT status check") {
		t.Fatalf("fallback = %q", att.Fallback)
	}
	if len(att.Blocks.BlockSet) < 3 {
		t.Fatalf("blocks = %d", len(att.Blocks.BlockSet))
	}
}

func TestBuildResolvedAttachmentUsesResolvedMetadata(t *testing.T) {
	att := BuildResolvedAttachment(models.BetterStackSlackIncident{
		BetterStackIncidentID: "123",
		Name:                  "OLT status check",
	}, time.Date(2026, 5, 4, 3, 0, 0, 0, time.UTC), "ops@example.com")

	if att.Color != colorResolved {
		t.Fatalf("color = %q", att.Color)
	}
	if !strings.Contains(att.Fallback, "Resolved") {
		t.Fatalf("fallback = %q", att.Fallback)
	}
}

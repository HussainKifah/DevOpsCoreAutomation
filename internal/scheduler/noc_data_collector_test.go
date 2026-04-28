package scheduler

import (
	"testing"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

func TestSelectNocDataDuplicateKeepKeepsHighestIPv4(t *testing.T) {
	matches := []models.NocDataDevice{
		{Model: gorm.Model{ID: 10}, Host: "10.90.130.1"},
		{Model: gorm.Model{ID: 20}, Host: "10.90.255.1"},
		{Model: gorm.Model{ID: 30}, Host: "10.90.200.1"},
	}

	keep := selectNocDataDuplicateKeep(matches)

	if keep.ID != 20 {
		t.Fatalf("expected highest IP row id=20 to be kept, got id=%d host=%q", keep.ID, keep.Host)
	}
}

func TestSelectNocDataDuplicateKeepUsesIDTieBreakerForSameIP(t *testing.T) {
	matches := []models.NocDataDevice{
		{Model: gorm.Model{ID: 30}, Host: "10.90.255.1"},
		{Model: gorm.Model{ID: 20}, Host: "10.90.255.1"},
	}

	keep := selectNocDataDuplicateKeep(matches)

	if keep.ID != 20 {
		t.Fatalf("expected lowest id to break same-IP tie, got id=%d", keep.ID)
	}
}

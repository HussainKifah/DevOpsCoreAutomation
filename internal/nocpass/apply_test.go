package nocpass

import (
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
)

type fakeNocPassRepo struct {
	policy *models.NocPassPolicy
}

func (f *fakeNocPassRepo) ListPolicies() ([]models.NocPassPolicy, error) {
	if f.policy == nil {
		return nil, nil
	}
	return []models.NocPassPolicy{*f.policy}, nil
}
func (f *fakeNocPassRepo) GetPolicy(id uint) (*models.NocPassPolicy, error) { return f.policy, nil }
func (f *fakeNocPassRepo) CreatePolicy(policy *models.NocPassPolicy) error {
	f.policy = policy
	return nil
}
func (f *fakeNocPassRepo) SavePolicy(policy *models.NocPassPolicy) error {
	f.policy = policy
	return nil
}
func (f *fakeNocPassRepo) DeletePolicy(id uint) error                           { return nil }
func (f *fakeNocPassRepo) ListExclusions() ([]models.NocPassExclusion, error)   { return nil, nil }
func (f *fakeNocPassRepo) CreateExclusion(e *models.NocPassExclusion) error     { return nil }
func (f *fakeNocPassRepo) DeleteExclusion(id uint) error                        { return nil }
func (f *fakeNocPassRepo) Search(q string) ([]models.NocPassDevice, error)      { return nil, nil }
func (f *fakeNocPassRepo) ListStatuses() ([]models.NocPassDevice, error)        { return nil, nil }
func (f *fakeNocPassRepo) GetByID(id uint) (*models.NocPassDevice, error)       { return nil, nil }
func (f *fakeNocPassRepo) GetByHost(host string) (*models.NocPassDevice, error) { return nil, nil }
func (f *fakeNocPassRepo) TouchHostState(displayName, host, vendor string) (*models.NocPassDevice, error) {
	return &models.NocPassDevice{DisplayName: displayName, Host: host, Vendor: vendor, Enabled: true}, nil
}
func (f *fakeNocPassRepo) Delete(id uint) error                              { return nil }
func (f *fakeNocPassRepo) ListKeepUsers() ([]models.NocPassKeepUser, error)  { return nil, nil }
func (f *fakeNocPassRepo) CreateKeepUser(user *models.NocPassKeepUser) error { return nil }
func (f *fakeNocPassRepo) DeleteKeepUser(id uint) error                      { return nil }
func (f *fakeNocPassRepo) ListSavedUsers() ([]models.NocPassSavedUser, error) {
	return nil, nil
}
func (f *fakeNocPassRepo) GetSavedUser(id uint) (*models.NocPassSavedUser, error) {
	return nil, nil
}
func (f *fakeNocPassRepo) CreateSavedUser(user *models.NocPassSavedUser) error { return nil }
func (f *fakeNocPassRepo) DeleteSavedUser(id uint) error                       { return nil }
func (f *fakeNocPassRepo) UpdateAfterApply(id uint, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error {
	return nil
}
func (f *fakeNocPassRepo) UpdateAfterApplyByHost(displayName, host, vendor string, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error {
	return nil
}

func TestEnsurePolicyPasswordsRandomReusesSameDayPasswords(t *testing.T) {
	repo := &fakeNocPassRepo{policy: &models.NocPassPolicy{PasswordMode: "random"}}
	key := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)

	first, changed, err := EnsurePolicyPasswords(repo, key, repo.policy, now)
	if err != nil {
		t.Fatalf("first EnsurePolicyPasswords failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected first random password generation to mark changed")
	}

	second, changed, err := EnsurePolicyPasswords(repo, key, repo.policy, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("second EnsurePolicyPasswords failed: %v", err)
	}
	if changed {
		t.Fatalf("expected same-day random password reuse without change")
	}
	if first.Fiberx != second.Fiberx || first.Support != second.Support {
		t.Fatalf("expected same-day random password reuse, got %#v vs %#v", first, second)
	}
	if first.Fiberx == first.Support {
		t.Fatalf("expected separate daily passwords for fiberx and support")
	}
}

func TestEnsurePolicyPasswordsRandomChangesNextDay(t *testing.T) {
	repo := &fakeNocPassRepo{policy: &models.NocPassPolicy{PasswordMode: "random"}}
	key := []byte("01234567890123456789012345678901")
	day1 := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)
	day2 := day1.Add(24 * time.Hour)

	first, _, err := EnsurePolicyPasswords(repo, key, repo.policy, day1)
	if err != nil {
		t.Fatalf("day1 EnsurePolicyPasswords failed: %v", err)
	}
	second, changed, err := EnsurePolicyPasswords(repo, key, repo.policy, day2)
	if err != nil {
		t.Fatalf("day2 EnsurePolicyPasswords failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected next-day random password regeneration")
	}
	if first.Fiberx == second.Fiberx {
		t.Fatalf("expected next-day fiberx password to change")
	}
	if first.Support == second.Support {
		t.Fatalf("expected next-day support password to change")
	}
}

func TestEnsurePolicyPasswordsManualPersistsAcrossDays(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	fiberxEnc, err := crypto.Encrypt(key, "FiberxManual123")
	if err != nil {
		t.Fatalf("encrypt fiberx manual password: %v", err)
	}
	supportEnc, err := crypto.Encrypt(key, "SupportManual123")
	if err != nil {
		t.Fatalf("encrypt support manual password: %v", err)
	}
	repo := &fakeNocPassRepo{policy: &models.NocPassPolicy{PasswordMode: "manual", EncManualFiberxPassword: fiberxEnc, EncManualSupportPassword: supportEnc}}

	day1 := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)
	day2 := day1.Add(24 * time.Hour)

	first, _, err := EnsurePolicyPasswords(repo, key, repo.policy, day1)
	if err != nil {
		t.Fatalf("day1 EnsurePolicyPasswords failed: %v", err)
	}
	second, _, err := EnsurePolicyPasswords(repo, key, repo.policy, day2)
	if err != nil {
		t.Fatalf("day2 EnsurePolicyPasswords failed: %v", err)
	}
	if first.Fiberx != "FiberxManual123" || second.Fiberx != "FiberxManual123" {
		t.Fatalf("expected fiberx manual password to persist, got %#v and %#v", first, second)
	}
	if first.Support != "SupportManual123" || second.Support != "SupportManual123" {
		t.Fatalf("expected support manual password to persist, got %#v and %#v", first, second)
	}
}

func TestResolvePolicyTargets(t *testing.T) {
	rows := []models.NocDataDevice{
		{Host: "10.0.0.1", Site: "Basra FTTH", Vendor: "mikrotik", DeviceModel: "CCR", Hostname: "basra-1"},
		{Host: "10.0.0.2", Site: "Basra WiFi", Vendor: "cisco_ios", DeviceModel: "N9K-1", Hostname: "basra-2"},
		{Host: "10.0.0.3", Site: "Najaf FTTH", Vendor: "huawei", DeviceModel: "NE40E", Hostname: "najaf-1"},
	}

	if got := ResolvePolicyTargets(rows, TargetAllNetworks, "all"); len(got) != 3 {
		t.Fatalf("all networks expected 3 rows, got %d", len(got))
	}
	if got := ResolvePolicyTargets(rows, TargetNetworkType, "ftth"); len(got) != 2 {
		t.Fatalf("ftth expected 2 rows, got %d", len(got))
	}
	if got := ResolvePolicyTargets(rows, TargetProvince, "Basra"); len(got) != 2 {
		t.Fatalf("Basra expected 2 rows, got %d", len(got))
	}
	if got := ResolvePolicyTargets(rows, TargetVendor, "cisco_nexus"); len(got) != 1 {
		t.Fatalf("cisco_nexus expected 1 row, got %d", len(got))
	}
	if got := ResolvePolicyTargets(rows, TargetModel, "CCR"); len(got) != 1 {
		t.Fatalf("model CCR expected 1 row, got %d", len(got))
	}
	if got := ResolvePolicyTargets(rows, TargetDevice, "10.0.0.3"); len(got) != 1 || got[0].Host != "10.0.0.3" {
		t.Fatalf("device target expected host 10.0.0.3, got %#v", got)
	}
}

package nocpass

import (
	"testing"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
)

type fakeNocPassRepo struct {
	policy   *models.NocPassPolicy
	policies []models.NocPassPolicy
	creds    map[string]models.NocPassCredential
}

func (f *fakeNocPassRepo) ListPolicies() ([]models.NocPassPolicy, error) {
	if len(f.policies) > 0 {
		return f.policies, nil
	}
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
func (f *fakeNocPassRepo) ListCredentials() ([]models.NocPassCredential, error) {
	out := make([]models.NocPassCredential, 0, len(f.creds))
	for _, item := range f.creds {
		out = append(out, item)
	}
	return out, nil
}
func (f *fakeNocPassRepo) UpsertCredential(host, username, source string, savedUserID *uint, encPassword []byte, appliedAt time.Time) error {
	if f.creds == nil {
		f.creds = map[string]models.NocPassCredential{}
	}
	key := NormalizeUsername(host) + "\x00" + NormalizeUsername(username)
	f.creds[key] = models.NocPassCredential{
		Host:              host,
		Username:          username,
		CanonicalUsername: NormalizeUsername(username),
		Source:            source,
		SavedUserID:       savedUserID,
		EncPassword:       append([]byte(nil), encPassword...),
		LastApplyOK:       true,
		LastAppliedAt:     &appliedAt,
	}
	return nil
}
func (f *fakeNocPassRepo) MarkCredentialFailure(host, username, source string, savedUserID *uint, errMsg string) error {
	if f.creds == nil {
		return nil
	}
	key := NormalizeUsername(host) + "\x00" + NormalizeUsername(username)
	item, ok := f.creds[key]
	if !ok {
		return nil
	}
	item.Source = source
	item.SavedUserID = savedUserID
	item.LastApplyOK = false
	item.LastApplyError = errMsg
	f.creds[key] = item
	return nil
}
func (f *fakeNocPassRepo) DeleteCredential(host, username string) error {
	if f.creds != nil {
		delete(f.creds, NormalizeUsername(host)+"\x00"+NormalizeUsername(username))
	}
	return nil
}
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

func TestSaveConfirmedCredentialRecordsBothRotatorUsers(t *testing.T) {
	repo := &fakeNocPassRepo{}
	key := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)

	if err := saveConfirmedCredential(repo, key, "10.0.0.1", UserFiberx, "rotator", nil, "FiberxPass123", now); err != nil {
		t.Fatalf("save fiberx credential: %v", err)
	}
	if err := saveConfirmedCredential(repo, key, "10.0.0.1", UserSupport, "rotator", nil, "SupportPass123", now); err != nil {
		t.Fatalf("save support credential: %v", err)
	}

	fiberx := repo.creds["10.0.0.1\x00fiberx"]
	support := repo.creds["10.0.0.1\x00support"]
	fiberxPlain, err := crypto.Decrypt(key, fiberx.EncPassword)
	if err != nil {
		t.Fatalf("decrypt fiberx credential: %v", err)
	}
	supportPlain, err := crypto.Decrypt(key, support.EncPassword)
	if err != nil {
		t.Fatalf("decrypt support credential: %v", err)
	}
	if fiberxPlain != "FiberxPass123" || supportPlain != "SupportPass123" {
		t.Fatalf("unexpected stored credentials: fiberx=%q support=%q", fiberxPlain, supportPlain)
	}
}

func TestCredentialFailureDoesNotOverwriteConfirmedPassword(t *testing.T) {
	repo := &fakeNocPassRepo{}
	key := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)

	if err := saveConfirmedCredential(repo, key, "10.0.0.1", UserFiberx, "rotator", nil, "LastGoodPass123", now); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := repo.MarkCredentialFailure("10.0.0.1", UserFiberx, "rotator", nil, "new password failed"); err != nil {
		t.Fatalf("mark credential failure: %v", err)
	}

	item := repo.creds["10.0.0.1\x00fiberx"]
	plain, err := crypto.Decrypt(key, item.EncPassword)
	if err != nil {
		t.Fatalf("decrypt credential: %v", err)
	}
	if plain != "LastGoodPass123" {
		t.Fatalf("failure overwrote confirmed password: %q", plain)
	}
	if item.LastApplyOK || item.LastApplyError != "new password failed" {
		t.Fatalf("expected failure status with preserved password, got ok=%v err=%q", item.LastApplyOK, item.LastApplyError)
	}
}

func TestSaveConfirmedCredentialRecordsSavedUser(t *testing.T) {
	repo := &fakeNocPassRepo{}
	key := []byte("01234567890123456789012345678901")
	now := time.Date(2026, 4, 21, 10, 0, 0, 0, time.Local)
	savedUserID := uint(42)

	if err := saveConfirmedCredential(repo, key, "10.0.0.5", "noc-ops", "saved_user", &savedUserID, "SavedUserPass123", now); err != nil {
		t.Fatalf("save saved-user credential: %v", err)
	}

	item := repo.creds["10.0.0.5\x00noc-ops"]
	if item.Source != "saved_user" || item.SavedUserID == nil || *item.SavedUserID != savedUserID {
		t.Fatalf("unexpected saved-user metadata: source=%q id=%v", item.Source, item.SavedUserID)
	}
	plain, err := crypto.Decrypt(key, item.EncPassword)
	if err != nil {
		t.Fatalf("decrypt saved-user credential: %v", err)
	}
	if plain != "SavedUserPass123" {
		t.Fatalf("unexpected saved-user password: %q", plain)
	}
}

func TestFilterPolicyOwnedTargetsAllNetworkSkipsSpecificPolicies(t *testing.T) {
	rows := []models.NocDataDevice{
		{Host: "10.0.0.1", Site: "Basra FTTH", Vendor: "mikrotik", DeviceModel: "CCR", Hostname: "basra-1"},
		{Host: "10.0.0.2", Site: "Basra WiFi", Vendor: "cisco_ios", DeviceModel: "C9300", Hostname: "basra-2"},
		{Host: "10.0.0.3", Site: "Najaf FTTH", Vendor: "huawei", DeviceModel: "NE40E", Hostname: "najaf-1"},
	}
	all := models.NocPassPolicy{Enabled: true, TargetType: TargetAllNetworks, TargetValue: "all"}
	basra := models.NocPassPolicy{Enabled: true, TargetType: TargetProvince, TargetValue: "Basra"}
	najafDevice := models.NocPassPolicy{Enabled: true, TargetType: TargetDevice, TargetValue: "10.0.0.3"}
	all.ID = 1
	basra.ID = 2
	najafDevice.ID = 3

	targets := ResolvePolicyTargets(rows, all.TargetType, all.TargetValue)
	got := FilterPolicyOwnedTargets(targets, &all, []models.NocPassPolicy{all, basra, najafDevice})
	if len(got) != 0 {
		t.Fatalf("all-network policy should skip devices owned by province/device policies, got %#v", got)
	}
}

func TestFilterPolicyOwnedTargetsProvinceSkipsDevicePolicy(t *testing.T) {
	rows := []models.NocDataDevice{
		{Host: "10.0.0.1", Site: "Basra FTTH", Vendor: "mikrotik", DeviceModel: "CCR", Hostname: "basra-1"},
		{Host: "10.0.0.2", Site: "Basra WiFi", Vendor: "cisco_ios", DeviceModel: "C9300", Hostname: "basra-2"},
		{Host: "10.0.0.3", Site: "Najaf FTTH", Vendor: "huawei", DeviceModel: "NE40E", Hostname: "najaf-1"},
	}
	basra := models.NocPassPolicy{Enabled: true, TargetType: TargetProvince, TargetValue: "Basra"}
	device := models.NocPassPolicy{Enabled: true, TargetType: TargetDevice, TargetValue: "10.0.0.2"}
	basra.ID = 2
	device.ID = 3

	targets := ResolvePolicyTargets(rows, basra.TargetType, basra.TargetValue)
	got := FilterPolicyOwnedTargets(targets, &basra, []models.NocPassPolicy{basra, device})
	if len(got) != 1 || got[0].Host != "10.0.0.1" {
		t.Fatalf("province policy should keep only Basra devices not owned by device policies, got %#v", got)
	}
}

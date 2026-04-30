package nocpass

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
	"gorm.io/gorm"
)

const (
	RotateInterval = 24 * time.Hour
	DeviceApplyGap = 500 * time.Millisecond
)

type RunSummary struct {
	PolicyID      uint
	PolicyName    string
	TargetLabel   string
	DeviceCount   int
	SuccessCount  int
	FailureCount  int
	Failures      []string
	ActiveDate    string
	ActiveMode    string
	ActiveChanged bool
}

type policyPasswords struct {
	Fiberx  string
	Support string
}

func todayString(now time.Time) string {
	return now.Format("2006-01-02")
}

func ShouldRunPolicy(policy *models.NocPassPolicy, now time.Time) bool {
	if policy == nil || !policy.Enabled {
		return false
	}
	today := todayString(now)
	switch strings.ToLower(strings.TrimSpace(policy.PasswordMode)) {
	case "manual":
		if policy.LastRunAt == nil {
			return true
		}
		return policy.LastRunAt.Format("2006-01-02") != today
	default:
		return strings.TrimSpace(policy.ActivePasswordDate) != today
	}
}

func ResolvePolicyTargets(rows []models.NocDataDevice, targetType, targetValue string) []models.NocDataDevice {
	normalizedType := strings.ToLower(strings.TrimSpace(targetType))
	normalizedValue := strings.TrimSpace(targetValue)
	if normalizedType == "" {
		normalizedType = TargetAllNetworks
		normalizedValue = "all"
	}

	seenHosts := make(map[string]struct{}, len(rows))
	out := make([]models.NocDataDevice, 0, len(rows))
	for i := range rows {
		row := rows[i]
		host := strings.ToLower(strings.TrimSpace(row.Host))
		if host == "" {
			continue
		}
		if _, ok := seenHosts[host]; ok {
			continue
		}
		if normalizedType == TargetDevice {
			if normalizedValue != "" && host != strings.ToLower(strings.TrimSpace(normalizedValue)) {
				continue
			}
		} else if !TargetMatches(normalizedType, normalizedValue, &row) {
			continue
		}
		seenHosts[host] = struct{}{}
		out = append(out, row)
	}
	return out
}

func policyTargetPriority(targetType string) int {
	switch strings.ToLower(strings.TrimSpace(targetType)) {
	case TargetDevice:
		return 50
	case TargetProvince:
		return 40
	case TargetModel:
		return 30
	case TargetVendor:
		return 20
	case TargetNetworkType:
		return 10
	case TargetAllNetworks:
		return 0
	default:
		return 0
	}
}

func policyMatchesRow(policy *models.NocPassPolicy, row *models.NocDataDevice) bool {
	if policy == nil || row == nil || !policy.Enabled {
		return false
	}
	targetType := strings.ToLower(strings.TrimSpace(policy.TargetType))
	targetValue := strings.TrimSpace(policy.TargetValue)
	if targetType == "" {
		targetType = TargetAllNetworks
		targetValue = "all"
	}
	host := strings.ToLower(strings.TrimSpace(row.Host))
	if host == "" {
		return false
	}
	if targetType == TargetDevice {
		return host == strings.ToLower(strings.TrimSpace(targetValue))
	}
	return TargetMatches(targetType, targetValue, row)
}

func policyOwnsRow(policy *models.NocPassPolicy, policies []models.NocPassPolicy, row *models.NocDataDevice) bool {
	if !policyMatchesRow(policy, row) {
		return false
	}
	bestID := policy.ID
	bestPriority := policyTargetPriority(policy.TargetType)
	for i := range policies {
		candidate := &policies[i]
		if candidate.ID == policy.ID || !policyMatchesRow(candidate, row) {
			continue
		}
		priority := policyTargetPriority(candidate.TargetType)
		if priority > bestPriority || (priority == bestPriority && (bestID == 0 || candidate.ID < bestID)) {
			bestID = candidate.ID
			bestPriority = priority
		}
	}
	return bestID == policy.ID
}

func FilterPolicyOwnedTargets(rows []models.NocDataDevice, policy *models.NocPassPolicy, policies []models.NocPassPolicy) []models.NocDataDevice {
	if policy == nil || len(rows) == 0 {
		return nil
	}
	if len(policies) == 0 {
		return rows
	}
	out := make([]models.NocDataDevice, 0, len(rows))
	for i := range rows {
		if policyOwnsRow(policy, policies, &rows[i]) {
			out = append(out, rows[i])
		}
	}
	return out
}

func decryptPolicyPassword(masterKey []byte, primary, legacy []byte) (string, error) {
	if len(primary) > 0 {
		return crypto.Decrypt(masterKey, primary)
	}
	if len(legacy) > 0 {
		return crypto.Decrypt(masterKey, legacy)
	}
	return "", fmt.Errorf("password is not set")
}

func EnsurePolicyPasswords(repo repository.NocPassRepository, masterKey []byte, policy *models.NocPassPolicy, now time.Time) (*policyPasswords, bool, error) {
	if policy == nil {
		return nil, false, errors.New("nil policy")
	}
	mode := strings.ToLower(strings.TrimSpace(policy.PasswordMode))
	today := todayString(now)

	switch mode {
	case "manual":
		fiberxPlain, err := decryptPolicyPassword(masterKey, policy.EncManualFiberxPassword, policy.EncManualPassword)
		if err != nil {
			return nil, false, fmt.Errorf("decrypt fiberx manual password: %w", err)
		}
		supportPlain, err := decryptPolicyPassword(masterKey, policy.EncManualSupportPassword, policy.EncManualPassword)
		if err != nil {
			return nil, false, fmt.Errorf("decrypt support manual password: %w", err)
		}
		if len(policy.EncActiveFiberxPassword) == 0 || len(policy.EncActiveSupportPassword) == 0 || strings.TrimSpace(policy.ActivePasswordDate) != today {
			policy.EncActiveFiberxPassword = append([]byte(nil), policy.EncManualFiberxPassword...)
			policy.EncActiveSupportPassword = append([]byte(nil), policy.EncManualSupportPassword...)
			if len(policy.EncActiveFiberxPassword) == 0 {
				policy.EncActiveFiberxPassword = append([]byte(nil), policy.EncManualPassword...)
			}
			if len(policy.EncActiveSupportPassword) == 0 {
				policy.EncActiveSupportPassword = append([]byte(nil), policy.EncManualPassword...)
			}
			policy.EncActivePassword = append([]byte(nil), policy.EncActiveFiberxPassword...)
			policy.ActivePasswordDate = today
			if err := repo.SavePolicy(policy); err != nil {
				return nil, false, err
			}
			return &policyPasswords{Fiberx: fiberxPlain, Support: supportPlain}, true, nil
		}
		return &policyPasswords{Fiberx: fiberxPlain, Support: supportPlain}, false, nil
	default:
		if len(policy.EncActiveFiberxPassword) > 0 && len(policy.EncActiveSupportPassword) > 0 && strings.TrimSpace(policy.ActivePasswordDate) == today {
			fiberxPlain, err := crypto.Decrypt(masterKey, policy.EncActiveFiberxPassword)
			if err != nil {
				return nil, false, fmt.Errorf("decrypt fiberx active password: %w", err)
			}
			supportPlain, err := crypto.Decrypt(masterKey, policy.EncActiveSupportPassword)
			if err != nil {
				return nil, false, fmt.Errorf("decrypt support active password: %w", err)
			}
			return &policyPasswords{Fiberx: fiberxPlain, Support: supportPlain}, false, nil
		}
		fiberxPass, err := RandomPassword(15)
		if err != nil {
			return nil, false, fmt.Errorf("generate fiberx password: %w", err)
		}
		supportPass, err := RandomPassword(15)
		if err != nil {
			return nil, false, fmt.Errorf("generate support password: %w", err)
		}
		fiberxEnc, err := crypto.Encrypt(masterKey, fiberxPass)
		if err != nil {
			return nil, false, fmt.Errorf("encrypt fiberx active password: %w", err)
		}
		supportEnc, err := crypto.Encrypt(masterKey, supportPass)
		if err != nil {
			return nil, false, fmt.Errorf("encrypt support active password: %w", err)
		}
		policy.EncActiveFiberxPassword = fiberxEnc
		policy.EncActiveSupportPassword = supportEnc
		policy.EncActivePassword = append([]byte(nil), fiberxEnc...)
		policy.ActivePasswordDate = today
		if err := repo.SavePolicy(policy); err != nil {
			return nil, false, err
		}
		return &policyPasswords{Fiberx: fiberxPass, Support: supportPass}, true, nil
	}
}

func RunPolicy(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte, policyID uint, now time.Time) (*RunSummary, error) {
	policy, err := repo.GetPolicy(policyID)
	if err != nil {
		return nil, err
	}
	if !policy.Enabled {
		return nil, fmt.Errorf("policy disabled")
	}

	rows, err := nocDataRepo.ListAll()
	if err != nil {
		return nil, err
	}

	targetType := strings.ToLower(strings.TrimSpace(policy.TargetType))
	targetValue := strings.TrimSpace(policy.TargetValue)
	targetLabel := strings.TrimSpace(policy.TargetLabel)
	if targetType == "" {
		targetType = TargetAllNetworks
		targetValue = "all"
	}
	if targetLabel == "" {
		targetLabel = DefaultTargetLabel(targetType, targetValue, nil)
	}

	targetRows := ResolvePolicyTargets(rows, targetType, targetValue)
	matchedCount := len(targetRows)
	policies, err := repo.ListPolicies()
	if err != nil {
		policy.LastRunAt = &now
		policy.LastStatus = "error"
		policy.LastMessage = "list policies: " + err.Error()
		_ = repo.SavePolicy(policy)
		return nil, fmt.Errorf("list policies: %w", err)
	}
	targetRows = FilterPolicyOwnedTargets(targetRows, policy, policies)
	if matchedCount > 0 && len(targetRows) == 0 {
		policy.LastRunAt = &now
		policy.LastStatus = "ok"
		policy.LastMessage = fmt.Sprintf("no devices owned by this policy on target %s; more specific policies own the matching devices", targetLabel)
		_ = repo.SavePolicy(policy)
		return &RunSummary{PolicyID: policy.ID, PolicyName: strings.TrimSpace(policy.Name), TargetLabel: targetLabel, ActiveMode: policy.PasswordMode}, nil
	}
	exclusions, err := repo.ListExclusions()
	if err != nil {
		policy.LastRunAt = &now
		policy.LastStatus = "error"
		policy.LastMessage = "list exclusions: " + err.Error()
		_ = repo.SavePolicy(policy)
		return nil, fmt.Errorf("list exclusions: %w", err)
	}
	filteredRows := make([]models.NocDataDevice, 0, len(targetRows))
	for i := range targetRows {
		excluded, excludeErr := isNocPassHostExcluded(targetRows[i].Host, exclusions)
		if excludeErr != nil {
			policy.LastRunAt = &now
			policy.LastStatus = "error"
			policy.LastMessage = "match exclusions: " + excludeErr.Error()
			_ = repo.SavePolicy(policy)
			return nil, fmt.Errorf("match exclusions: %w", excludeErr)
		}
		if excluded {
			continue
		}
		filteredRows = append(filteredRows, targetRows[i])
	}
	targetRows = filteredRows
	if len(targetRows) == 0 {
		policy.LastRunAt = &now
		policy.LastStatus = "error"
		policy.LastMessage = fmt.Sprintf("no matching devices found for target %q after exclusions", targetLabel)
		_ = repo.SavePolicy(policy)
		return &RunSummary{PolicyID: policy.ID, PolicyName: strings.TrimSpace(policy.Name), TargetLabel: targetLabel, ActiveMode: policy.PasswordMode}, fmt.Errorf("no matching devices found for target %q after exclusions", targetLabel)
	}

	passwords, activeChanged, err := EnsurePolicyPasswords(repo, masterKey, policy, now)
	if err != nil {
		policy.LastRunAt = &now
		policy.LastStatus = "error"
		policy.LastMessage = err.Error()
		_ = repo.SavePolicy(policy)
		return nil, err
	}

	summary := &RunSummary{
		PolicyID:      policy.ID,
		PolicyName:    strings.TrimSpace(policy.Name),
		TargetLabel:   targetLabel,
		DeviceCount:   len(targetRows),
		ActiveDate:    policy.ActivePasswordDate,
		ActiveMode:    policy.PasswordMode,
		ActiveChanged: activeChanged,
	}

	for i := range targetRows {
		row := targetRows[i]
		if err := ApplyToNocDataDevice(repo, nocDataRepo, masterKey, &row, passwords); err != nil {
			summary.FailureCount++
			summary.Failures = append(summary.Failures, fmt.Sprintf("%s (%s): %v", DeviceLabel(&row), row.Host, err))
		} else {
			summary.SuccessCount++
		}
		if i < len(targetRows)-1 {
			time.Sleep(DeviceApplyGap)
		}
	}

	policy.LastRunAt = &now
	switch {
	case summary.FailureCount == 0:
		policy.LastStatus = "ok"
		policy.LastMessage = fmt.Sprintf("completed for %d device(s) on target %s", summary.SuccessCount, targetLabel)
	case summary.SuccessCount == 0:
		policy.LastStatus = "error"
		policy.LastMessage = strings.Join(summary.Failures, "; ")
	default:
		policy.LastStatus = "error"
		policy.LastMessage = fmt.Sprintf("completed for %d device(s), %d failed on target %s: %s", summary.SuccessCount, summary.FailureCount, targetLabel, strings.Join(summary.Failures, "; "))
	}
	if err := repo.SavePolicy(policy); err != nil {
		return summary, err
	}
	return summary, nil
}

func isNocPassHostExcluded(host string, exclusions []models.NocPassExclusion) (bool, error) {
	for _, item := range exclusions {
		match, err := HostMatchesIPv4Spec(item.Subnet, item.Target, host)
		if err != nil {
			return false, err
		}
		if match {
			return true, nil
		}
	}
	return false, nil
}

func DeviceLabel(row *models.NocDataDevice) string {
	if row == nil {
		return ""
	}
	if strings.TrimSpace(row.Hostname) != "" {
		return strings.TrimSpace(row.Hostname)
	}
	if strings.TrimSpace(row.DisplayName) != "" {
		return strings.TrimSpace(row.DisplayName)
	}
	return strings.TrimSpace(row.Host)
}

type applyCredential struct {
	Username string
	Password string
}

func ApplyToNocDataDevice(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte, row *models.NocDataDevice, passwords *policyPasswords) error {
	if row == nil {
		return errors.New("nil device")
	}
	if passwords == nil || strings.TrimSpace(passwords.Fiberx) == "" || strings.TrimSpace(passwords.Support) == "" {
		return errors.New("missing fiberx/support passwords")
	}
	host := strings.TrimSpace(row.Host)
	if host == "" {
		return errors.New("empty host")
	}
	creds, err := resolveApplyCredentials(nocDataRepo, masterKey, row)
	if err != nil {
		_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, NormalizeSourceVendor(row.Vendor, row.DeviceModel), nil, nil, false, err.Error())
		return err
	}

	deviceState, err := repo.TouchHostState(DeviceLabel(row), host, NormalizeSourceVendor(row.Vendor, row.DeviceModel))
	if err != nil {
		return err
	}

	initialMikrotik := len(deviceState.EncNocPassword) == 0
	vendor, err := ShellVendor(deviceState)
	if err != nil {
		_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, err.Error())
		return err
	}

	var lastErr error
	for _, cred := range creds {
		adminUser := strings.TrimSpace(cred.Username)
		adminPass := strings.TrimSpace(cred.Password)
		if adminUser == "" || adminPass == "" {
			continue
		}

		keepUsers, managedUsers, err := listProtectedKeepUsers(repo, row, adminUser)
		if err != nil {
			_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, "list keep users: "+err.Error())
			return fmt.Errorf("list keep users: %w", err)
		}

		existingUsers, err := discoverExistingUsers(deviceState, vendor, adminUser, adminPass)
		if err != nil {
			lastErr = err
			continue
		}

		cmds, err := BuildCommandList(deviceState, passwords.Fiberx, passwords.Support, initialMikrotik, existingUsers, keepUsers)
		if err != nil {
			_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, err.Error())
			return err
		}

		log.Printf("[noc-pass] applying policy host=%s vendor=%s accounts=%s+%s keep=%d auth_user=%q transport=auto", host, vendor, UserFiberx, UserSupport, len(keepUsers), adminUser)
		out, method, runErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), host, adminUser, adminPass, vendor, "", cmds...)
		if runErr != nil {
			msg := runErr.Error()
			if len(out) > 0 {
				msg = msg + " — " + strings.TrimSpace(out[:min(200, len(out))])
			}
			lastErr = fmt.Errorf("apply with %s: %s", adminUser, msg)
			continue
		}
		log.Printf("[noc-pass] policy apply OK host=%s method=%s", host, method)

		enc, err := crypto.Encrypt(masterKey, passwords.Fiberx)
		if err != nil {
			_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, "encrypt new password: "+err.Error())
			return err
		}

		appliedAt := time.Now()
		if err := saveConfirmedCredential(repo, masterKey, host, UserFiberx, "rotator", nil, passwords.Fiberx, appliedAt); err != nil {
			_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, "save fiberx credential: "+err.Error())
			return err
		}
		if err := saveConfirmedCredential(repo, masterKey, host, UserSupport, "rotator", nil, passwords.Support, appliedAt); err != nil {
			_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, "save support credential: "+err.Error())
			return err
		}
		if err := repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, enc, &appliedAt, true, ""); err != nil {
			return err
		}
		for i := range managedUsers {
			if err := ApplySavedUserToDevice(repo, nocDataRepo, masterKey, row, &managedUsers[i]); err != nil {
				return fmt.Errorf("apply saved user %s: %w", managedUsers[i].Username, err)
			}
		}
		return nil
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no usable NOC Setup credential succeeded")
	}
	_ = repo.UpdateAfterApplyByHost(DeviceLabel(row), host, deviceState.Vendor, nil, nil, false, lastErr.Error())
	_ = repo.MarkCredentialFailure(host, UserFiberx, "rotator", nil, lastErr.Error())
	_ = repo.MarkCredentialFailure(host, UserSupport, "rotator", nil, lastErr.Error())
	return lastErr
}

func saveConfirmedCredential(repo repository.NocPassRepository, masterKey []byte, host, username, source string, savedUserID *uint, password string, appliedAt time.Time) error {
	enc, err := crypto.Encrypt(masterKey, password)
	if err != nil {
		return err
	}
	return repo.UpsertCredential(host, username, source, savedUserID, enc, appliedAt)
}

func resolveApplyCredentials(nocDataRepo repository.NocDataRepository, masterKey []byte, row *models.NocDataDevice) ([]applyCredential, error) {
	family := credentialVendorFamilyForApply(NormalizeSourceVendor(row.Vendor, row.DeviceModel))
	if family == "" {
		return nil, fmt.Errorf("unsupported NOC Data vendor %q", row.Vendor)
	}

	var out []applyCredential
	seen := map[string]struct{}{}
	add := func(user, pass string) {
		user = strings.TrimSpace(user)
		pass = strings.TrimSpace(pass)
		if user == "" || pass == "" {
			return
		}
		key := strings.ToLower(user) + "\x00" + pass
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, applyCredential{Username: user, Password: pass})
	}

	if len(row.EncUsername) > 0 && len(row.EncPassword) > 0 {
		user, userErr := crypto.Decrypt(masterKey, row.EncUsername)
		pass, passErr := crypto.Decrypt(masterKey, row.EncPassword)
		if userErr == nil && passErr == nil {
			add(user, pass)
		}
	}

	items, err := nocDataRepo.ListCredentials(family)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		user, userErr := crypto.Decrypt(masterKey, item.EncUsername)
		if userErr != nil {
			return nil, fmt.Errorf("decrypt NOC Setup username: %w", userErr)
		}
		pass, passErr := crypto.Decrypt(masterKey, item.EncPassword)
		if passErr != nil {
			return nil, fmt.Errorf("decrypt NOC Setup password: %w", passErr)
		}
		add(user, pass)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("missing %s credentials in NOC Setup users", family)
	}
	return out, nil
}

func credentialVendorFamilyForApply(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios", "cisco_nexus":
		return "cisco"
	case "huawei":
		return "huawei"
	case "mikrotik":
		return "mikrotik"
	default:
		return ""
	}
}

func listProtectedKeepUsers(repo repository.NocPassRepository, row *models.NocDataDevice, adminUser string) ([]string, []models.NocPassSavedUser, error) {
	list, err := repo.ListKeepUsers()
	if err != nil {
		return nil, nil, err
	}
	savedUsers, err := ManagedSavedUsersForRow(repo, row)
	if err != nil {
		return nil, nil, err
	}
	names := make([]string, 0, len(list)+1)
	seen := map[string]struct{}{}
	add := func(name string) {
		raw := strings.TrimSpace(name)
		if raw == "" {
			return
		}
		key := NormalizeUsername(raw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		names = append(names, raw)
	}
	add(adminUser)
	for _, item := range list {
		if strings.TrimSpace(item.Username) != "" {
			add(item.Username)
			continue
		}
		add(item.CanonicalUsername)
	}
	for _, item := range savedUsers {
		add(item.Username)
	}
	return names, savedUsers, nil
}

func discoverExistingUsers(d *models.NocPassDevice, vendor, adminUser, adminPass string) ([]string, error) {
	discoveryCmds, err := UserDiscoveryCommands(d)
	if err != nil {
		if strings.EqualFold(strings.TrimSpace(d.Vendor), "mikrotik") {
			return nil, nil
		}
		return nil, err
	}
	out, _, runErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), d.Host, adminUser, adminPass, vendor, "", discoveryCmds...)
	if runErr != nil {
		return nil, fmt.Errorf("discover users: %w", runErr)
	}
	return ExtractUsernamesForVendor(d.Vendor, out), nil
}

// RotateAndApply remains for compatibility with legacy single-device endpoints.
func RotateAndApply(repo repository.NocPassRepository, masterKey []byte, deviceID uint) error {
	d, err := repo.GetByID(deviceID)
	if err != nil {
		return err
	}
	if !d.Enabled {
		return fmt.Errorf("device disabled")
	}
	return errors.New("legacy single-device rotation is deprecated")
}

// ShouldRotate is true only after a successful rotation was recorded and 24h have passed.
func ShouldRotate(d *models.NocPassDevice) bool {
	if d == nil || d.PasswordRotatedAt == nil {
		return false
	}
	return time.Since(*d.PasswordRotatedAt) >= RotateInterval
}

func LoadDeviceStateByHost(repo repository.NocPassRepository, host string) (*models.NocPassDevice, error) {
	state, err := repo.GetByHost(host)
	if err == nil {
		return state, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return nil, err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

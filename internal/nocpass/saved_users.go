package nocpass

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

type SavedUserTargets struct {
	NetworkTypes []string `json:"network_types"`
	Provinces    []string `json:"provinces"`
	Vendors      []string `json:"vendors"`
	Models       []string `json:"models"`
	Devices      []string `json:"devices"`
}

func EncodeSavedUserList(values []string) string {
	normalized := NormalizeUsernames(values...)
	if len(normalized) == 0 {
		return "[]"
	}
	data, _ := json.Marshal(normalized)
	return string(data)
}

func DecodeSavedUserList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	return NormalizeUsernames(values...)
}

func savedUserTargetsFromModel(item *models.NocPassSavedUser) SavedUserTargets {
	if item == nil {
		return SavedUserTargets{}
	}
	return SavedUserTargets{
		NetworkTypes: DecodeSavedUserList(item.NetworkTypesJSON),
		Provinces:    DecodeSavedUserList(item.ProvincesJSON),
		Vendors:      DecodeSavedUserList(item.VendorsJSON),
		Models:       DecodeSavedUserList(item.ModelsJSON),
		Devices:      NormalizeDeviceTargets(DecodeSavedUserList(item.DevicesJSON)...),
	}
}

func NormalizeDeviceTargets(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func SavedUserMatchesRow(item *models.NocPassSavedUser, row *models.NocDataDevice) bool {
	if item == nil || row == nil {
		return false
	}
	targets := savedUserTargetsFromModel(item)
	host := strings.ToLower(strings.TrimSpace(row.Host))
	networkType := NormalizeUsername(NetworkTypeFromSite(row.Site))
	province := NormalizeUsername(ProvinceFromSite(row.Site))
	vendor := NormalizeUsername(NormalizeSourceVendor(row.Vendor, row.DeviceModel))
	model := NormalizeUsername(strings.TrimSpace(row.DeviceModel))

	if len(targets.NetworkTypes) == 0 && len(targets.Provinces) == 0 && len(targets.Vendors) == 0 && len(targets.Models) == 0 && len(targets.Devices) == 0 {
		return false
	}
	if len(targets.NetworkTypes) > 0 && !containsUsername(targets.NetworkTypes, networkType) {
		return false
	}
	if len(targets.Provinces) > 0 && !containsUsername(targets.Provinces, province) {
		return false
	}
	if len(targets.Vendors) > 0 && !containsUsername(targets.Vendors, vendor) {
		return false
	}
	if len(targets.Models) > 0 && !containsUsername(targets.Models, model) {
		return false
	}
	if len(targets.Devices) > 0 && !containsUsername(targets.Devices, host) {
		return false
	}
	return true
}

func ManagedSavedUsersForRow(repo repository.NocPassRepository, row *models.NocDataDevice) ([]models.NocPassSavedUser, error) {
	list, err := repo.ListSavedUsers()
	if err != nil {
		return nil, err
	}
	out := make([]models.NocPassSavedUser, 0, len(list))
	for i := range list {
		if SavedUserMatchesRow(&list[i], row) {
			out = append(out, list[i])
		}
	}
	return out, nil
}

func NormalizeSavedUserPrivilege(privilege string) string {
	switch NormalizeUsername(privilege) {
	case "write":
		return "write"
	case "read":
		return "read"
	default:
		return "full"
	}
}

func BuildSavedUserCreateCommands(vendor, username, password, privilege string) ([]string, error) {
	privilege = NormalizeSavedUserPrivilege(privilege)
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios":
		level := "15"
		if privilege == "write" {
			level = "5"
		}
		if privilege == "read" {
			level = "1"
		}
		return []string{
			"configure terminal",
			fmt.Sprintf("username %s privilege %s secret 0 %s", username, level, password),
			"end",
			"write memory",
		}, nil
	case "cisco_nexus":
		role := "network-admin"
		if privilege != "full" {
			role = "network-operator"
		}
		return []string{
			"configure terminal",
			fmt.Sprintf("username %s password %s role %s", username, password, role),
			"end",
			"copy running-config startup-config",
		}, nil
	case "huawei":
		level := "15"
		if privilege == "write" {
			level = "3"
		}
		if privilege == "read" {
			level = "1"
		}
		return []string{
			fmt.Sprintf("local-user %s password irreversible-cipher %s", username, password),
			fmt.Sprintf("local-user %s privilege level %s", username, level),
			fmt.Sprintf("local-user %s service-type ssh terminal", username),
			"save",
			"y",
		}, nil
	case "mikrotik":
		group := NormalizeSavedUserPrivilege(privilege)
		return []string{
			buildMikrotikEnsureUserCommand(username, group, password),
			fmt.Sprintf("/user set [find name=%s] password=%s", username, password),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported vendor %q", vendor)
	}
}

func BuildSavedUserDeleteCommands(vendor, username string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "cisco_ios", "cisco_nexus":
		saveCmd := "write memory"
		if strings.EqualFold(vendor, "cisco_nexus") {
			saveCmd = "copy running-config startup-config"
		}
		return []string{
			"configure terminal",
			fmt.Sprintf("no username %s", username),
			"end",
			saveCmd,
		}, nil
	case "huawei":
		return []string{
			fmt.Sprintf("undo local-user %s", username),
			"save",
			"y",
		}, nil
	case "mikrotik":
		return []string{fmt.Sprintf(`/user remove [find name="%s"]`, username)}, nil
	default:
		return nil, fmt.Errorf("unsupported vendor %q", vendor)
	}
}

func ApplySavedUserToDevice(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte, row *models.NocDataDevice, item *models.NocPassSavedUser) error {
	if row == nil || item == nil {
		return nil
	}
	password, err := crypto.Decrypt(masterKey, item.EncPassword)
	if err != nil {
		return fmt.Errorf("decrypt saved user password: %w", err)
	}
	deviceState, err := repo.TouchHostState(DeviceLabel(row), strings.TrimSpace(row.Host), NormalizeSourceVendor(row.Vendor, row.DeviceModel))
	if err != nil {
		return err
	}
	vendor, err := ShellVendor(deviceState)
	if err != nil {
		return err
	}
	creds, err := resolveApplyCredentials(nocDataRepo, masterKey, row)
	if err != nil {
		return err
	}
	cmds, err := BuildSavedUserCreateCommands(deviceState.Vendor, item.Username, password, item.Privilege)
	if err != nil {
		return err
	}
	var lastErr error
	for _, cred := range creds {
		_, _, runErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), strings.TrimSpace(row.Host), cred.Username, cred.Password, vendor, "", cmds...)
		if runErr == nil {
			appliedAt := time.Now()
			savedUserID := item.ID
			if err := saveConfirmedCredential(repo, masterKey, strings.TrimSpace(row.Host), item.Username, "saved_user", &savedUserID, password, appliedAt); err != nil {
				return fmt.Errorf("save saved-user credential: %w", err)
			}
			return nil
		}
		lastErr = runErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable credential for saved user apply")
	}
	savedUserID := item.ID
	_ = repo.MarkCredentialFailure(strings.TrimSpace(row.Host), item.Username, "saved_user", &savedUserID, lastErr.Error())
	return lastErr
}

func DeleteSavedUserFromDevice(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte, row *models.NocDataDevice, item *models.NocPassSavedUser) error {
	if row == nil || item == nil {
		return nil
	}
	deviceState, err := repo.TouchHostState(DeviceLabel(row), strings.TrimSpace(row.Host), NormalizeSourceVendor(row.Vendor, row.DeviceModel))
	if err != nil {
		return err
	}
	vendor, err := ShellVendor(deviceState)
	if err != nil {
		return err
	}
	creds, err := resolveApplyCredentials(nocDataRepo, masterKey, row)
	if err != nil {
		return err
	}
	cmds, err := BuildSavedUserDeleteCommands(deviceState.Vendor, item.Username)
	if err != nil {
		return err
	}
	var lastErr error
	for _, cred := range creds {
		_, _, runErr := shell.NocDataSendCommandUsingMethodContext(context.Background(), strings.TrimSpace(row.Host), cred.Username, cred.Password, vendor, "", cmds...)
		if runErr == nil {
			_ = repo.DeleteCredential(strings.TrimSpace(row.Host), item.Username)
			return nil
		}
		lastErr = runErr
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable credential for saved user delete")
	}
	return lastErr
}

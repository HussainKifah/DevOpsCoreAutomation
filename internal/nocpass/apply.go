package nocpass

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/crypto"
	"github.com/Flafl/DevOpsCore/internal/models"
	"github.com/Flafl/DevOpsCore/internal/repository"
	"github.com/Flafl/DevOpsCore/internal/shell"
)

const RotateInterval = 24 * time.Hour

// RotateAndApply generates a new password, pushes it to the device, and persists ciphertext on success.
func RotateAndApply(repo repository.NocPassRepository, masterKey []byte, deviceID uint) error {
	d, err := repo.GetByID(deviceID)
	if err != nil {
		return err
	}
	if !d.Enabled {
		return fmt.Errorf("device disabled")
	}

	adminUser, err := crypto.Decrypt(masterKey, d.EncAdminUser)
	if err != nil {
		return fmt.Errorf("admin user decrypt: %w", err)
	}
	adminPass, err := crypto.Decrypt(masterKey, d.EncAdminPass)
	if err != nil {
		return fmt.Errorf("admin password decrypt: %w", err)
	}
	adminUser = strings.TrimSpace(adminUser)
	adminPass = strings.TrimSpace(adminPass)
	if adminUser == "" || adminPass == "" {
		return fmt.Errorf("admin SSH username or password is empty after decrypt")
	}

	keepUsers, err := listProtectedKeepUsers(repo, adminUser)
	if err != nil {
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, "list keep users: "+err.Error())
		return fmt.Errorf("list keep users: %w", err)
	}

	newPass, err := RandomPassword(15)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}

	initialMikrotik := len(d.EncNocPassword) == 0
	vendor, err := ShellVendor(d)
	if err != nil {
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, err.Error())
		return err
	}

	existingUsers, err := discoverExistingUsers(d, vendor, adminUser, adminPass)
	if err != nil {
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, err.Error())
		return err
	}

	cmds, err := BuildCommandList(d, newPass, initialMikrotik, existingUsers, keepUsers)
	if err != nil {
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, err.Error())
		return err
	}

	log.Printf("[noc-pass] applying rotation host=%s vendor=%s accounts=%s+%s keep=%d", d.Host, vendor, UserFiberx, UserReadOnly, len(keepUsers))
	out, runErr := shell.NocPassSendCommand(d.Host, adminUser, adminPass, vendor, cmds...)
	if runErr != nil {
		msg := runErr.Error()
		if len(out) > 0 {
			msg = msg + " — " + strings.TrimSpace(out[:min(200, len(out))])
		}
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, msg)
		return fmt.Errorf("ssh apply: %w", runErr)
	}

	enc, err := crypto.Encrypt(masterKey, newPass)
	if err != nil {
		_ = repo.UpdateAfterApply(deviceID, nil, nil, false, "encrypt new password: "+err.Error())
		return err
	}

	now := time.Now()
	if err := repo.UpdateAfterApply(deviceID, enc, &now, true, ""); err != nil {
		return err
	}
	log.Printf("[noc-pass] rotation OK host=%s", d.Host)
	return nil
}

func listProtectedKeepUsers(repo repository.NocPassRepository, adminUser string) ([]string, error) {
	list, err := repo.ListKeepUsers()
	if err != nil {
		return nil, err
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
		add(item.Username)
	}
	return names, nil
}

func discoverExistingUsers(d *models.NocPassDevice, vendor, adminUser, adminPass string) ([]string, error) {
	discoveryCmd, err := CiscoUserDiscoveryCommand(d)
	if err != nil {
		if strings.EqualFold(strings.TrimSpace(d.Vendor), "mikrotik") {
			return nil, nil
		}
		return nil, err
	}
	out, runErr := shell.NocPassSendCommand(d.Host, adminUser, adminPass, vendor, discoveryCmd)
	if runErr != nil {
		return nil, fmt.Errorf("discover users: %w", runErr)
	}
	return ExtractCiscoUsernames(out), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ShouldRotate is true only after a successful rotation was recorded and 24h have passed.
// The first push runs when the device is created (handler) or via Rotate Now — not on this ticker.
func ShouldRotate(d *models.NocPassDevice) bool {
	if d.PasswordRotatedAt == nil {
		return false
	}
	return time.Since(*d.PasswordRotatedAt) >= RotateInterval
}

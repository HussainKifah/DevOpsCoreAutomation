package repository

import (
	"errors"
	"strings"
	"time"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NocPassRepository interface {
	ListPolicies() ([]models.NocPassPolicy, error)
	GetPolicy(id uint) (*models.NocPassPolicy, error)
	CreatePolicy(policy *models.NocPassPolicy) error
	SavePolicy(policy *models.NocPassPolicy) error
	DeletePolicy(id uint) error
	ListExclusions() ([]models.NocPassExclusion, error)
	CreateExclusion(e *models.NocPassExclusion) error
	DeleteExclusion(id uint) error
	Search(q string) ([]models.NocPassDevice, error)
	ListStatuses() ([]models.NocPassDevice, error)
	GetByID(id uint) (*models.NocPassDevice, error)
	GetByHost(host string) (*models.NocPassDevice, error)
	TouchHostState(displayName, host, vendor string) (*models.NocPassDevice, error)
	Delete(id uint) error
	ListKeepUsers() ([]models.NocPassKeepUser, error)
	CreateKeepUser(user *models.NocPassKeepUser) error
	DeleteKeepUser(id uint) error
	ListSavedUsers() ([]models.NocPassSavedUser, error)
	GetSavedUser(id uint) (*models.NocPassSavedUser, error)
	CreateSavedUser(user *models.NocPassSavedUser) error
	DeleteSavedUser(id uint) error
	ListCredentials() ([]models.NocPassCredential, error)
	UpsertCredential(host, username, source string, savedUserID *uint, encPassword []byte, appliedAt time.Time) error
	MarkCredentialFailure(host, username, source string, savedUserID *uint, errMsg string) error
	DeleteCredential(host, username string) error
	UpdateAfterApply(id uint, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error
	UpdateAfterApplyByHost(displayName, host, vendor string, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error
}

type nocPassRepo struct {
	db *gorm.DB
}

func NewNocPassRepository(db *gorm.DB) NocPassRepository {
	return &nocPassRepo{db: db}
}

func defaultPolicy() *models.NocPassPolicy {
	return &models.NocPassPolicy{
		Name:         "NOC PASS Policy",
		Enabled:      false,
		TargetType:   "all_networks",
		TargetValue:  "all",
		TargetLabel:  "All Networks",
		PasswordMode: "random",
		LastStatus:   "pending",
	}
}

func normalizePolicy(policy *models.NocPassPolicy) {
	if policy == nil {
		return
	}
	if strings.TrimSpace(policy.Name) == "" {
		policy.Name = "NOC PASS Policy"
	}
	if strings.TrimSpace(policy.TargetType) == "" {
		policy.TargetType = "all_networks"
	}
	if strings.TrimSpace(policy.TargetValue) == "" {
		policy.TargetValue = "all"
	}
	if strings.TrimSpace(policy.TargetLabel) == "" {
		policy.TargetLabel = "All Networks"
	}
	if strings.TrimSpace(policy.PasswordMode) == "" {
		policy.PasswordMode = "random"
	}
	if strings.TrimSpace(policy.LastStatus) == "" {
		policy.LastStatus = "pending"
	}
}

func (r *nocPassRepo) ensureSeedPolicy() error {
	var count int64
	if err := r.db.Model(&models.NocPassPolicy{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.db.Create(defaultPolicy()).Error
}

func (r *nocPassRepo) ListPolicies() ([]models.NocPassPolicy, error) {
	if err := r.ensureSeedPolicy(); err != nil {
		return nil, err
	}
	var policies []models.NocPassPolicy
	if err := r.db.Order("created_at ASC, id ASC").Find(&policies).Error; err != nil {
		return nil, err
	}
	for i := range policies {
		normalizePolicy(&policies[i])
	}
	return policies, nil
}

func (r *nocPassRepo) GetPolicy(id uint) (*models.NocPassPolicy, error) {
	if id == 0 {
		policies, err := r.ListPolicies()
		if err != nil {
			return nil, err
		}
		if len(policies) == 0 {
			return nil, gorm.ErrRecordNotFound
		}
		return &policies[0], nil
	}
	var policy models.NocPassPolicy
	if err := r.db.First(&policy, id).Error; err != nil {
		return nil, err
	}
	normalizePolicy(&policy)
	return &policy, nil
}

func (r *nocPassRepo) CreatePolicy(policy *models.NocPassPolicy) error {
	if policy == nil {
		return errors.New("nil policy")
	}
	normalizePolicy(policy)
	return r.db.Create(policy).Error
}

func (r *nocPassRepo) SavePolicy(policy *models.NocPassPolicy) error {
	if policy == nil {
		return errors.New("nil policy")
	}
	normalizePolicy(policy)
	if policy.ID == 0 {
		return r.CreatePolicy(policy)
	}
	return r.db.Save(policy).Error
}

func (r *nocPassRepo) DeletePolicy(id uint) error {
	return r.db.Delete(&models.NocPassPolicy{}, id).Error
}

func (r *nocPassRepo) ListExclusions() ([]models.NocPassExclusion, error) {
	var list []models.NocPassExclusion
	err := r.db.Order("subnet ASC, target ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) CreateExclusion(e *models.NocPassExclusion) error {
	return r.db.Create(e).Error
}

func (r *nocPassRepo) DeleteExclusion(id uint) error {
	return r.db.Delete(&models.NocPassExclusion{}, id).Error
}

func (r *nocPassRepo) ListStatuses() ([]models.NocPassDevice, error) {
	var list []models.NocPassDevice
	err := r.db.Order("display_name ASC, host ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) Search(q string) ([]models.NocPassDevice, error) {
	var list []models.NocPassDevice
	qq := strings.TrimSpace(q)
	tx := r.db.Model(&models.NocPassDevice{})
	if qq != "" {
		pat := "%" + strings.ToLower(qq) + "%"
		tx = tx.Where("LOWER(display_name) LIKE ? OR LOWER(host) LIKE ?", pat, pat)
	}
	err := tx.Order("display_name ASC, host ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) GetByID(id uint) (*models.NocPassDevice, error) {
	var d models.NocPassDevice
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *nocPassRepo) GetByHost(host string) (*models.NocPassDevice, error) {
	var d models.NocPassDevice
	err := r.db.Where("LOWER(host) = ?", strings.ToLower(strings.TrimSpace(host))).Order("id ASC").First(&d).Error
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *nocPassRepo) TouchHostState(displayName, host, vendor string) (*models.NocPassDevice, error) {
	trimmedHost := strings.TrimSpace(host)
	if trimmedHost == "" {
		return nil, errors.New("host required")
	}
	existing, err := r.GetByHost(trimmedHost)
	if err == nil {
		updates := map[string]interface{}{
			"display_name": strings.TrimSpace(displayName),
			"vendor":       strings.TrimSpace(vendor),
			"enabled":      true,
		}
		if err := r.db.Model(&models.NocPassDevice{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return nil, err
		}
		existing.DisplayName = strings.TrimSpace(displayName)
		existing.Vendor = strings.TrimSpace(vendor)
		existing.Enabled = true
		return existing, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	item := &models.NocPassDevice{
		DisplayName: strings.TrimSpace(displayName),
		Host:        trimmedHost,
		Vendor:      strings.TrimSpace(vendor),
		Enabled:     true,
	}
	if err := r.db.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

func (r *nocPassRepo) Delete(id uint) error {
	return r.db.Delete(&models.NocPassDevice{}, id).Error
}

func (r *nocPassRepo) ListKeepUsers() ([]models.NocPassKeepUser, error) {
	var list []models.NocPassKeepUser
	err := r.db.Order("username ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) CreateKeepUser(user *models.NocPassKeepUser) error {
	if user == nil {
		return errors.New("nil keep user")
	}
	user.Username = strings.TrimSpace(user.Username)
	user.CanonicalUsername = strings.ToLower(strings.TrimSpace(user.CanonicalUsername))
	if user.CanonicalUsername == "" {
		user.CanonicalUsername = strings.ToLower(strings.TrimSpace(user.Username))
	}
	var existing models.NocPassKeepUser
	err := r.db.
		Where("LOWER(COALESCE(canonical_username, username)) = ?", user.CanonicalUsername).
		First(&existing).Error
	if err == nil {
		return errors.New("username already exists")
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return r.db.Create(user).Error
}

func (r *nocPassRepo) DeleteKeepUser(id uint) error {
	return r.db.Unscoped().Delete(&models.NocPassKeepUser{}, id).Error
}

func (r *nocPassRepo) ListSavedUsers() ([]models.NocPassSavedUser, error) {
	var list []models.NocPassSavedUser
	err := r.db.Order("username ASC, id ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) GetSavedUser(id uint) (*models.NocPassSavedUser, error) {
	var item models.NocPassSavedUser
	if err := r.db.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *nocPassRepo) CreateSavedUser(user *models.NocPassSavedUser) error {
	if user == nil {
		return errors.New("nil saved user")
	}
	user.Username = strings.TrimSpace(user.Username)
	user.CanonicalUsername = strings.ToLower(strings.TrimSpace(user.CanonicalUsername))
	if user.CanonicalUsername == "" {
		user.CanonicalUsername = strings.ToLower(strings.TrimSpace(user.Username))
	}
	return r.db.Create(user).Error
}

func (r *nocPassRepo) DeleteSavedUser(id uint) error {
	return r.db.Unscoped().Delete(&models.NocPassSavedUser{}, id).Error
}

func (r *nocPassRepo) ListCredentials() ([]models.NocPassCredential, error) {
	var list []models.NocPassCredential
	err := r.db.Order("host ASC, canonical_username ASC").Find(&list).Error
	return list, err
}

func (r *nocPassRepo) UpsertCredential(host, username, source string, savedUserID *uint, encPassword []byte, appliedAt time.Time) error {
	trimmedHost := strings.TrimSpace(host)
	displayUsername := strings.TrimSpace(username)
	canonical := strings.ToLower(strings.TrimSpace(username))
	if trimmedHost == "" {
		return errors.New("host required")
	}
	if canonical == "" {
		return errors.New("username required")
	}
	if len(encPassword) == 0 {
		return errors.New("password required")
	}
	item := models.NocPassCredential{
		Host:              trimmedHost,
		Username:          displayUsername,
		CanonicalUsername: canonical,
		Source:            strings.TrimSpace(source),
		EncPassword:       encPassword,
		SavedUserID:       savedUserID,
		LastApplyOK:       true,
		LastApplyError:    "",
		LastAppliedAt:     &appliedAt,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "host"}, {Name: "canonical_username"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"username",
			"source",
			"enc_password",
			"saved_user_id",
			"last_apply_ok",
			"last_apply_error",
			"last_applied_at",
			"updated_at",
			"deleted_at",
		}),
	}).Create(&item).Error
}

func (r *nocPassRepo) MarkCredentialFailure(host, username, source string, savedUserID *uint, errMsg string) error {
	trimmedHost := strings.TrimSpace(host)
	canonical := strings.ToLower(strings.TrimSpace(username))
	if trimmedHost == "" || canonical == "" {
		return nil
	}
	updates := map[string]interface{}{
		"username":         strings.TrimSpace(username),
		"source":           strings.TrimSpace(source),
		"saved_user_id":    savedUserID,
		"last_apply_ok":    false,
		"last_apply_error": errMsg,
		"updated_at":       time.Now(),
	}
	return r.db.Model(&models.NocPassCredential{}).
		Where("LOWER(host) = ? AND canonical_username = ?", strings.ToLower(trimmedHost), canonical).
		Updates(updates).Error
}

func (r *nocPassRepo) DeleteCredential(host, username string) error {
	trimmedHost := strings.TrimSpace(host)
	canonical := strings.ToLower(strings.TrimSpace(username))
	if trimmedHost == "" || canonical == "" {
		return nil
	}
	return r.db.Unscoped().
		Where("LOWER(host) = ? AND canonical_username = ?", strings.ToLower(trimmedHost), canonical).
		Delete(&models.NocPassCredential{}).Error
}

func (r *nocPassRepo) UpdateAfterApply(id uint, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"last_apply_ok":    ok,
		"last_apply_error": errMsg,
		"last_applied_at":  now,
	}
	if ok && len(encNocPass) > 0 && rotatedAt != nil {
		updates["enc_noc_password"] = encNocPass
		updates["password_rotated_at"] = rotatedAt
	}
	return r.db.Model(&models.NocPassDevice{}).Where("id = ?", id).Updates(updates).Error
}

func (r *nocPassRepo) UpdateAfterApplyByHost(displayName, host, vendor string, encNocPass []byte, rotatedAt *time.Time, ok bool, errMsg string) error {
	item, err := r.TouchHostState(displayName, host, vendor)
	if err != nil {
		return err
	}
	return r.UpdateAfterApply(item.ID, encNocPass, rotatedAt, ok, errMsg)
}

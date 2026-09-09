package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func configureAccountVerificationTest(t *testing.T, enabled, providers, delay string) {
	t.Helper()
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	previous := make(map[string]string)
	for _, key := range []string{setting.AccountVerificationEnabledOption, setting.AccountVerificationProvidersOption, setting.AccountVerificationFreezeDelayMinutesOption} {
		if value, ok := common.OptionMap[key]; ok {
			previous[key] = value
		}
	}
	common.OptionMap[setting.AccountVerificationEnabledOption] = enabled
	common.OptionMap[setting.AccountVerificationProvidersOption] = providers
	common.OptionMap[setting.AccountVerificationFreezeDelayMinutesOption] = delay
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		for _, key := range []string{setting.AccountVerificationEnabledOption, setting.AccountVerificationProvidersOption, setting.AccountVerificationFreezeDelayMinutesOption} {
			if value, ok := previous[key]; ok {
				common.OptionMap[key] = value
			} else {
				delete(common.OptionMap, key)
			}
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func TestInsertEnrollsNewUserButNotLegacyUser(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")

	legacy := &User{Username: "verification-legacy", Password: "password", Role: common.RoleCommonUser, AffCode: "verification-legacy"}
	require.NoError(t, DB.Create(legacy).Error)

	newUser := &User{Username: "verification-new", Password: "password", Role: common.RoleCommonUser, AffCode: "verification-new"}
	require.NoError(t, newUser.InsertWithTx(DB, 0))

	assert.Zero(t, legacy.VerificationRequiredAt)
	assert.NotZero(t, newUser.VerificationRequiredAt)
}

func TestEnforceAccountVerificationFreezesAndDeletesOnlyEnrolledUsers(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	now := time.Now().Unix()

	pending := &User{Username: "verification-pending", Password: "password", AffCode: "verification-pending", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, VerificationRequiredAt: now - 3601}
	legacy := &User{Username: "verification-legacy-untouched", Password: "password", AffCode: "verification-legacy-untouched", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(pending).Error)
	require.NoError(t, DB.Create(legacy).Error)
	require.NoError(t, DB.Create(&Token{UserId: pending.Id, Key: "verification-token"}).Error)

	summary, err := EnforceAccountVerification(now)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Frozen)

	var frozen User
	require.NoError(t, DB.First(&frozen, pending.Id).Error)
	assert.Equal(t, UserStatusVerificationFrozen, frozen.Status)
	assert.Equal(t, now, frozen.VerificationFrozenAt)

	oldFrozenAt := now - int64(setting.AccountVerificationDeletionDelayDays*24*60*60) - 1
	require.NoError(t, DB.Model(&User{}).Where("id = ?", pending.Id).Update("verification_frozen_at", oldFrozenAt).Error)
	summary, err = EnforceAccountVerification(now)
	require.NoError(t, err)
	assert.Equal(t, 1, summary.Deleted)
	assert.ErrorIs(t, DB.Unscoped().First(&User{}, pending.Id).Error, gorm.ErrRecordNotFound)
	assert.ErrorIs(t, DB.Unscoped().Where("user_id = ?", pending.Id).First(&Token{}).Error, gorm.ErrRecordNotFound)
	require.NoError(t, DB.First(&legacy, legacy.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, legacy.Status)
}

func TestVerificationProviderUnfreezesAccountWithoutEnablingAdminBan(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email,telegram", "60")

	frozen := &User{
		Username:                   "verification-frozen",
		Password:                   "password",
		AffCode:                    "verification-frozen",
		Status:                     UserStatusVerificationFrozen,
		VerificationRequiredAt:     100,
		VerificationReminderSentAt: 200,
		VerificationFrozenAt:       time.Now().Unix(),
	}
	require.NoError(t, DB.Create(frozen).Error)
	require.NoError(t, MarkUserEmailVerified(frozen.Id, time.Now().Unix()))
	require.NoError(t, DB.First(frozen, frozen.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, frozen.Status)
	assert.NotZero(t, frozen.EmailVerifiedAt)
	assert.Zero(t, frozen.VerificationRequiredAt)
	assert.Zero(t, frozen.VerificationReminderSentAt)
	assert.Zero(t, frozen.VerificationFrozenAt)

	banned := &User{Username: "verification-banned", Password: "password", AffCode: "verification-banned", Status: common.UserStatusDisabled}
	require.NoError(t, DB.Create(banned).Error)
	require.NoError(t, MarkUserTelegramVerified(banned.Id, time.Now().Unix()))
	require.NoError(t, DB.First(banned, banned.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, banned.Status)
}

func TestVerificationProviderOutsidePolicyDoesNotUnfreezeAccount(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")

	frozen := &User{Username: "verification-policy", Password: "password", AffCode: "verification-policy", Status: UserStatusVerificationFrozen, VerificationFrozenAt: time.Now().Unix()}
	require.NoError(t, DB.Create(frozen).Error)
	require.NoError(t, MarkUserTelegramVerified(frozen.Id, time.Now().Unix()))
	require.NoError(t, DB.First(frozen, frozen.Id).Error)
	assert.Equal(t, UserStatusVerificationFrozen, frozen.Status)
	assert.NotZero(t, frozen.TelegramVerifiedAt)
}

func TestAccountVerificationBlocksManualEnableWithoutProof(t *testing.T) {
	configureAccountVerificationTest(t, "true", "email", "60")

	tests := []struct {
		name  string
		user  User
		block bool
	}{
		{
			name: "frozen user",
			user: User{
				Role:                   common.RoleCommonUser,
				Status:                 UserStatusVerificationFrozen,
				VerificationRequiredAt: 100,
				VerificationFrozenAt:   200,
			},
			block: true,
		},
		{
			name: "manually disabled pending user",
			user: User{
				Role:                   common.RoleCommonUser,
				Status:                 common.UserStatusDisabled,
				VerificationRequiredAt: 100,
			},
			block: true,
		},
		{
			name: "verified frozen user",
			user: User{
				Role:                   common.RoleCommonUser,
				Status:                 UserStatusVerificationFrozen,
				VerificationRequiredAt: 100,
				VerificationFrozenAt:   200,
				EmailVerifiedAt:        300,
			},
			block: false,
		},
		{
			name: "legacy disabled user",
			user: User{
				Role:   common.RoleCommonUser,
				Status: common.UserStatusDisabled,
			},
			block: false,
		},
		{
			name: "frozen administrator",
			user: User{
				Role:                   common.RoleAdminUser,
				Status:                 UserStatusVerificationFrozen,
				VerificationRequiredAt: 100,
				VerificationFrozenAt:   200,
			},
			block: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.block, AccountVerificationBlocksManualEnable(&tt.user))
		})
	}
}

func TestEnableUserPersistsLifecycleResetAfterVerification(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")

	user := &User{
		Username:                   "verification-manual-enable",
		Password:                   "password",
		AffCode:                    "verification-manual-enable",
		Role:                       common.RoleCommonUser,
		Status:                     UserStatusVerificationFrozen,
		VerificationRequiredAt:     100,
		VerificationReminderSentAt: 200,
		VerificationFrozenAt:       300,
		EmailVerifiedAt:            400,
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, EnableUser(user.Id))
	var actual User
	require.NoError(t, DB.First(&actual, user.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, actual.Status)
	assert.Zero(t, actual.VerificationRequiredAt)
	assert.Zero(t, actual.VerificationReminderSentAt)
	assert.Zero(t, actual.VerificationFrozenAt)
}

func TestEnableUserRejectsUnverifiedLifecycleUser(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")

	user := &User{
		Username:               "verification-manual-enable-blocked",
		Password:               "password",
		AffCode:                "verification-manual-enable-blocked",
		Role:                   common.RoleCommonUser,
		Status:                 UserStatusVerificationFrozen,
		VerificationRequiredAt: 100,
		VerificationFrozenAt:   300,
	}
	require.NoError(t, DB.Create(user).Error)

	err := EnableUser(user.Id)
	require.ErrorIs(t, err, ErrAccountVerificationRequired)
	var actual User
	require.NoError(t, DB.First(&actual, user.Id).Error)
	assert.Equal(t, UserStatusVerificationFrozen, actual.Status)
	assert.Equal(t, int64(100), actual.VerificationRequiredAt)
	assert.Equal(t, int64(300), actual.VerificationFrozenAt)
}

func TestGitHubBindingCountsAsVerificationProof(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "github", "1")
	now := time.Now().Unix()
	user := &User{Username: "verification-github", Password: "password", AffCode: "verification-github", Status: common.UserStatusEnabled, VerificationRequiredAt: now - 3600, GitHubId: "github-id"}
	require.NoError(t, DB.Create(user).Error)

	summary, err := EnforceAccountVerification(now)
	require.NoError(t, err)
	assert.Zero(t, summary.Frozen)
}

func TestHardDeleteVerificationUserUsesFrozenTimestampCAS(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	user := &User{Username: "verification-cas", Password: "password", AffCode: "verification-cas", Status: UserStatusVerificationFrozen, VerificationFrozenAt: 100}
	require.NoError(t, DB.Create(user).Error)

	removed, err := HardDeleteVerificationUser(user.Id, 101)
	require.NoError(t, err)
	assert.False(t, removed)
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, UserStatusVerificationFrozen, current.Status)
}

func TestEnforceAccountVerificationDoesNotDeletePromotedFrozenUser(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	now := time.Now().Unix()
	user := &User{
		Username:               "verification-promoted",
		Password:               "password",
		AffCode:                "verification-promoted",
		Role:                   common.RoleAdminUser,
		Status:                 UserStatusVerificationFrozen,
		VerificationRequiredAt: now - 3600,
		VerificationFrozenAt:   now - int64(setting.AccountVerificationDeletionDelayDays*24*60*60) - 1,
	}
	require.NoError(t, DB.Create(user).Error)

	summary, err := EnforceAccountVerification(now)
	require.NoError(t, err)
	assert.Zero(t, summary.Deleted)
	var current User
	require.NoError(t, DB.First(&current, user.Id).Error)
	assert.Equal(t, UserStatusVerificationFrozen, current.Status)
}

func TestVerificationReminderClaimCanBeRetriedAfterSendFailure(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	now := time.Now().Unix()
	user := &User{
		Username:               "verification-reminder",
		Password:               "password",
		AffCode:                "verification-reminder",
		Role:                   common.RoleCommonUser,
		Status:                 common.UserStatusEnabled,
		VerificationRequiredAt: now - 1,
	}
	require.NoError(t, DB.Create(user).Error)

	claimed, err := ClaimVerificationReminder(user.Id, now)
	require.NoError(t, err)
	assert.True(t, claimed)

	claimed, err = ClaimVerificationReminder(user.Id, now)
	require.NoError(t, err)
	assert.False(t, claimed)
	require.NoError(t, ReleaseVerificationReminder(user.Id, now))

	claimed, err = ClaimVerificationReminder(user.Id, now)
	require.NoError(t, err)
	assert.True(t, claimed)
	require.NoError(t, CompleteVerificationReminder(user.Id, now, now))

	claimed, err = ClaimVerificationReminder(user.Id, now)
	require.NoError(t, err)
	assert.False(t, claimed)
}

func TestMarkUserVerificationPendingOnlyEnrollsEnabledCommonUsers(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")

	admin := &User{Username: "verification-admin", Password: "password", AffCode: "verification-admin", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	disabled := &User{Username: "verification-disabled", Password: "password", AffCode: "verification-disabled", Role: common.RoleCommonUser, Status: common.UserStatusDisabled}
	require.NoError(t, DB.Create(admin).Error)
	require.NoError(t, DB.Create(disabled).Error)

	assert.Error(t, MarkUserVerificationPending(admin.Id, time.Now().Unix()))
	assert.Error(t, MarkUserVerificationPending(disabled.Id, time.Now().Unix()))

	var persisted User
	require.NoError(t, DB.First(&persisted, admin.Id).Error)
	assert.Zero(t, persisted.VerificationRequiredAt)
	persisted = User{}
	require.NoError(t, DB.First(&persisted, disabled.Id).Error)
	assert.Zero(t, persisted.VerificationRequiredAt)
}

func TestClearBindingInvalidatesVerificationProofAndReenrollsUser(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	user := &User{
		Username:        "verification-clear-email",
		Password:        "password",
		AffCode:         "verification-clear-email",
		Role:            common.RoleCommonUser,
		Status:          common.UserStatusEnabled,
		Email:           "verified@example.com",
		EmailVerifiedAt: 100,
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, user.ClearBinding("email"))
	var actual User
	require.NoError(t, DB.First(&actual, user.Id).Error)
	assert.Empty(t, actual.Email)
	assert.Zero(t, actual.EmailVerifiedAt)
	assert.NotZero(t, actual.VerificationRequiredAt)
}

func TestClearBindingDoesNotEnrollLegacyUserWithoutProof(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	user := &User{
		Username: "verification-clear-unverified",
		Password: "password",
		AffCode:  "verification-clear-unverified",
		Role:     common.RoleCommonUser,
		Status:   common.UserStatusEnabled,
		Email:    "unverified@example.com",
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, user.ClearBinding("email"))
	var actual User
	require.NoError(t, DB.First(&actual, user.Id).Error)
	assert.Empty(t, actual.Email)
	assert.Zero(t, actual.EmailVerifiedAt)
	assert.Zero(t, actual.VerificationRequiredAt)
}

func TestClearBindingDoesNotEnrollLegacyUserForUnrelatedProvider(t *testing.T) {
	truncateTables(t)
	configureAccountVerificationTest(t, "true", "email", "60")
	user := &User{
		Username:   "verification-clear-unrelated",
		Password:   "password",
		AffCode:    "verification-clear-unrelated",
		Role:       common.RoleCommonUser,
		Status:     common.UserStatusEnabled,
		TelegramId: "telegram-id",
	}
	require.NoError(t, DB.Create(user).Error)

	require.NoError(t, user.ClearBinding("telegram"))
	var actual User
	require.NoError(t, DB.First(&actual, user.Id).Error)
	assert.Empty(t, actual.TelegramId)
	assert.Zero(t, actual.TelegramVerifiedAt)
	assert.Zero(t, actual.VerificationRequiredAt)
}

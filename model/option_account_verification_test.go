package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAccountVerificationOptionValidationAndNormalization(t *testing.T) {
	require.NoError(t, validateOptionValue(setting.AccountVerificationEnabledOption, "true"))
	require.NoError(t, validateOptionValue(setting.AccountVerificationProvidersOption, "github,email"))
	require.NoError(t, validateOptionValue(setting.AccountVerificationFreezeDelayMinutesOption, "1440"))

	normalized, err := normalizeOptionValueForSave(setting.AccountVerificationProvidersOption, " GitHub, email,github ")
	require.NoError(t, err)
	assert.Equal(t, "email,github", normalized)

	for _, test := range []struct {
		key   string
		value string
	}{
		{key: setting.AccountVerificationEnabledOption, value: "yes"},
		{key: setting.AccountVerificationProvidersOption, value: "sms"},
		{key: setting.AccountVerificationProvidersOption, value: ""},
		{key: setting.AccountVerificationFreezeDelayMinutesOption, value: "0"},
	} {
		t.Run(test.key+"="+test.value, func(t *testing.T) {
			require.Error(t, validateOptionValue(test.key, test.value))
		})
	}
}

func TestAccountVerificationOptionsStateRequiresPersistedDisableForReset(t *testing.T) {
	tests := []struct {
		name               string
		options            []*Option
		enabledPresent     bool
		explicitlyDisabled bool
		invalid            bool
	}{
		{
			name: "missing policy",
		},
		{
			name: "missing enabled option",
			options: []*Option{
				{Key: setting.AccountVerificationProvidersOption, Value: "email"},
				{Key: setting.AccountVerificationFreezeDelayMinutesOption, Value: "60"},
			},
		},
		{
			name: "enabled",
			options: []*Option{
				{Key: setting.AccountVerificationEnabledOption, Value: "true"},
			},
			enabledPresent: true,
		},
		{
			name: "explicit disable",
			options: []*Option{
				{Key: setting.AccountVerificationEnabledOption, Value: "false"},
			},
			enabledPresent:     true,
			explicitlyDisabled: true,
		},
		{
			name: "malformed enabled",
			options: []*Option{
				{Key: setting.AccountVerificationEnabledOption, Value: "yes"},
			},
			enabledPresent: true,
			invalid:        true,
		},
		{
			name: "malformed policy does not look like disable",
			options: []*Option{
				{Key: setting.AccountVerificationEnabledOption, Value: "true"},
				{Key: setting.AccountVerificationProvidersOption, Value: "email,sms"},
			},
			enabledPresent: true,
			invalid:        true,
		},
		{
			name: "malformed policy blocks destructive reset",
			options: []*Option{
				{Key: setting.AccountVerificationEnabledOption, Value: "false"},
				{Key: setting.AccountVerificationProvidersOption, Value: "email,sms"},
			},
			enabledPresent:     true,
			explicitlyDisabled: true,
			invalid:            true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			enabledPresent, explicitlyDisabled, invalid := accountVerificationOptionsState(test.options)
			assert.Equal(t, test.enabledPresent, enabledPresent)
			assert.Equal(t, test.explicitlyDisabled, explicitlyDisabled)
			assert.Equal(t, test.invalid, invalid)
		})
	}
}

func TestAccountVerificationOptionsUpdateRuntime(t *testing.T) {
	originalEnabled := setting.AccountVerificationEnabled
	originalProviders := setting.AccountVerificationProviders
	originalDelay := setting.AccountVerificationFreezeDelayMinutes
	common.OptionMapRWMutex.RLock()
	originalMap := make(map[string]string, 3)
	originalMapPresent := make(map[string]bool, 3)
	for _, key := range []string{
		setting.AccountVerificationEnabledOption,
		setting.AccountVerificationProvidersOption,
		setting.AccountVerificationFreezeDelayMinutesOption,
	} {
		originalMap[key], originalMapPresent[key] = common.OptionMap[key]
	}
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		setting.AccountVerificationEnabled = originalEnabled
		setting.AccountVerificationProviders = originalProviders
		setting.AccountVerificationFreezeDelayMinutes = originalDelay
		common.OptionMapRWMutex.Lock()
		for key, value := range originalMap {
			if originalMapPresent[key] {
				common.OptionMap[key] = value
			} else {
				delete(common.OptionMap, key)
			}
		}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, updateOptionMap(setting.AccountVerificationEnabledOption, "true"))
	require.NoError(t, updateOptionMap(setting.AccountVerificationProvidersOption, "github,email"))
	require.NoError(t, updateOptionMap(setting.AccountVerificationFreezeDelayMinutesOption, "90"))

	assert.True(t, setting.AccountVerificationEnabled)
	assert.Equal(t, "email,github", setting.AccountVerificationProviders)
	assert.Equal(t, 90, setting.AccountVerificationFreezeDelayMinutes)
	common.OptionMapRWMutex.RLock()
	assert.Equal(t, "email,github", common.OptionMap[setting.AccountVerificationProvidersOption])
	assert.Equal(t, "90", common.OptionMap[setting.AccountVerificationFreezeDelayMinutesOption])
	common.OptionMapRWMutex.RUnlock()
}

func TestAccountVerificationMalformedOptionDoesNotReachRuntimeMap(t *testing.T) {
	common.OptionMapRWMutex.RLock()
	previous, hadPrevious := common.OptionMap[setting.AccountVerificationProvidersOption]
	previousEnabled, hadPreviousEnabled := common.OptionMap[setting.AccountVerificationEnabledOption]
	common.OptionMapRWMutex.RUnlock()
	originalEnabled := setting.AccountVerificationEnabled
	setting.AccountVerificationEnabled = true
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[setting.AccountVerificationEnabledOption] = "true"
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		setting.AccountVerificationEnabled = originalEnabled
		common.OptionMapRWMutex.Lock()
		if hadPrevious {
			common.OptionMap[setting.AccountVerificationProvidersOption] = previous
		} else {
			delete(common.OptionMap, setting.AccountVerificationProvidersOption)
		}
		if hadPreviousEnabled {
			common.OptionMap[setting.AccountVerificationEnabledOption] = previousEnabled
		} else {
			delete(common.OptionMap, setting.AccountVerificationEnabledOption)
		}
		common.OptionMapRWMutex.Unlock()
	})

	require.Error(t, updateOptionMapFromDatabase(setting.AccountVerificationProvidersOption, "email,sms"))
	common.OptionMapRWMutex.RLock()
	actual := common.OptionMap[setting.AccountVerificationProvidersOption]
	actualEnabled := common.OptionMap[setting.AccountVerificationEnabledOption]
	common.OptionMapRWMutex.RUnlock()
	assert.Equal(t, previous, actual)
	assert.False(t, setting.AccountVerificationEnabled)
	assert.Equal(t, "false", actualEnabled)
}

func TestDisablingAccountVerificationReleasesLifecycleUsers(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, DB.Exec("DELETE FROM options").Error)
	t.Cleanup(func() { DB.Exec("DELETE FROM options") })
	configureAccountVerificationTest(t, "true", "email", "60")

	pending := &User{
		Username:                   "verification-disable-pending",
		Password:                   "password",
		AffCode:                    "verification-disable-pending",
		Role:                       common.RoleCommonUser,
		Status:                     common.UserStatusEnabled,
		VerificationRequiredAt:     100,
		VerificationReminderSentAt: 200,
	}
	frozen := &User{
		Username:               "verification-disable-frozen",
		Password:               "password",
		AffCode:                "verification-disable-frozen",
		Role:                   common.RoleCommonUser,
		Status:                 UserStatusVerificationFrozen,
		VerificationRequiredAt: 100,
		VerificationFrozenAt:   300,
	}
	disabled := &User{
		Username:               "verification-disable-disabled",
		Password:               "password",
		AffCode:                "verification-disable-disabled",
		Role:                   common.RoleCommonUser,
		Status:                 common.UserStatusDisabled,
		VerificationRequiredAt: 400,
	}
	require.NoError(t, DB.Create(pending).Error)
	require.NoError(t, DB.Create(frozen).Error)
	require.NoError(t, DB.Create(disabled).Error)

	require.NoError(t, DB.Create(&Option{Key: setting.AccountVerificationEnabledOption, Value: "true"}).Error)
	require.NoError(t, UpdateOption(setting.AccountVerificationEnabledOption, "false"))

	var actual User
	require.NoError(t, DB.First(&actual, pending.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, actual.Status)
	assert.Zero(t, actual.VerificationRequiredAt)
	assert.Zero(t, actual.VerificationReminderSentAt)
	actual = User{}
	require.NoError(t, DB.First(&actual, frozen.Id).Error)
	assert.Equal(t, common.UserStatusEnabled, actual.Status)
	assert.Zero(t, actual.VerificationRequiredAt)
	assert.Zero(t, actual.VerificationFrozenAt)
	actual = User{}
	require.NoError(t, DB.First(&actual, disabled.Id).Error)
	assert.Equal(t, common.UserStatusDisabled, actual.Status)
	assert.Zero(t, actual.VerificationRequiredAt)
}

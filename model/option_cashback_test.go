package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const amountCashbackOptionKey = "payment_setting.amount_cashback"

func TestValidateOptionValueCashback(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "valid zero values", value: `[{"min_amount":0,"cashback_percent":0}]`},
		{name: "valid fractional values", value: `[{"min_amount":0.1,"cashback_percent":1.5}]`},
		{name: "missing min amount", value: `[{"cashback_percent":1}]`, wantError: true},
		{name: "missing cashback percent", value: `[{"min_amount":1}]`, wantError: true},
		{name: "null", value: `[{"min_amount":null,"cashback_percent":1}]`, wantError: true},
		{name: "numeric string", value: `[{"min_amount":"1","cashback_percent":1}]`, wantError: true},
		{name: "invalid JSON number", value: `[{"min_amount":NaN,"cashback_percent":1}]`, wantError: true},
		{name: "negative amount", value: `[{"min_amount":-1,"cashback_percent":1}]`, wantError: true},
		{name: "percent over 100", value: `[{"min_amount":1,"cashback_percent":101}]`, wantError: true},
		{name: "duplicate min amount", value: `[{"min_amount":1,"cashback_percent":1},{"min_amount":1,"cashback_percent":2}]`, wantError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOptionValue(amountCashbackOptionKey, tc.value)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestUpdateOptionsBulkValidatesReferralCashbackInEachTier(t *testing.T) {
	preserveAmountCashbackOptionState(t)

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		amountCashbackOptionKey: `[{"min_amount":0,"cashback_percent":7,"referral_cashback_percent":8}]`,
	}))
	referralPercent := 8.0
	require.Equal(t, operation_setting.AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 7, ReferralCashbackPercent: &referralPercent}}, operation_setting.GetPaymentSetting().AmountCashback)

	require.Error(t, UpdateOptionsBulk(map[string]string{
		amountCashbackOptionKey: `[{"min_amount":0,"cashback_percent":8,"referral_cashback_percent":7}]`,
	}))
	require.Equal(t, operation_setting.AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 7, ReferralCashbackPercent: &referralPercent}}, operation_setting.GetPaymentSetting().AmountCashback)

	var cashback Option
	require.NoError(t, DB.First(&cashback, "key = ?", amountCashbackOptionKey).Error)
	require.Equal(t, `[{"min_amount":0,"cashback_percent":7,"referral_cashback_percent":8}]`, cashback.Value)
}

func TestDeprecatedReferralCashbackOptionIsRejectedAndRemoved(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))

	require.Error(t, UpdateOption(deprecatedReferralCashbackOption, "10"))
	var option Option
	require.ErrorIs(t, DB.First(&option, "key = ?", deprecatedReferralCashbackOption).Error, gorm.ErrRecordNotFound)

	require.NoError(t, DB.Create(&Option{Key: deprecatedReferralCashbackOption, Value: "10"}).Error)
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMap[deprecatedReferralCashbackOption] = "10"
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, removeDeprecatedReferralCashbackOption())
	require.ErrorIs(t, DB.First(&option, "key = ?", deprecatedReferralCashbackOption).Error, gorm.ErrRecordNotFound)
	common.OptionMapRWMutex.RLock()
	_, exists := common.OptionMap[deprecatedReferralCashbackOption]
	common.OptionMapRWMutex.RUnlock()
	require.False(t, exists)
}

func TestNormalizeAmountCashbackOptionValueForSave(t *testing.T) {
	testCases := []struct {
		name     string
		value    string
		expected string
	}{
		{name: "legacy null", value: `null`, expected: `[]`},
		{name: "empty array", value: `[]`, expected: `[]`},
		{
			name:     "valid array",
			value:    "[\n  {\"min_amount\": 0.1, \"cashback_percent\": 1.5}\n]",
			expected: `[{"min_amount":0.1,"cashback_percent":1.5}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			normalized, err := normalizeOptionValueForSave(amountCashbackOptionKey, tc.value)
			require.NoError(t, err)
			require.Equal(t, tc.expected, normalized)
		})
	}

	_, err := normalizeOptionValueForSave(amountCashbackOptionKey, `{}`)
	require.Error(t, err)
}

func preserveAmountCashbackOptionState(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&Option{}))

	var originalOption Option
	optionErr := DB.First(&originalOption, "key = ?", amountCashbackOptionKey).Error
	if optionErr != nil {
		require.ErrorIs(t, optionErr, gorm.ErrRecordNotFound)
	}
	var originalCashbacks operation_setting.AmountCashbackConfig
	if current := operation_setting.GetPaymentSetting().AmountCashback; current != nil {
		originalCashbacks = append(operation_setting.AmountCashbackConfig{}, current...)
	}
	common.OptionMapRWMutex.RLock()
	originalMapValue, hadOriginalMapValue := common.OptionMap[amountCashbackOptionKey]
	common.OptionMapRWMutex.RUnlock()

	t.Cleanup(func() {
		_ = DB.Delete(&Option{}, "key = ?", amountCashbackOptionKey).Error
		if optionErr == nil {
			_ = DB.Create(&originalOption).Error
		}
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
		common.OptionMapRWMutex.Lock()
		if hadOriginalMapValue {
			common.OptionMap[amountCashbackOptionKey] = originalMapValue
		} else {
			delete(common.OptionMap, amountCashbackOptionKey)
		}
		common.OptionMapRWMutex.Unlock()
	})
}

func TestUpdateOptionCanonicalizesAmountCashbackAtomically(t *testing.T) {
	preserveAmountCashbackOptionState(t)
	require.NoError(t, DB.Delete(&Option{}, "key = ?", amountCashbackOptionKey).Error)

	for _, value := range []string{`null`, `[]`} {
		require.NoError(t, UpdateOption(amountCashbackOptionKey, value))
		var stored Option
		require.NoError(t, DB.First(&stored, "key = ?", amountCashbackOptionKey).Error)
		require.Equal(t, `[]`, stored.Value)
		common.OptionMapRWMutex.RLock()
		require.Equal(t, `[]`, common.OptionMap[amountCashbackOptionKey])
		common.OptionMapRWMutex.RUnlock()
	}

	expected := `[{"min_amount":0.1,"cashback_percent":1.5}]`
	require.NoError(t, UpdateOption(amountCashbackOptionKey, "[\n{\"min_amount\":0.1,\"cashback_percent\":1.5}\n]"))
	var stored Option
	require.NoError(t, DB.First(&stored, "key = ?", amountCashbackOptionKey).Error)
	require.Equal(t, expected, stored.Value)
	common.OptionMapRWMutex.RLock()
	require.Equal(t, expected, common.OptionMap[amountCashbackOptionKey])
	common.OptionMapRWMutex.RUnlock()
	require.Equal(t, operation_setting.AmountCashbackConfig{{MinAmount: 0.1, CashbackPercent: 1.5}}, operation_setting.GetPaymentSetting().AmountCashback)

	require.Error(t, UpdateOption(amountCashbackOptionKey, `[{"min_amount":1}]`))
	var afterInvalid Option
	require.NoError(t, DB.First(&afterInvalid, "key = ?", amountCashbackOptionKey).Error)
	require.Equal(t, expected, afterInvalid.Value)
	common.OptionMapRWMutex.RLock()
	require.Equal(t, expected, common.OptionMap[amountCashbackOptionKey])
	common.OptionMapRWMutex.RUnlock()
	require.Equal(t, operation_setting.AmountCashbackConfig{{MinAmount: 0.1, CashbackPercent: 1.5}}, operation_setting.GetPaymentSetting().AmountCashback)
}

func TestUpdateOptionMapFromDatabaseCanonicalizesLegacyNullCashback(t *testing.T) {
	preserveAmountCashbackOptionState(t)
	require.NoError(t, DB.Save(&Option{Key: amountCashbackOptionKey, Value: `null`}).Error)

	var loaded Option
	require.NoError(t, DB.First(&loaded, "key = ?", amountCashbackOptionKey).Error)
	require.NoError(t, updateOptionMapFromDatabase(loaded.Key, loaded.Value))

	common.OptionMapRWMutex.RLock()
	require.Equal(t, `[]`, common.OptionMap[amountCashbackOptionKey])
	common.OptionMapRWMutex.RUnlock()
	require.NotNil(t, operation_setting.GetPaymentSetting().AmountCashback)
	require.Empty(t, operation_setting.GetPaymentSetting().AmountCashback)

	var unchanged Option
	require.NoError(t, DB.First(&unchanged, "key = ?", amountCashbackOptionKey).Error)
	require.Equal(t, `null`, unchanged.Value)
}

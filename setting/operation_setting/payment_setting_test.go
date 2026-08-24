package operation_setting

import (
	"math"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestAmountCashbackConfigUsesHighestMatchingMinimum(t *testing.T) {
	var cashbacks AmountCashbackConfig
	require.NoError(t, common.Unmarshal([]byte(`[
		{"min_amount":100,"cashback_percent":1},
		{"min_amount":200,"cashback_percent":3},
		{"min_amount":150,"cashback_percent":2}
	]`), &cashbacks))

	require.Equal(t, 0.0, cashbacks.CashbackPercentForAmount(99))
	require.Equal(t, 1.0, cashbacks.CashbackPercentForAmount(100))
	require.Equal(t, 2.0, cashbacks.CashbackPercentForAmount(151))
	require.Equal(t, 3.0, cashbacks.CashbackPercentForAmount(250))
}

func TestAmountCashbackConfigAllowsZeroPercentAtThreshold(t *testing.T) {
	cashbacks := AmountCashbackConfig{
		{MinAmount: 10, CashbackPercent: 1},
		{MinAmount: 20, CashbackPercent: 0},
	}

	require.Equal(t, 1.0, cashbacks.CashbackPercentForAmount(19))
	require.Equal(t, 0.0, cashbacks.CashbackPercentForAmount(20))
}

func TestValidateAmountCashback(t *testing.T) {
	testCases := []struct {
		name      string
		cashbacks AmountCashbackConfig
		wantError bool
	}{
		{name: "fractional threshold", cashbacks: AmountCashbackConfig{{MinAmount: 0.1, CashbackPercent: 1}}},
		{name: "zero values", cashbacks: AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 0}}},
		{name: "negative amount", cashbacks: AmountCashbackConfig{{MinAmount: -0.1, CashbackPercent: 1}}, wantError: true},
		{name: "non-finite amount", cashbacks: AmountCashbackConfig{{MinAmount: math.Inf(1), CashbackPercent: 1}}, wantError: true},
		{name: "negative percent", cashbacks: AmountCashbackConfig{{MinAmount: 1, CashbackPercent: -1}}, wantError: true},
		{name: "percent over 100", cashbacks: AmountCashbackConfig{{MinAmount: 1, CashbackPercent: 101}}, wantError: true},
		{name: "non-finite percent", cashbacks: AmountCashbackConfig{{MinAmount: 1, CashbackPercent: math.NaN()}}, wantError: true},
		{name: "duplicate amount", cashbacks: AmountCashbackConfig{{MinAmount: 1, CashbackPercent: 1}, {MinAmount: 1, CashbackPercent: 2}}, wantError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateAmountCashback(tc.cashbacks)
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAmountCashbackConfigJSONContract(t *testing.T) {
	testCases := []struct {
		name      string
		value     string
		wantError bool
	}{
		{name: "valid zero values", value: `[{"min_amount":0,"cashback_percent":0}]`},
		{name: "valid fractional threshold", value: `[{"min_amount":0.1,"cashback_percent":1.5}]`},
		{name: "missing min amount", value: `[{"cashback_percent":1}]`, wantError: true},
		{name: "missing cashback percent", value: `[{"min_amount":1}]`, wantError: true},
		{name: "null min amount", value: `[{"min_amount":null,"cashback_percent":1}]`, wantError: true},
		{name: "null cashback percent", value: `[{"min_amount":1,"cashback_percent":null}]`, wantError: true},
		{name: "string min amount", value: `[{"min_amount":"1","cashback_percent":1}]`, wantError: true},
		{name: "string cashback percent", value: `[{"min_amount":1,"cashback_percent":"1"}]`, wantError: true},
		{name: "NaN is invalid JSON", value: `[{"min_amount":NaN,"cashback_percent":1}]`, wantError: true},
		{name: "invalid JSON", value: `[{"min_amount":1,]`, wantError: true},
		{name: "object config", value: `{}`, wantError: true},
		{name: "duplicate min amount", value: `[{"min_amount":1,"cashback_percent":1},{"min_amount":1,"cashback_percent":2}]`, wantError: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cashbacks AmountCashbackConfig
			err := common.Unmarshal([]byte(tc.value), &cashbacks)
			if err == nil {
				err = ValidateAmountCashback(cashbacks)
			}
			if tc.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestAmountCashbackConfigMarshalContract(t *testing.T) {
	testCases := []struct {
		name      string
		cashbacks AmountCashbackConfig
		expected  string
	}{
		{name: "nil", cashbacks: nil, expected: `[]`},
		{name: "empty", cashbacks: AmountCashbackConfig{}, expected: `[]`},
		{
			name:      "values",
			cashbacks: AmountCashbackConfig{{MinAmount: 0, CashbackPercent: 0}, {MinAmount: 1.5, CashbackPercent: 2.5}},
			expected:  `[{"min_amount":0,"cashback_percent":0},{"min_amount":1.5,"cashback_percent":2.5}]`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := common.Marshal(tc.cashbacks)
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(data))
		})
	}
}

func TestAmountCashbackConfigEmptyAndLegacyNullRoundTripAsArray(t *testing.T) {
	for _, input := range []string{`[]`, `null`} {
		t.Run(input, func(t *testing.T) {
			var cashbacks AmountCashbackConfig
			require.NoError(t, common.Unmarshal([]byte(input), &cashbacks))
			require.NotNil(t, cashbacks)
			require.Empty(t, cashbacks)

			data, err := common.Marshal(cashbacks)
			require.NoError(t, err)
			require.JSONEq(t, `[]`, string(data))
		})
	}
}

func TestPaymentSettingDefaultConfigExportsEmptyCashbackArray(t *testing.T) {
	values, err := config.ConfigToMap(&PaymentSetting{})
	require.NoError(t, err)
	require.JSONEq(t, `[]`, values["amount_cashback"])
}

func TestPaymentSettingRuntimeConfigRejectsInvalidCashbackAtomically(t *testing.T) {
	original := AmountCashbackConfig{{MinAmount: 10, CashbackPercent: 1}}
	setting := PaymentSetting{AmountCashback: original}

	require.NoError(t, config.UpdateConfigFromMap(&setting, map[string]string{
		"amount_cashback": `[{"min_amount":20}]`,
	}))
	require.Equal(t, original, setting.AmountCashback)

	require.NoError(t, config.UpdateConfigFromMap(&setting, map[string]string{
		"amount_cashback": `[{"min_amount":20,"cashback_percent":2}]`,
	}))
	require.Equal(t, AmountCashbackConfig{{MinAmount: 20, CashbackPercent: 2}}, setting.AmountCashback)
}

func TestPaymentSettingRuntimeConfigNormalizesLegacyNullCashback(t *testing.T) {
	setting := PaymentSetting{
		AmountCashback: AmountCashbackConfig{{MinAmount: 10, CashbackPercent: 1}},
	}

	require.NoError(t, config.UpdateConfigFromMap(&setting, map[string]string{
		"amount_cashback": `null`,
	}))
	require.NotNil(t, setting.AmountCashback)
	require.Empty(t, setting.AmountCashback)

	values, err := config.ConfigToMap(&setting)
	require.NoError(t, err)
	require.JSONEq(t, `[]`, values["amount_cashback"])
}

func TestPaymentSettingUnmarshalsFractionalAmountOptions(t *testing.T) {
	var setting PaymentSetting
	require.NoError(t, common.Unmarshal([]byte(`{"amount_options":[0.1,1.5]}`), &setting))
	require.Equal(t, []float64{0.1, 1.5}, setting.AmountOptions)
}

func TestEnsureYooKassaPayMethodAddsEditableDefaultOnce(t *testing.T) {
	methods := []map[string]string{{"name": "Custom", "type": "custom1"}}
	methods, changed := EnsureYooKassaPayMethod(methods, true)
	require.True(t, changed)
	require.Len(t, methods, 2)
	require.Equal(t, map[string]string{"name": "СБП", "type": YooKassaSBPPaymentMethod, "topup_group": "default"}, methods[1])
	methods, changed = EnsureYooKassaPayMethod(methods, true)
	require.False(t, changed)
	require.Len(t, methods, 2)
}

func TestEnsureYooKassaPayMethodPreservesExistingConfiguration(t *testing.T) {
	methods := []map[string]string{{"name": "СБП", "type": YooKassaSBPPaymentMethod, "topup_group": "premium", "icon": "custom-icon"}}
	normalized, changed := EnsureYooKassaPayMethod(methods, true)
	require.False(t, changed)
	require.Equal(t, methods, normalized)
}

func TestEnsureYooKassaPayMethodNormalizesCurrencyAndDeduplicates(t *testing.T) {
	methods := []map[string]string{
		{"name": "СБП", "type": YooKassaSBPPaymentMethod, "currency": "USD", "topup_group": "premium", "icon": "custom-icon"},
		{"name": "Duplicate", "type": YooKassaSBPPaymentMethod, "topup_group": "default"},
	}

	normalized, changed := EnsureYooKassaPayMethod(methods, true)
	require.True(t, changed)
	require.Len(t, normalized, 1)
	require.Equal(t, "СБП", normalized[0]["name"])
	require.Equal(t, "custom-icon", normalized[0]["icon"])
	_, hasCurrency := normalized[0]["currency"]
	require.False(t, hasCurrency)
	require.Equal(t, "premium", normalized[0]["topup_group"])
}

func TestEnsureYooKassaPayMethodCanonicalizesLegacyType(t *testing.T) {
	methods, changed := EnsureYooKassaPayMethod([]map[string]string{{
		"name": "СБП", "type": " YOOKASSA_SBP ", "currency": "RUB", "topup_group": "premium",
	}}, true)
	require.True(t, changed)
	require.Len(t, methods, 1)
	require.Equal(t, YooKassaSBPPaymentMethod, methods[0]["type"])
}

func TestEnsureYooKassaPayMethodDoesNotAddWhenDisabled(t *testing.T) {
	methods, changed := EnsureYooKassaPayMethod(nil, false)
	require.False(t, changed)
	require.Empty(t, methods)
}

func TestNormalizePayMethodsRemovesLegacyGatewayCurrencies(t *testing.T) {
	methods := []map[string]string{
		{"type": "nowpayments", "currency": "USD"},
		{"type": "stripe", "currency": "RUB"},
		{"type": " yookassa_sbp ", "currency": "USD"},
		{"type": "waffo_pancake", "currency": "EUR"},
	}
	NormalizePayMethods(methods)
	for _, method := range methods {
		_, hasCurrency := method["currency"]
		require.False(t, hasCurrency)
	}
	require.Equal(t, "yookassa_sbp", methods[2]["type"])
}

func TestNormalizePayMethodsNormalizesLegacyYooKassaLabels(t *testing.T) {
	methods := []map[string]string{
		{"type": YooKassaSBPPaymentMethod, "name": "СБП / YooKassa"},
		{"type": YooKassaSBPPaymentMethod, "name": "Custom SBP"},
	}
	NormalizePayMethods(methods)
	require.Equal(t, "СБП", methods[0]["name"])
	require.Equal(t, "Custom SBP", methods[1]["name"])
}

func TestNormalizePayMethodsRemovesProviderOwnedMinimums(t *testing.T) {
	methods := []map[string]string{
		{"type": "stripe", "min_topup": "100"},
		{"type": "waffo", "min_topup": "100"},
		{"type": "waffo_pancake", "min_topup": "100"},
		{"type": "yookassa_sbp", "min_topup": "100"},
		{"type": "nowpayments", "min_topup": "100"},
		{"type": "alipay", "min_topup": "10"},
	}

	NormalizePayMethods(methods)

	for _, method := range methods[:5] {
		_, exists := method["min_topup"]
		require.False(t, exists, "provider-owned minimum should be removed for %q", method["type"])
	}
	require.Equal(t, "10", methods[5]["min_topup"])
}

func TestValidatePayMethodsPendingTTL(t *testing.T) {
	require.NoError(t, ValidatePayMethods([]map[string]string{
		{"type": "alipay"},
		{"type": "alipay", "pending_ttl_minutes": "inherit"},
		{"type": "alipay", "pending_ttl_minutes": "525600"},
	}))
	for _, ttl := range []string{"0", "-1", "525601", "one hour"} {
		require.Error(t, ValidatePayMethods([]map[string]string{{"type": "alipay", "pending_ttl_minutes": ttl}}), ttl)
	}
}

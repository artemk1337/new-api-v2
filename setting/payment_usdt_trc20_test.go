package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateTRONAddressUsesBase58CheckAndMainnetVersion(t *testing.T) {
	validAddress := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	require.NoError(t, ValidateTRONAddress(validAddress))
	assert.Error(t, ValidateTRONAddress(USDTTRC20Contract))
	assert.Error(t, ValidateTRONAddress("TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj60"))
	assert.Error(t, ValidateTRONAddress("0x41"+USDTTRC20Contract))
	assert.Error(t, ValidateTRONAddress(""))
}

func TestValidateDirectUSDTConfigValuesFailsClosedWhenEnabled(t *testing.T) {
	require.NoError(t, ValidateDirectUSDTConfigValues(false, "", ""))
	assert.Error(t, ValidateDirectUSDTConfigValues(true, "", "read-only-key"))
	validAddress := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	assert.Error(t, ValidateDirectUSDTConfigValues(true, validAddress, ""))
	require.NoError(t, ValidateDirectUSDTConfigValues(true, validAddress, "read-only-key"))
}

func TestUSDTTRC20AmountSuffixDefaultsPreserveLegacyRange(t *testing.T) {
	assert.Equal(t, 1, DefaultUSDTTRC20AmountSuffixMinUnits)
	assert.Equal(t, 9999, DefaultUSDTTRC20AmountSuffixMaxUnits)
	assert.NoError(t, ValidateUSDTTRC20AmountSuffixRange(
		DefaultUSDTTRC20AmountSuffixMinUnits,
		DefaultUSDTTRC20AmountSuffixMaxUnits,
	))
}

func TestUSDTTRC20AmountTailLimitRanges(t *testing.T) {
	assert.Equal(t, 10_000, DefaultUSDTTRC20AmountTailLimitUnits)
	for _, limit := range []int{2, 10, 100, 1_000, 10_000} {
		assert.NoError(t, ValidateUSDTTRC20AmountTailLimit(limit))
		min, max, step, err := USDTTRC20AmountSuffixRangeForLimit(limit)
		require.NoError(t, err)
		assert.Equal(t, 1, min)
		assert.Equal(t, limit-1, max)
		assert.Equal(t, 1, step)
		assert.Equal(t, limit-1, max-min+1)
	}
	for _, limit := range []int{0, 1, 10_001} {
		assert.Error(t, ValidateUSDTTRC20AmountTailLimit(limit))
	}
}

func TestUSDTTRC20AmountTailLimitMigratesLegacyPrecision(t *testing.T) {
	for _, test := range []struct {
		precision int
		limit     int
	}{{3, 10}, {4, 100}, {5, 1_000}, {6, 10_000}} {
		limit, err := USDTTRC20AmountTailLimitForPrecision(test.precision)
		require.NoError(t, err)
		assert.Equal(t, test.limit, limit)
	}
	_, err := USDTTRC20AmountTailLimitForPrecision(2)
	assert.Error(t, err)
}

func TestValidateUSDTTRC20AmountSuffixRange(t *testing.T) {
	for _, values := range [][2]int{{1, 1}, {1, 9999}, {9999, 9999}} {
		assert.NoError(t, ValidateUSDTTRC20AmountSuffixRange(values[0], values[1]))
	}
	for _, values := range [][2]int{{-1, 1}, {0, 1}, {1, 0}, {0, 10000}} {
		assert.Error(t, ValidateUSDTTRC20AmountSuffixRange(values[0], values[1]))
	}
}

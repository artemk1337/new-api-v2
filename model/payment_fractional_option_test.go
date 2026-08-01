package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionMapLoadsFractionalProviderMinimums(t *testing.T) {
	originalStripeMinTopUp := setting.StripeMinTopUp
	originalWaffoMinTopUp := setting.WaffoMinTopUp
	originalWaffoPancakeMinTopUp := setting.WaffoPancakeMinTopUp
	t.Cleanup(func() {
		setting.StripeMinTopUp = originalStripeMinTopUp
		setting.WaffoMinTopUp = originalWaffoMinTopUp
		setting.WaffoPancakeMinTopUp = originalWaffoPancakeMinTopUp
	})

	for _, key := range []string{"StripeMinTopUp", "WaffoMinTopUp", "WaffoPancakeMinTopUp"} {
		require.NoError(t, updateOptionMapFromDatabase(key, "0.1"))
		assert.Equal(t, "0.1", common.OptionMap[key])
	}

	assert.Equal(t, 0.1, setting.StripeMinTopUp)
	assert.Equal(t, 0.1, setting.WaffoMinTopUp)
	assert.Equal(t, 0.1, setting.WaffoPancakeMinTopUp)
}

package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNOWPaymentsEnabledOptionUpdatesRuntime(t *testing.T) {
	originalEnabled := setting.NOWPaymentsEnabled
	common.OptionMapRWMutex.RLock()
	originalValue, hadOriginalValue := common.OptionMap["NOWPaymentsEnabled"]
	common.OptionMapRWMutex.RUnlock()
	t.Cleanup(func() {
		setting.NOWPaymentsEnabled = originalEnabled
		common.OptionMapRWMutex.Lock()
		if hadOriginalValue {
			common.OptionMap["NOWPaymentsEnabled"] = originalValue
		} else {
			delete(common.OptionMap, "NOWPaymentsEnabled")
		}
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, updateOptionMap("NOWPaymentsEnabled", "true"))
	assert.True(t, setting.NOWPaymentsEnabled)

	require.NoError(t, updateOptionMap("NOWPaymentsEnabled", "false"))
	assert.False(t, setting.NOWPaymentsEnabled)
}

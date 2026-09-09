package operation_setting

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
)

func TestRegistrationRateLimitReadsLiveOptionsAndClampsLegacyValues(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{
		RegistrationRateLimitEnabled:       "false",
		RegistrationRateLimitAttempts:      "2000",
		RegistrationRateLimitSuccesses:     "7",
		RegistrationRateLimitWindowMinutes: "2000",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	config := RegistrationRateLimit()
	assert.False(t, config.Enabled)
	assert.Equal(t, MaxRegistrationRateLimitCount, config.Attempts)
	assert.Equal(t, 7, config.Successes)
	assert.Equal(t, 24*time.Hour, config.Window)
}

func TestRegistrationRateLimitUsesSafeDefaultsForMissingOptions(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	previous := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previous
		common.OptionMapRWMutex.Unlock()
	})

	config := RegistrationRateLimit()
	assert.True(t, config.Enabled)
	assert.Equal(t, DefaultRegistrationRateLimitAttempts, config.Attempts)
	assert.Equal(t, DefaultRegistrationRateLimitSuccesses, config.Successes)
	assert.Equal(t, 24*time.Hour, config.Window)
}

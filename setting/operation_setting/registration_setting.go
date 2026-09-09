package operation_setting

import (
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const (
	RegistrationRateLimitEnabled       = "RegistrationRateLimitEnabled"
	RegistrationRateLimitAttempts      = "RegistrationRateLimitAttempts"
	RegistrationRateLimitSuccesses     = "RegistrationRateLimitSuccesses"
	RegistrationRateLimitWindowMinutes = "RegistrationRateLimitWindowMinutes"

	DefaultRegistrationRateLimitAttempts      = 10
	DefaultRegistrationRateLimitSuccesses     = 3
	DefaultRegistrationRateLimitWindowMinutes = 1440
	MaxRegistrationRateLimitCount             = 1000
	MaxRegistrationRateLimitWindowMinutes     = 1440
	RegistrationRateLimitRetention            = 24 * time.Hour
)

type RegistrationRateLimitConfig struct {
	Enabled   bool
	Attempts  int
	Successes int
	Window    time.Duration
}

func RegistrationRateLimit() RegistrationRateLimitConfig {
	common.OptionMapRWMutex.RLock()
	enabledValue, enabledExists := common.OptionMap[RegistrationRateLimitEnabled]
	attemptsValue := common.OptionMap[RegistrationRateLimitAttempts]
	successesValue := common.OptionMap[RegistrationRateLimitSuccesses]
	windowValue := common.OptionMap[RegistrationRateLimitWindowMinutes]
	common.OptionMapRWMutex.RUnlock()

	enabled := true
	if enabledExists {
		parsed, err := strconv.ParseBool(strings.TrimSpace(enabledValue))
		if err == nil {
			enabled = parsed
		}
	}
	attempts := positiveBoundedOption(attemptsValue, DefaultRegistrationRateLimitAttempts, MaxRegistrationRateLimitCount)
	successes := positiveBoundedOption(successesValue, DefaultRegistrationRateLimitSuccesses, MaxRegistrationRateLimitCount)
	minutes := positiveBoundedOption(windowValue, DefaultRegistrationRateLimitWindowMinutes, MaxRegistrationRateLimitWindowMinutes)
	return RegistrationRateLimitConfig{
		Enabled:   enabled,
		Attempts:  attempts,
		Successes: successes,
		Window:    time.Duration(minutes) * time.Minute,
	}
}

func positiveBoundedOption(value string, fallback, maximum int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return min(parsed, maximum)
}

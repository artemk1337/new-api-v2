package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func TestRegistrationRateLimitOptionsAreServerValidated(t *testing.T) {
	for _, key := range []string{
		operation_setting.RegistrationRateLimitAttempts,
		operation_setting.RegistrationRateLimitSuccesses,
	} {
		require.NoError(t, validateOptionValue(key, "1"))
		require.NoError(t, validateOptionValue(key, "1000"))
		require.Error(t, validateOptionValue(key, "0"))
		require.Error(t, validateOptionValue(key, "1001"))
	}
	require.NoError(t, validateOptionValue(operation_setting.RegistrationRateLimitWindowMinutes, "1440"))
	require.Error(t, validateOptionValue(operation_setting.RegistrationRateLimitWindowMinutes, "1441"))
	require.NoError(t, validateOptionValue(operation_setting.RegistrationRateLimitEnabled, "true"))
	require.NoError(t, validateOptionValue(operation_setting.RegistrationRateLimitEnabled, "false"))
	require.Error(t, validateOptionValue(operation_setting.RegistrationRateLimitEnabled, "1"))
}

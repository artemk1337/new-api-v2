package system_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/require"
)

func TestPasskeyEnabledByDefaultAndConfigurable(t *testing.T) {
	settings := GetPasskeySettings()
	original := *settings
	t.Cleanup(func() {
		*settings = original
	})

	require.True(t, settings.Enabled)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"passkey.enabled": "false",
	}))
	require.False(t, settings.Enabled)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"passkey.enabled": "true",
	}))
	require.True(t, settings.Enabled)
}

package console_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLegacyUptimeKumaOptionsAreIgnored(t *testing.T) {
	settings := defaultConsoleSetting
	manager := config.NewConfigManager()
	manager.Register("console_setting", &settings)

	require.NoError(t, manager.LoadFromDB(map[string]string{
		"console_setting.api_info_enabled":    "false",
		"console_setting.uptime_kuma_enabled": "true",
		"console_setting.uptime_kuma_groups":  `[{"url":"https://status.example.com"}]`,
	}))

	assert.False(t, settings.ApiInfoEnabled)
	assert.NotContains(t, manager.ExportAllConfigs(), "console_setting.uptime_kuma_enabled")
	assert.NotContains(t, manager.ExportAllConfigs(), "console_setting.uptime_kuma_groups")
}

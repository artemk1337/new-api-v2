package system_setting

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIPBlacklist(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty list", value: `[]`},
		{name: "addresses and prefixes", value: `["203.0.113.7", "2001:db8::/32"]`},
		{name: "null is not an array", value: `null`, wantErr: true},
		{name: "malformed JSON", value: `"203.0.113.7"`, wantErr: true},
		{name: "invalid address", value: `["not-an-ip"]`, wantErr: true},
		{name: "empty entry", value: `[""]`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIPBlacklist(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestIsIPBlacklistedMatchesAddressAndCIDR(t *testing.T) {
	settings := GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})

	require.NoError(t, settings.UpdateConfigFromMap(map[string]string{
		"ip_blacklist": `["203.0.113.7", "2001:db8::/32"]`,
	}))
	assert.True(t, IsIPBlacklisted("203.0.113.7"))
	assert.False(t, IsIPBlacklisted("203.0.113.8"))
	assert.True(t, IsIPBlacklisted("2001:db8::10"))
	assert.False(t, IsIPBlacklisted("2001:db9::10"))
	assert.False(t, IsIPBlacklisted("not-an-ip"))
}

func TestSecuritySettingLoadsThroughConfigManager(t *testing.T) {
	settings := GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"security.ip_blacklist": `["198.51.100.4"]`,
	}))
	assert.Equal(t, []string{"198.51.100.4"}, settings.IPBlacklist)
}

func TestIsIPBlacklistedCanonicalizesMappedAndCompatibleIPv4(t *testing.T) {
	settings := GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})
	require.NoError(t, settings.UpdateConfigFromMap(map[string]string{
		"ip_blacklist": `["203.0.113.7", "198.51.100.0/24"]`,
	}))

	assert.True(t, IsIPBlacklisted("::ffff:203.0.113.7"))
	assert.True(t, IsIPBlacklisted("::203.0.113.7"))
	assert.True(t, IsIPBlacklisted("::ffff:198.51.100.42"))
	assert.True(t, IsIPBlacklisted("::198.51.100.42"))
	assert.False(t, IsIPBlacklisted("::ffff:198.51.101.42"))
}

func TestIPBlacklistCanonicalizesIPv4MappedPrefixes(t *testing.T) {
	settings := GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})
	require.NoError(t, settings.UpdateConfigFromMap(map[string]string{
		"ip_blacklist": `["::ffff:192.0.2.0/120", "::192.0.3.0/120"]`,
	}))

	assert.True(t, IsIPBlacklisted("192.0.2.10"))
	assert.True(t, IsIPBlacklisted("::ffff:192.0.2.10"))
	assert.True(t, IsIPBlacklisted("192.0.3.10"))
	assert.True(t, IsIPBlacklisted("::192.0.3.10"))
	assert.False(t, IsIPBlacklisted("192.0.4.10"))
}

func TestIPBlacklistSnapshotCanUpdateWhileRequestsRead(t *testing.T) {
	settings := GetSecuritySettings()
	original := append([]string(nil), settings.IPBlacklist...)
	t.Cleanup(func() {
		value, _ := common.Marshal(original)
		_ = settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": string(value)})
	})

	var group sync.WaitGroup
	errors := make(chan error, 4)
	group.Add(2)
	go func() {
		defer group.Done()
		for _, value := range []string{
			`["203.0.113.7"]`,
			`["198.51.100.0/24"]`,
			`[]`,
			`["::ffff:192.0.2.0/120"]`,
		} {
			if err := settings.UpdateConfigFromMap(map[string]string{"ip_blacklist": value}); err != nil {
				errors <- err
			}
		}
	}()
	go func() {
		defer group.Done()
		for _, ip := range []string{"203.0.113.7", "198.51.100.4", "::ffff:192.0.2.4", "2001:db8::1"} {
			_ = IsIPBlacklisted(ip)
		}
	}()
	group.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
}

package setting

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseModelRequestRateLimitDuration(t *testing.T) {
	tests := []struct {
		value    string
		expected time.Duration
	}{
		{value: "10s", expected: 10 * time.Second},
		{value: "5m", expected: 5 * time.Minute},
		{value: "1h", expected: time.Hour},
	}

	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			duration, err := ParseModelRequestRateLimitDuration(test.value)

			require.NoError(t, err)
			assert.Equal(t, test.expected, duration)
		})
	}
}

func TestParseModelRequestRateLimitDurationRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "0s", "01m", "1.5m", "10d", "1h30m", "-1s"} {
		t.Run(value, func(t *testing.T) {
			_, err := ParseModelRequestRateLimitDuration(value)

			require.Error(t, err)
		})
	}
}

func TestResolveModelRequestRateLimitDuration(t *testing.T) {
	tests := []struct {
		name           string
		durationValue  string
		durationExists bool
		legacyMinutes  string
		expected       string
	}{
		{
			name:          "legacy minutes",
			legacyMinutes: "5",
			expected:      "5m",
		},
		{
			name:           "new duration has priority",
			durationValue:  "10s",
			durationExists: true,
			legacyMinutes:  "5",
			expected:       "10s",
		},
		{
			name:          "invalid legacy defaults",
			legacyMinutes: "0",
			expected:      "1m",
		},
		{
			name:          "legacy duration overflow defaults",
			legacyMinutes: "9223372036854775807",
			expected:      "1m",
		},
		{
			name:          "legacy duration capacity overflow defaults",
			legacyMinutes: "715827883",
			expected:      "1m",
		},
		{
			name:           "invalid new duration defaults instead of using legacy",
			durationValue:  "5d",
			durationExists: true,
			legacyMinutes:  "5",
			expected:       "1m",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := ResolveModelRequestRateLimitDuration(test.durationValue, test.durationExists, test.legacyMinutes)

			assert.Equal(t, test.expected, actual)
		})
	}
}

func TestModelRequestRateLimitDurationConfigTracksCanonicalOption(t *testing.T) {
	original := ModelRequestRateLimitDurationConfig()
	t.Cleanup(func() {
		require.NoError(t, SetResolvedModelRequestRateLimitDuration(original.Value, original.Canonical))
	})

	require.NoError(t, SetResolvedModelRequestRateLimitDuration("1m", false))
	assert.False(t, ModelRequestRateLimitDurationConfig().Canonical)

	require.NoError(t, UpdateModelRequestRateLimitDuration("10s"))
	config := ModelRequestRateLimitDurationConfig()
	assert.True(t, config.Canonical)
	assert.Equal(t, 10*time.Second, config.Window)
}

func TestModelRequestRateLimitDurationConfigSwitchesAtActivationTimestamp(t *testing.T) {
	original := ModelRequestRateLimitDurationConfig()
	originalNow := modelRequestRateLimitDurationNow
	t.Cleanup(func() {
		modelRequestRateLimitDurationNow = originalNow
		require.NoError(t, SetResolvedModelRequestRateLimitDuration(original.Value, original.Canonical))
	})

	now := time.Unix(1_700_000_000, 0)
	modelRequestRateLimitDurationNow = func() time.Time { return now }
	require.NoError(t, ConfigureModelRequestRateLimitDuration("10s", "5m", true, now.Add(time.Minute).Unix()))

	pending := ModelRequestRateLimitDurationConfig()
	assert.False(t, pending.Canonical)
	assert.Equal(t, "5m", pending.Value)

	now = now.Add(time.Minute)
	active := ModelRequestRateLimitDurationConfig()
	assert.True(t, active.Canonical)
	assert.Equal(t, "10s", active.Value)
}

func TestModelRequestRateLimitWindowReturnsConsistentSnapshot(t *testing.T) {
	original := ModelRequestRateLimitDurationConfig()
	t.Cleanup(func() {
		require.NoError(t, SetResolvedModelRequestRateLimitDuration(original.Value, original.Canonical))
	})

	values := []string{"10s", "1h"}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 1_000 {
			_ = UpdateModelRequestRateLimitDuration(values[i%len(values)])
		}
	}()

	for range 1_000 {
		duration, value := ModelRequestRateLimitWindow()
		expected, err := ParseModelRequestRateLimitDuration(value)

		require.NoError(t, err)
		assert.Equal(t, expected, duration)
	}
	<-done
}

func TestUpdateModelRequestRateLimitGroupByJSONStringKeepsOldValueOnError(t *testing.T) {
	original := ModelRequestRateLimitGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(original))
	})

	require.NoError(t, UpdateModelRequestRateLimitGroupByJSONString(`{"paid-users":[10,5]}`))

	err := UpdateModelRequestRateLimitGroupByJSONString(`{`)

	require.Error(t, err)
	total, success, found := GetGroupRateLimit("paid-users")
	require.True(t, found)
	assert.Equal(t, 10, total)
	assert.Equal(t, 5, success)
}

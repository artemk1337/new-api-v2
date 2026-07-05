package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

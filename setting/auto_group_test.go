package setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateAutoGroupsInvalidJSONPreservesCurrentValue(t *testing.T) {
	original := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(original))
	})
	require.NoError(t, UpdateAutoGroupsByJsonString(`["default","vip"]`))

	require.Error(t, UpdateAutoGroupsByJsonString(`["sale"`))

	assert.Equal(t, []string{"default", "vip"}, GetAutoGroups())
}

func TestGetAutoGroupsReturnsCopy(t *testing.T) {
	original := AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, UpdateAutoGroupsByJsonString(original))
	})
	require.NoError(t, UpdateAutoGroupsByJsonString(`["default"]`))

	groups := GetAutoGroups()
	groups[0] = "changed"

	assert.Equal(t, []string{"default"}, GetAutoGroups())
}

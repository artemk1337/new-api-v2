package model

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReplaceChannelGroupNamesWithIDs(t *testing.T) {
	truncateTables(t)

	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"vip","ratio":1,"selectable":true,"description":"vip"},
		{"id":3,"name":"svip","ratio":1,"selectable":false,"description":""}
	]`))

	channel := &Channel{
		Name:  "group-migration",
		Key:   "test-key",
		Group: "default,vip,custom",
	}
	require.NoError(t, DB.Create(channel).Error)

	require.NoError(t, ReplaceChannelGroupNamesWithIDs())

	var reloaded Channel
	require.NoError(t, DB.First(&reloaded, channel.Id).Error)
	assert.Equal(t, "1,2,custom", reloaded.Group)
}

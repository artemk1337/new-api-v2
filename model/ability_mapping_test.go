package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/require"
)

func TestGetEnabledChannelModelMappingsNormalizesNullMapping(t *testing.T) {
	truncateTables(t)
	const (
		group     = "default"
		modelName = "alias-model"
	)
	priority := int64(0)
	mapping := `{"alias-model":"provider-model"}`
	require.NoError(t, DB.Create(&Channel{Id: 1, Key: "key-1", Name: "unmapped", Status: common.ChannelStatusEnabled, ModelMapping: nil}).Error)
	require.NoError(t, DB.Create(&Channel{Id: 2, Key: "key-2", Name: "mapped", Status: common.ChannelStatusEnabled, ModelMapping: &mapping}).Error)
	for _, channelID := range []int{1, 2} {
		require.NoError(t, DB.Create(&Ability{
			Group:     ratio_setting.PricingGroupKey(group),
			Model:     modelName,
			ChannelId: channelID,
			Enabled:   true,
			Priority:  &priority,
		}).Error)
	}

	mappings, err := GetEnabledChannelModelMappings([]string{group}, modelName, "")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"{}", mapping}, mappings)
}

func TestGetEnabledChannelModelMappingsForCompactAbilities(t *testing.T) {
	truncateTables(t)
	const (
		group     = "default"
		baseModel = "alias-model"
	)
	priority := int64(0)
	mappingA := `{"alias-model":"provider-a"}`
	mappingB := `{"alias-model":"provider-b"}`
	for _, channel := range []*Channel{
		{Id: 1, Key: "key-1", Name: "first", Status: common.ChannelStatusEnabled, ModelMapping: &mappingA},
		{Id: 2, Key: "key-2", Name: "second", Status: common.ChannelStatusEnabled, ModelMapping: &mappingB},
	} {
		require.NoError(t, DB.Create(channel).Error)
		require.NoError(t, DB.Create(&Ability{
			Group:     ratio_setting.PricingGroupKey(group),
			Model:     baseModel + "-openai-compact",
			ChannelId: channel.Id,
			Enabled:   true,
			Priority:  &priority,
		}).Error)
	}

	mappings, err := GetEnabledChannelModelMappingsForModels(
		[]string{group},
		[]string{baseModel, baseModel + "-openai-compact"},
		"",
	)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{mappingA, mappingB}, mappings)
}

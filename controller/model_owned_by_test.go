package controller

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChannelOwnerNameUsesAdaptorChannelName(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		expected    string
	}{
		{
			name:        "openai",
			channelType: constant.ChannelTypeOpenAI,
			expected:    "openai",
		},
		{
			name:        "codex",
			channelType: constant.ChannelTypeCodex,
			expected:    "codex",
		},
		{
			name:        "openrouter",
			channelType: constant.ChannelTypeOpenRouter,
			expected:    "openrouter",
		},
		{
			name:        "azure fallback",
			channelType: constant.ChannelTypeAzure,
			expected:    "azure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, channelOwnerName(tt.channelType))
		})
	}
}

func TestBuildOpenAIModelOverridesOwnedBy(t *testing.T) {
	modelItem := buildOpenAIModel("gpt-5.4", map[string]string{"gpt-5.4": "openai"})
	require.Equal(t, "gpt-5.4", modelItem.Id)
	require.Equal(t, "openai", modelItem.OwnedBy)
}

func TestBuildOpenAIModelFallsBackToCustomForUnknownModels(t *testing.T) {
	modelItem := buildOpenAIModel("custom-test-model", nil)
	require.Equal(t, "custom-test-model", modelItem.Id)
	require.Equal(t, "custom", modelItem.OwnedBy)
}

func TestGetModelListGroupsUsesUserUsablePricingGroupsWhenTokenGroupIsEmpty(t *testing.T) {
	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalUsableGroups := setting.UserUsableGroups2JSONString()
	originalSpecialUsable := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsableGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(originalSpecialUsable))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP"}`))
	require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(`{}`))

	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "paid-users")

	groups, err := getModelListGroups(ctx)
	require.NoError(t, err)

	require.Equal(t, "paid-users", groups.userGroup)
	require.Empty(t, groups.tokenGroup)
	require.Equal(t, []string{"1", "2"}, groups.ownerGroups)
}

func TestGetModelListGroupsUsesExplicitTokenGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "2")

	groups, err := getModelListGroups(ctx)
	require.NoError(t, err)

	require.Equal(t, "default", groups.userGroup)
	require.Equal(t, "2", groups.tokenGroup)
	require.Equal(t, []string{"2"}, groups.ownerGroups)
}

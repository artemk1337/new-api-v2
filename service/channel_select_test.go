package service

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCheapestAvailableGroupChoosesLowestRatioWithChannel(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":0.8,"selectable":true},
		{"id":3,"name":"budget","ratio":0.5,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:       ctx,
		ModelName: "gpt-test",
	}
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		id, _ := strconv.Atoi(group)
		return &model.Channel{Id: id}, nil
	}

	channel, group, err := selectCheapestAvailableGroup(param, "default", fetch)
	budgetKey := ratio_setting.PricingGroupKey("budget")
	budgetID, _ := strconv.Atoi(budgetKey)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, budgetKey, group)
	assert.Equal(t, budgetID, channel.Id)
}

func TestSelectCheapestAvailableGroupSkipsGroupsWithoutModel(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"budget","ratio":0.5,"selectable":true}
	]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:       ctx,
		ModelName: "gpt-test",
	}
	budgetKey := ratio_setting.PricingGroupKey("budget")
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		if group == budgetKey {
			return nil, nil
		}
		return &model.Channel{Id: 1}, nil
	}

	channel, group, err := selectCheapestAvailableGroup(param, "default", fetch)
	defaultKey := ratio_setting.PricingGroupKey("default")

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, defaultKey, group)
	assert.Equal(t, 1, channel.Id)
}

func TestBuildAutoGroupSnapshotUsesConfiguredSubsetAndEffectiveUserRatio(t *testing.T) {
	restoreGroupSettings(t)
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true},
		{"id":3,"name":"budget","ratio":0.1,"selectable":true}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{
		"member":{"default":3,"vip":0.5,"budget":0.1}
	}`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, ModelName: "gpt-test", RequestPath: "/v1/chat/completions"}
	defaultGroup := ratio_setting.PricingGroupKey("default")
	vipGroup := ratio_setting.PricingGroupKey("vip")
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		require.Equal(t, 0, retry)
		return &model.Channel{Id: 1}, nil
	}

	groups, err := buildAutoGroupSnapshot(param, "member", []string{defaultGroup, vipGroup}, fetch)

	require.NoError(t, err)
	assert.Equal(t, []string{vipGroup, defaultGroup}, groups)
}

func TestBuildAutoGroupSnapshotDoesNotExpandBeyondAutoAllowlist(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true},
		{"id":3,"name":"budget","ratio":0.1,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["default","vip"]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, ModelName: "gpt-test"}
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		return &model.Channel{Id: 1}, nil
	}

	groups, err := buildAutoGroupSnapshot(param, "default", nil, fetch)

	require.NoError(t, err)
	assert.Equal(t, []string{
		ratio_setting.PricingGroupKey("default"),
		ratio_setting.PricingGroupKey("vip"),
	}, groups)
}

func TestBuildAutoGroupSnapshotDoesNotExpandInvalidConfiguredSubsetToAll(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true}
	]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, ModelName: "gpt-test"}
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		return &model.Channel{Id: 1}, nil
	}

	groups, err := buildAutoGroupSnapshot(param, "default", []string{"auto", "removed-group"}, fetch)

	require.NoError(t, err)
	assert.Empty(t, groups)
}

func TestBuildAutoGroupSnapshotSkipsFetchErrorWhenAnotherGroupIsAvailable(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true}
	]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{Ctx: ctx, ModelName: "gpt-test"}
	defaultGroup := ratio_setting.PricingGroupKey("default")
	vipGroup := ratio_setting.PricingGroupKey("vip")
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		if group == vipGroup {
			return nil, assert.AnError
		}
		return &model.Channel{Id: 1}, nil
	}

	groups, err := buildAutoGroupSnapshot(param, "default", nil, fetch)

	require.NoError(t, err)
	assert.Equal(t, []string{defaultGroup}, groups)
}

func TestSpecificChannelAutoGroupFetcherKeepsOnlySupportedGroups(t *testing.T) {
	const channelID = 42
	fetch := specificChannelAutoGroupFetcher(channelID, func(group string, modelName string, gotChannelID int) bool {
		require.Equal(t, "gpt-test", modelName)
		require.Equal(t, channelID, gotChannelID)
		return group == "premium"
	})

	channel, err := fetch("budget", "gpt-test", 0, "")
	require.NoError(t, err)
	assert.Nil(t, channel)

	channel, err = fetch("premium", "gpt-test", 0, "")
	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, channelID, channel.Id)
}

func TestSelectAutoSnapshotChannelUsesExactlyOneGroupPerAttempt(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	param := &RetryParam{
		Ctx:         ctx,
		ModelName:   "gpt-test",
		RequestPath: "/v1/chat/completions",
		Retry:       common.GetPointer(1),
	}
	calls := 0
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		calls++
		assert.Equal(t, "group-b", group)
		assert.Equal(t, "gpt-test", modelName)
		assert.Zero(t, retry)
		assert.Equal(t, "/v1/chat/completions", requestPath)
		return &model.Channel{Id: 22}, nil
	}

	channel, group, err := selectAutoSnapshotChannel(param, []string{"group-a", "group-b", "group-c"}, fetch)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, 22, channel.Id)
	assert.Equal(t, "group-b", group)
	assert.Equal(t, 1, calls)
}

func restoreGroupSettings(t *testing.T) {
	t.Helper()

	oldPricingGroups := ratio_setting.PricingGroups2JSONString()
	oldGroupSpecialUsableGroup := ratio_setting.GroupSpecialUsableGroup2JSONString()
	oldAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldPricingGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(oldGroupSpecialUsableGroup))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(oldAutoGroups))
	})
}

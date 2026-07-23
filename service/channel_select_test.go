package service

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
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

func restoreGroupSettings(t *testing.T) {
	t.Helper()

	oldPricingGroups := ratio_setting.PricingGroups2JSONString()
	oldGroupSpecialUsableGroup := ratio_setting.GroupSpecialUsableGroup2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldPricingGroups))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(oldGroupSpecialUsableGroup))
	})
}

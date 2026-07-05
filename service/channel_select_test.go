package service

import (
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectCheapestAvailableGroupChoosesLowestRatioWithChannel(t *testing.T) {
	restoreGroupSettings(t)
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"vip":0.8,"budget":0.5}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","vip":"VIP","budget":"Budget"}`))

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
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":1,"budget":0.5}`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{"default":"Default","budget":"Budget"}`))

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

	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldGroupSpecialUsableGroup := ratio_setting.GroupSpecialUsableGroup2JSONString()
	oldUserUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, ratio_setting.UpdateGroupSpecialUsableGroupByJSONString(oldGroupSpecialUsableGroup))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUserUsableGroups))
	})
}

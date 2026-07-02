package service

import (
	"net/http/httptest"
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
		return &model.Channel{Id: map[string]int{
			"1": 1,
			"2": 2,
			"3": 3,
		}[group]}, nil
	}

	channel, group, err := selectCheapestAvailableGroup(param, "default", fetch)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "3", group)
	assert.Equal(t, 3, channel.Id)
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
	fetch := func(group string, modelName string, retry int, requestPath string) (*model.Channel, error) {
		if group == "2" {
			return nil, nil
		}
		return &model.Channel{Id: 1}, nil
	}

	channel, group, err := selectCheapestAvailableGroup(param, "default", fetch)

	require.NoError(t, err)
	require.NotNil(t, channel)
	assert.Equal(t, "1", group)
	assert.Equal(t, 1, channel.Id)
}

func restoreGroupSettings(t *testing.T) {
	t.Helper()

	oldGroupRatio := ratio_setting.GroupRatio2JSONString()
	oldUserUsableGroups := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(oldGroupRatio))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(oldUserUsableGroups))
	})
}

package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetUserGroupsReturnsPricingGroupRefs(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "alice", Group: "paid-users"}).Error)

	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalAutoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(originalAutoGroups))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true,"description":"vip"}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["1"]`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)

	GetUserGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success    bool     `json:"success"`
		AutoGroups []string `json:"auto_groups"`
		Data       map[string]struct {
			Id   string `json:"id"`
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, "2", payload.Data["2"].Id)
	require.Equal(t, "Renamed VIP", payload.Data["2"].Name)
	require.Equal(t, "vip", payload.Data["2"].Desc)
	require.Equal(t, "auto", payload.Data["auto"].Id)
	require.Equal(t, []string{"1", "2"}, payload.AutoGroups)
}

func TestGetGroupsIncludesChannelStatusCounts(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Key: "one", Group: "1,2", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 2, Key: "two", Group: "2", Status: common.ChannelStatusAutoDisabled}).Error)
	require.NoError(t, db.Create(&model.Channel{Id: 3, Key: "three", Group: "1", Status: common.ChannelStatusManuallyDisabled}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	GetGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Data []struct {
			Id           int `json:"id"`
			ChannelStats struct {
				Total    int `json:"total"`
				Active   int `json:"active"`
				Inactive int `json:"inactive"`
			} `json:"channel_stats"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Len(t, payload.Data, 2)
	require.Equal(t, 2, payload.Data[0].ChannelStats.Total)
	require.Equal(t, 1, payload.Data[0].ChannelStats.Active)
	require.Equal(t, 1, payload.Data[0].ChannelStats.Inactive)
	require.Equal(t, 2, payload.Data[1].ChannelStats.Total)
	require.Equal(t, 1, payload.Data[1].ChannelStats.Active)
	require.Equal(t, 1, payload.Data[1].ChannelStats.Inactive)
}

func TestGetUserGroupsReturnsUserGroupReadError(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Migrator().DropTable(&model.User{}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/self/groups", nil)

	GetUserGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.False(t, payload.Success)
}

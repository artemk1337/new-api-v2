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
	originalUsable := setting.UserUsableGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(originalUsable))
	})

	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"default"},
		{"id":2,"name":"Renamed VIP","ratio":1.2,"selectable":true,"description":"vip"}
	]`))
	require.NoError(t, setting.UpdateUserUsableGroupsByJSONString(`{
		"2": "vip desc",
		"auto": "auto desc"
	}`))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Set("id", 1)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/user/groups", nil)

	GetUserGroups(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool `json:"success"`
		Data    map[string]struct {
			Id   string `json:"id"`
			Name string `json:"name"`
			Desc string `json:"desc"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Equal(t, "2", payload.Data["2"].Id)
	require.Equal(t, "Renamed VIP", payload.Data["2"].Name)
	require.Equal(t, "vip desc", payload.Data["2"].Desc)
	require.Equal(t, "auto", payload.Data["auto"].Id)
	require.Equal(t, "auto", payload.Data["auto"].Name)
	require.Equal(t, "auto desc", payload.Data["auto"].Desc)
}

package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetModelRequestNormalizesPlaygroundGroupName(t *testing.T) {
	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":1,"selectable":true}
	]`))

	req := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(`{"model":"gpt-test","group":"vip"}`))
	req.Header.Set("Content-Type", "application/json")
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = req

	modelRequest, shouldSelectChannel, err := getModelRequest(ctx)

	require.NoError(t, err)
	require.True(t, shouldSelectChannel)
	assert.Equal(t, "gpt-test", modelRequest.Model)
	assert.Equal(t, "2", modelRequest.Group)
	assert.Empty(t, common.GetContextKeyString(ctx, constant.ContextKeyTokenGroup))
}

func TestCanUsePlaygroundPricingGroupUsesUserGroupDomain(t *testing.T) {
	originalGroups := ratio_setting.PricingGroups2JSONString()
	originalSpecial := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.ReadAll()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Clear()
		ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.AddAll(originalSpecial)
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"locked","ratio":1,"selectable":true},
		{"id":3,"name":"vip","ratio":1,"selectable":true}
	]`))
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Clear()
	ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Set("paid-user", map[string]string{
		"+:vip": "VIP",
	})

	assert.True(t, canUsePlaygroundPricingGroup("paid-user", "", "paid-user", "3"))
	assert.False(t, canUsePlaygroundPricingGroup("paid-user", "2", "2", "3"))
	assert.True(t, canUsePlaygroundPricingGroup("paid-user", "2", "2", "2"))
}

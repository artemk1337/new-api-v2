package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPricingUsesSelectablePricingGroups(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	originalGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(originalGroups))
		model.InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true,"description":"Default"},
		{"id":7,"name":"sale","ratio":0.3,"selectable":true,"description":"Sale"},
		{"id":8,"name":"internal","ratio":1,"selectable":false,"description":"Internal"}
	]`))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Name: "pricing", Key: "key", Models: "sale-model,internal-model", Group: "7", Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&[]model.Ability{
		{Group: "7", Model: "sale-model", ChannelId: 1, Enabled: true},
		{Group: "8", Model: "internal-model", ChannelId: 1, Enabled: true},
	}).Error)
	model.InvalidatePricingCache()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing", nil)
	GetPricing(ctx)

	var payload struct {
		Success     bool              `json:"success"`
		Data        []model.Pricing   `json:"data"`
		UsableGroup map[string]string `json:"usable_group"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	assert.Equal(t, map[string]string{"1": "Default", "7": "Sale"}, payload.UsableGroup)
	require.Len(t, payload.Data, 1)
	assert.Equal(t, "sale-model", payload.Data[0].ModelName)
}

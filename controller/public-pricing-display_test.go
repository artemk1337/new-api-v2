package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestPublicPricingReturnsProviderListAndDefaultReference(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	groups, ratios, completions, prices := ratio_setting.PricingGroups2JSONString(), ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(ratios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(completions))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		model.InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":0.5,"selectable":true}]`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-display":2}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-display":3}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, db.Create(&model.Channel{Id: 1, Type: 1, Status: common.ChannelStatusEnabled}).Error)
	require.NoError(t, db.Create(&model.Ability{Model: "gpt-display", Group: "1", ChannelId: 1, Enabled: true}).Error)
	model.InvalidatePricingCache()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/pricing/v2", nil)

	GetPricingV2(ctx)

	require.Equal(t, http.StatusOK, recorder.Code)
	var payload struct {
		Success bool
		Data    []model.PublicPricingModel
		Vendors []model.PricingVendor
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 1)
	item := payload.Data[0]
	require.NotZero(t, item.VendorID)
	require.Equal(t, "OpenAI", item.VendorName)
	require.Equal(t, "OpenAI", item.VendorIcon)
	require.Contains(t, payload.Vendors, model.PricingVendor{ID: item.VendorID, Name: "OpenAI", Icon: "OpenAI"})
	require.Equal(t, &model.PublicBaseReference{InputUSD: 4, OutputUSD: 12, PriceUnit: "million_tokens"}, item.BaseReference)
	require.Equal(t, 2.0, *item.GroupPrices[0].InputUSD)
	require.Equal(t, 6.0, *item.GroupPrices[0].OutputUSD)
	require.Nil(t, item.OfficialPrice)
}

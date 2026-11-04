package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierPublicPricingSeparatesGroupContractsAndHidesCosts(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	original := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(original))
		model.InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1,"selectable":true},{"id":7,"name":"Kiro-Corp-1","ratio":2,"selectable":true},{"id":8,"name":"OpenAI-1","ratio":3,"selectable":true},{"id":9,"name":"hidden","ratio":9,"selectable":false}]`))
	supplier := model.Supplier{Code: "private-supplier", Name: "Private Supplier", Adapter: "manual", Enabled: true}
	require.NoError(t, model.CreateSupplier(&supplier))
	profile := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: "group-a", Name: "Private Profile", Enabled: true, USDPerCredit: 0.006}
	require.NoError(t, model.CreateSupplierPricingProfile(&profile))
	input, output, cache := 0.012, 0.06, 0.0012
	_, err := model.PublishSupplierProfileTariffs(profile.ID, profile.ConfigRevision, 0, []types.SupplierModelTariff{{ModelName: "mapped", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output, CacheReadUSD: &cache}})
	require.NoError(t, err)
	require.NoError(t, db.Create(&[]model.PricingGroupSupplierBinding{{GroupID: 7, SupplierID: supplier.ID, ProfileID: profile.ID}, {GroupID: 8, SupplierID: supplier.ID, ProfileID: profile.ID}, {GroupID: 9, SupplierID: supplier.ID, ProfileID: profile.ID}}).Error)
	mapping := `{"alias":"mapped"}`
	channel := model.Channel{Id: 11, Name: "private-channel", Key: "secret-key", SupplierID: &supplier.ID, SupplierProfileID: &profile.ID, ModelMapping: &mapping, Models: "alias", Group: "7,8,9", Status: common.ChannelStatusEnabled}
	require.NoError(t, db.Create(&channel).Error)
	require.NoError(t, db.Create(&[]model.Ability{{Model: "alias", Group: "7", ChannelId: 11, Enabled: true}, {Model: "alias", Group: "8", ChannelId: 11, Enabled: true}, {Model: "alias", Group: "9", ChannelId: 11, Enabled: true}}).Error)

	call := func(handler gin.HandlerFunc, path string) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, path, nil)
		handler(ctx)
		return recorder
	}
	response := call(GetPricingV2, "/api/pricing/v2")
	require.Equal(t, 200, response.Code)
	var payload struct {
		Success    bool
		Data       []model.PublicPricingModel
		GroupRatio map[string]float64 `json:"group_ratio"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Success)
	require.Len(t, payload.Data, 1)
	require.Len(t, payload.Data[0].GroupPrices, 2)
	prices := payload.Data[0].GroupPrices
	require.Equal(t, "alias", payload.Data[0].ModelName)
	require.InDelta(t, input*2, *prices[0].InputUSD, 1e-12)
	require.InDelta(t, output*2, *prices[0].OutputUSD, 1e-12)
	require.InDelta(t, cache*2, *prices[0].CacheReadUSD, 1e-12)
	require.InDelta(t, input*3, *prices[1].InputUSD, 1e-12)
	for _, secret := range []string{"private-supplier", "Private Supplier", "Private Profile", "secret-key", "supplier_id", "profile_id", "usd_per_credit", "expression", "hidden", "\"9\""} {
		require.NotContains(t, response.Body.String(), secret)
	}
	require.Equal(t, 1.0, payload.GroupRatio["7"])
	legacy := call(GetPricing, "/api/pricing")
	require.NotContains(t, legacy.Body.String(), "Kiro-Corp-1")
	require.NotContains(t, legacy.Body.String(), "OpenAI-1")
	// A missing bound model never borrows any global default price.
	require.NoError(t, db.Model(&model.SupplierModelPrice{}).Where("profile_id = ?", profile.ID).Delete(&model.SupplierModelPrice{}).Error)
	response = call(GetPricingV2, "/api/pricing/v2")
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &payload))
	require.True(t, payload.Data[0].GroupPrices[0].Unavailable)
}

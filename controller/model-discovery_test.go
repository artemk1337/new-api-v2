package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestListModelsUsesSupplierTariffsAndKeepsScope(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	groups := ratio_setting.PricingGroups2JSONString()
	prices := ratio_setting.ModelPrice2JSONString()
	autoGroups := setting.AutoGroups2JsonString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		require.NoError(t, setting.UpdateAutoGroupsByJsonString(autoGroups))
		model.InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":7,"name":"supplier-coding","ratio":2,"selectable":true},
		{"id":8,"name":"supplier-other","ratio":2,"selectable":true}
	]`))
	require.NoError(t, setting.UpdateAutoGroupsByJsonString(`["7","8"]`))
	// A global price must not make an unpriced supplier route discoverable.
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"zz-supplier-unpriced-global":5}`))
	supplier := model.Supplier{Code: "discovery", Name: "Private supplier", Adapter: "manual", Enabled: true}
	require.NoError(t, model.CreateSupplier(&supplier))
	input, output, zero := 2.0, 10.0, 0.0
	for _, item := range []struct {
		group int
		code  string
		name  string
	}{
		{7, "coding", "zz-supplier-only"}, {8, "other", "zz-other-supplier-only"},
	} {
		profile := model.SupplierPricingProfile{SupplierID: supplier.ID, Code: item.code, Name: item.code, Enabled: true, USDPerCredit: 1}
		require.NoError(t, model.CreateSupplierPricingProfile(&profile))
		_, err := model.PublishSupplierProfileTariffs(profile.ID, profile.ConfigRevision, 0, []types.SupplierModelTariff{
			{ModelName: item.name, BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output},
			{ModelName: "zz-supplier-free", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &zero, OutputUSD: &zero},
		})
		require.NoError(t, err)
		require.NoError(t, db.Create(&model.PricingGroupSupplierBinding{GroupID: item.group, SupplierID: supplier.ID, ProfileID: profile.ID}).Error)
		group := strconv.Itoa(item.group)
		mapping := `{"zz-supplier-alias":"` + item.name + `"}`
		channel := model.Channel{Type: 1, Name: item.code, Key: "fixture-only", Group: group,
			Models: item.name + ",zz-supplier-alias,zz-supplier-free,zz-supplier-unpriced-global", ModelMapping: &mapping,
			SupplierID: &supplier.ID, SupplierProfileID: &profile.ID, Status: common.ChannelStatusEnabled}
		require.NoError(t, db.Create(&channel).Error)
		for _, name := range []string{item.name, "zz-supplier-alias", "zz-supplier-free", "zz-supplier-unpriced-global"} {
			require.NoError(t, db.Create(&model.Ability{Group: group, Model: name, ChannelId: channel.Id, Enabled: true}).Error)
		}
	}
	request := func(group string, limits map[string]bool, candidates []string, mapping string) []string {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		common.SetContextKey(ctx, constant.ContextKeyTokenGroup, group)
		common.SetContextKey(ctx, constant.ContextKeyUserGroup, "default")
		if limits != nil {
			common.SetContextKey(ctx, constant.ContextKeyTokenModelLimitEnabled, true)
			common.SetContextKey(ctx, constant.ContextKeyTokenModelLimit, limits)
		}
		if candidates != nil {
			common.SetContextKey(ctx, constant.ContextKeyTokenAutoGroupCandidates, candidates)
		}
		common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, mapping)
		ListModels(ctx, constant.ChannelTypeOpenAI)
		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		ids := decodeListModelsResponse(t, recorder)
		names := make([]string, 0, len(ids))
		for name := range ids {
			names = append(names, name)
		}
		return names
	}
	t.Run("supplier price without global fallback", func(t *testing.T) {
		ids := request("7", nil, nil, "")
		require.ElementsMatch(t, []string{"zz-supplier-only", "zz-supplier-alias", "zz-supplier-free"}, ids)
	})
	t.Run("token allowlist", func(t *testing.T) {
		ids := request("7", map[string]bool{"zz-supplier-only": true, "zz-other-supplier-only": true, "zz-supplier-unpriced-global": true}, nil, "")
		require.Equal(t, []string{"zz-supplier-only"}, ids)
	})
	t.Run("auto candidate subset", func(t *testing.T) {
		ids := request("auto", nil, []string{"7"}, "")
		require.Contains(t, ids, "zz-supplier-only")
		require.NotContains(t, ids, "zz-other-supplier-only")
		require.NotContains(t, ids, "zz-supplier-unpriced-global")
	})
	t.Run("key and channel aliases", func(t *testing.T) {
		ids := request("7", nil, nil, `{"zz-key-alias":"zz-supplier-alias"}`)
		require.Contains(t, ids, "zz-key-alias")
	})
	t.Run("disabled supplier has no default fallback", func(t *testing.T) {
		require.NoError(t, db.Model(&model.Supplier{}).Where("id = ?", supplier.ID).Update("enabled", false).Error)
		require.Empty(t, request("7", nil, nil, ""))
	})
}

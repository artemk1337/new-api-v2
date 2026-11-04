package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func setupAutoAvailabilityPrices(t *testing.T) {
	t.Helper()
	groups := ratio_setting.PricingGroups2JSONString()
	prices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(groups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		require.NoError(t, model.DB.Where("group_id IN ?", []int{91, 92, 93}).Delete(&model.PricingGroupSupplierBinding{}).Error)
		for _, table := range []any{&model.SupplierModelPrice{}, &model.SupplierPricingRevision{}, &model.SupplierPricingState{}} {
			require.NoError(t, model.DB.Where("profile_id IN ?", []int{1501, 1502, 1503}).Delete(table).Error)
		}
		require.NoError(t, model.DB.Where("supplier_id = ?", 1501).Delete(&model.SupplierPricingProfile{}).Error)
		require.NoError(t, model.DB.Delete(&model.Supplier{}, "id = ?", 1501).Error)
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1},
		{"id":91,"name":"with-cache","ratio":2},
		{"id":92,"name":"without-cache","ratio":2},
		{"id":93,"name":"unpublished","ratio":2}
	]`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"shared-model":0.01}`))
	require.NoError(t, model.DB.Create(&model.Supplier{ID: 1501, Code: "auto-availability", Name: "Auto", Adapter: "manual", Enabled: true}).Error)
	for index := range 3 {
		profileID, groupID := 1501+index, 91+index
		require.NoError(t, model.DB.Create(&model.SupplierPricingProfile{ID: profileID, SupplierID: 1501, Code: string(rune('a' + index)), Name: "Profile", Enabled: true, USDPerCredit: 1, ConfigRevision: 1}).Error)
		require.NoError(t, model.DB.Create(&model.PricingGroupSupplierBinding{GroupID: groupID, SupplierID: 1501, ProfileID: profileID}).Error)
		if index == 2 {
			continue
		}
		tariff := types.SupplierModelTariff{ModelName: "shared-model", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2)}
		if index == 0 {
			tariff.CacheWrite1hUSD = supplierTestPrice(4)
		}
		data, err := common.Marshal(tariff)
		require.NoError(t, err)
		require.NoError(t, model.DB.Create(&model.SupplierPricingState{ProfileID: profileID, ActiveRevision: 1}).Error)
		require.NoError(t, model.DB.Create(&model.SupplierPricingRevision{ProfileID: profileID, Revision: 1, ConfigRevision: 1}).Error)
		require.NoError(t, model.DB.Create(&model.SupplierModelPrice{ProfileID: profileID, Revision: 1, ModelName: tariff.ModelName, Data: string(data)}).Error)
	}
}

func TestAutoQuoteSkipsUnpublishedSupplierWithoutDefaultFallback(t *testing.T) {
	setupAutoAvailabilityPrices(t)
	ctx, info := supplierTestContext(`{}`)
	info.UsingGroup = "auto"
	state, err := BuildAutoRouteState(ctx, info, []string{"93", "1"}, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Len(t, state.Candidates, 1)
	require.Equal(t, "1", state.InitialGroup)
	require.Equal(t, "1", info.UsingGroup)
	require.Nil(t, state.Candidates[0].PriceData.SupplierTariff)
	require.Equal(t, state.Candidates[0].EstimatedQuota, state.ReservedQuota)

	ctx, info = supplierTestContext(`{}`)
	info.UsingGroup = "93"
	_, err = ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.Error(t, err, "an explicitly selected unpublished profile must remain unavailable")
	require.True(t, IsRoutePriceUnavailable(err))
}

func TestAutoQuoteKeepsOnlyGroupsThatCanPriceRequestedCache(t *testing.T) {
	setupAutoAvailabilityPrices(t)
	ctx, info := supplierTestContext(`{"messages":[{"content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral","ttl":"1h"}}]}]}`)
	info.UsingGroup = "auto"
	state, err := BuildAutoRouteState(ctx, info, []string{"92", "91"}, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Len(t, state.Candidates, 1)
	require.Equal(t, "91", state.InitialGroup)
	require.Equal(t, 1501, info.PriceData.SupplierTariff.ProfileID)
	require.Equal(t, state.Candidates[0].EstimatedQuota, state.ReservedQuota)

	_, err = BuildAutoRouteState(ctx, info, []string{"92", "93"}, 100, &types.TokenCountMeta{})
	require.Error(t, err)
	require.True(t, IsRoutePriceUnavailable(err))
	require.Equal(t, "91", common.GetContextKeyString(ctx, constant.ContextKeyAutoGroup))
}

func TestAutoPerCallQuoteSkipsUnsupportedAndUnpublishedSupplier(t *testing.T) {
	for _, mode := range []string{types.SupplierBillingModeToken, types.SupplierBillingModeExpression} {
		t.Run(mode, func(t *testing.T) {
			setupAutoAvailabilityPrices(t)
			if mode == types.SupplierBillingModeExpression {
				tariff := types.SupplierModelTariff{ModelName: "shared-model", BillingMode: mode, PriceUnit: "million_tokens", Expression: "p * 1 + c * 2"}
				data, err := common.Marshal(tariff)
				require.NoError(t, err)
				require.NoError(t, model.DB.Model(&model.SupplierModelPrice{}).Where("profile_id = ? AND revision = ?", 1501, 1).Update("data", string(data)).Error)
			}
			ctx, info := supplierTestContext(`{}`)
			info.UsingGroup = "auto"
			state, err := BuildAutoPerCallRouteState(ctx, info, []string{"91", "93", "1"}, nil)
			require.NoError(t, err)
			require.Len(t, state.Candidates, 1)
			require.Equal(t, "1", state.InitialGroup)
			require.Equal(t, []string{"1"}, common.GetContextKeyStringSlice(ctx, constant.ContextKeyAutoGroupSnapshot))

			ctx, info = supplierTestContext(`{}`)
			info.UsingGroup = "91"
			_, err = ModelPriceHelperPerCall(ctx, info)
			require.Error(t, err, "an explicitly selected unsupported task tariff must remain unavailable")
			require.True(t, IsRoutePriceUnavailable(err))
		})
	}
}

func TestAutoQuoteDoesNotTreatMalformedCallerJSONAsUnavailableRoute(t *testing.T) {
	setupAutoAvailabilityPrices(t)
	for _, perCall := range []bool{false, true} {
		ctx, info := supplierTestContext(`{"messages":`)
		info.UsingGroup = "auto"
		var err error
		if perCall {
			_, err = BuildAutoPerCallRouteState(ctx, info, []string{"93", "1"}, nil)
		} else {
			_, err = BuildAutoRouteState(ctx, info, []string{"93", "1"}, 100, &types.TokenCountMeta{})
		}
		require.ErrorContains(t, err, "invalid pricing request")
		require.False(t, IsRoutePriceUnavailable(err))
		require.Empty(t, info.AutoRoute.Candidates)
		require.False(t, info.AttemptDispatched)
	}
}

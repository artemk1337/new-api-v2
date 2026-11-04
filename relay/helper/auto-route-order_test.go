package helper

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func preserveAutoOrderBillingConfig(t *testing.T) {
	t.Helper()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error { saved[key] = value; return nil }))
	quotaPerUnit := common.GetQuotaPerUnit()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		common.SetQuotaPerUnit(quotaPerUnit)
	})
}

func TestAutoRouteSelectsFrozenCheapestPairDespiteHigherRequestReserve(t *testing.T) {
	setupAutoAvailabilityPrices(t)
	preserveAutoOrderBillingConfig(t)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"shared-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"shared-model":"p * 3 + c * 0.01"}`,
	}))
	tariff := types.SupplierModelTariff{ModelName: "shared-model", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: supplierTestPrice(0.1), OutputUSD: supplierTestPrice(1.5)}
	data, err := common.Marshal(tariff)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.SupplierModelPrice{}).Where("profile_id = ?", 1501).Update("data", string(data)).Error)
	require.NoError(t, model.DB.Model(&model.SupplierPricingProfile{}).Where("id = ?", 1502).Update("enabled", false).Error)
	ctx, info := supplierTestContext(`{}`)
	info.UsingGroup = "auto"
	state, err := BuildAutoRouteState(ctx, info, []string{"93", "92", "91", "1"}, 1000, &types.TokenCountMeta{MaxTokens: 1})
	require.NoError(t, err)
	require.Len(t, state.Candidates, 2)
	require.Equal(t, "1", state.InitialGroup)
	require.Equal(t, "1", info.UsingGroup)
	require.Greater(t, state.Candidates[0].EstimatedQuota, state.Candidates[1].EstimatedQuota)
	require.Equal(t, state.Candidates[0].EstimatedQuota, state.ReservedQuota)
	require.Equal(t, "1", state.ReserveGroup)
	require.Equal(t, 1.0, state.Candidates[0].TieredSnapshot.GroupRatio)

	// Configuration changes cannot reorder a quote or replace its output rate.
	require.NoError(t, model.DB.Model(&model.SupplierPricingProfile{}).Where("id = ?", 1501).Update("enabled", false).Error)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"billing_setting.billing_expr": `{"shared-model":"p * 0.01 + c * 500"}`}))
	common.SetQuotaPerUnit(123)
	SortAutoRouteCandidates(state.Candidates)
	require.Equal(t, "1", state.Candidates[0].Group)
	require.True(t, info.ApplyAutoRouteCandidate("1"))
	require.Equal(t, "p * 3 + c * 0.01", info.TieredBillingSnapshot.ExprString)
	require.Nil(t, info.PriceData.SupplierTariff)
	require.True(t, info.ApplyAutoRouteCandidate("91"))
	require.Equal(t, 1.5, *info.PriceData.SupplierTariff.OutputUSD)
}

func TestAutoPairOrderUsesFrozenCustomQuotaPerUnitForLegacyPrices(t *testing.T) {
	setupAutoAvailabilityPrices(t)
	preserveAutoOrderBillingConfig(t)
	oldRatios := ratio_setting.ModelRatio2JSONString()
	oldCompletion := ratio_setting.CompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"shared-model":0.6}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"shared-model":0.25}`))
	tariff := types.SupplierModelTariff{ModelName: "shared-model", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: supplierTestPrice(0.4), OutputUSD: supplierTestPrice(0.1)}
	data, err := common.Marshal(tariff)
	require.NoError(t, err)
	require.NoError(t, model.DB.Model(&model.SupplierModelPrice{}).Where("profile_id = ?", 1501).Update("data", string(data)).Error)
	common.SetQuotaPerUnit(1_000_000)
	ctx, info := supplierTestContext(`{}`)
	info.UsingGroup = "auto"
	state, err := BuildAutoRouteState(ctx, info, []string{"91", "1"}, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, "1", state.InitialGroup)
	require.Equal(t, 1_000_000.0, info.PriceData.QuotaPerUnit)
	pair, ok := frozenRetailPairPrice(state.Candidates[0])
	require.True(t, ok)
	require.InDelta(t, 0.75, pair, 1e-12)
	common.SetQuotaPerUnit(500_000)
	SortAutoRouteCandidates(state.Candidates)
	require.Equal(t, "1", state.Candidates[0].Group)
}

func TestAutoPairOrderUsesCurrentLengthTierAndPreservesOtherUnitSlots(t *testing.T) {
	for _, length := range []int{100, 101} {
		first := "short-cheap"
		if length == 101 {
			first = "steady"
		}
		candidates := []relaycommon.AutoRouteCandidate{
			{Group: "short-cheap", EstimatedQuota: 100, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `len <= 100 ? p * 1 + c * 1 : p * 5 + c * 1`, GroupRatio: 1, EstimatedPromptTokens: length}},
			{Group: "opaque", EstimatedQuota: 200, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `p * p + c`, GroupRatio: 1, EstimatedPromptTokens: length}},
			{Group: "fixed", EstimatedQuota: 300, PriceData: types.PriceData{UsePrice: true, ModelPrice: 0.01}},
			{Group: "steady", EstimatedQuota: 1000, TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: `p * 3 + c * 0.01`, GroupRatio: 1, EstimatedPromptTokens: length}},
		}
		SortAutoRouteCandidates(candidates)
		require.Equal(t, first, candidates[0].Group)
		require.Equal(t, "opaque", candidates[1].Group)
		require.Equal(t, "fixed", candidates[2].Group)
		SortAutoRouteCandidates(candidates)
		require.Equal(t, first, candidates[0].Group)
	}
	_, known := frozenRetailPairPrice(relaycommon.AutoRouteCandidate{PriceData: types.PriceData{QuotaPerUnit: 500000}})
	require.False(t, known, "an absent pair must not turn into a free or default rate")
}

func TestAutoPairOrderDoesNotTreatUnknownSupplierOutputAsFree(t *testing.T) {
	candidate := relaycommon.AutoRouteCandidate{
		PriceData:      types.PriceData{SupplierTariff: &types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1)}},
		TieredSnapshot: &billingexpr.BillingSnapshot{ExprString: "p * 1 + c * 0", GroupRatio: 1},
	}
	_, known := frozenRetailPairPrice(candidate)
	require.False(t, known)
	candidate.PriceData.SupplierTariff.OutputUSD = supplierTestPrice(0)
	pair, known := frozenRetailPairPrice(candidate)
	require.True(t, known)
	require.Equal(t, 1.0, pair)
}

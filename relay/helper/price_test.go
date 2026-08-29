package helper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestModelPriceHelperTieredUsesPreloadedRequestInput(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-test-model":"param(\"stream\") == true ? tier(\"stream\", p * 3) : tier(\"base\", p * 2)"}`,
	}))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/channel/test/1", nil)
	req.Body = nil
	req.ContentLength = 0
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req
	ctx.Set("group", "default")

	info := &relaycommon.RelayInfo{
		OriginModelName: "tiered-test-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		RequestHeaders:  map[string]string{"Content-Type": "application/json"},
		BillingRequestInput: &billingexpr.RequestInput{
			Headers: map[string]string{"Content-Type": "application/json"},
			Body:    []byte(`{"stream":true}`),
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Equal(t, 1500, priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "stream", info.TieredBillingSnapshot.EstimatedTier)
	require.Equal(t, billing_setting.BillingModeTieredExpr, info.TieredBillingSnapshot.BillingMode)
	require.Equal(t, common.QuotaPerUnit, info.TieredBillingSnapshot.QuotaPerUnit)
}

func TestModelPriceHelperTieredReservesReasoningAsMaximumCompletion(t *testing.T) {
	gin.SetMode(gin.TestMode)

	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"reasoning-test-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"reasoning-test-model":"tier(\"base\", p * 1 + c * 10 + rt * 2)"}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{OriginModelName: "reasoning-test-model"}

	priceData, err := modelPriceHelperTiered(
		ctx,
		info,
		100,
		&types.TokenCountMeta{MaxTokens: 10},
		types.GroupRatioInfo{GroupRatio: 1},
	)

	require.NoError(t, err)
	// Reserve the higher possible output rate: (100 + 10*10) / 1M * quota.
	require.Equal(t, billingexpr.QuotaRound(200.0/1_000_000*common.QuotaPerUnit), priceData.QuotaToPreConsume)
	require.NotNil(t, info.TieredBillingSnapshot)
}

func TestHasModelBillingConfigRequiresPositiveBasePrice(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	oldRatio := ratio_setting.ModelRatio2JSONString()
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio))
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})

	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"zero-price":0,"positive-price":0.01}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"zero-ratio":0,"positive-ratio":2}`))
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"tiered-price":"tiered_expr","invalid-tiered":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"tiered-price":"tier(\"base\", p * 1)","invalid-tiered":"not valid +++"}`,
	}))

	require.False(t, HasModelBillingConfig("zero-price"))
	require.True(t, HasModelBillingConfig("positive-price"))
	require.False(t, HasModelBillingConfig("zero-ratio"))
	require.True(t, HasModelBillingConfig("positive-ratio"))
	require.True(t, HasModelBillingConfig("tiered-price"))
	require.False(t, HasModelBillingConfig("invalid-tiered"))
}

func TestModelPriceHelperUsesMappedTargetBillingConfig(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"alias-model":9,"provider-model":0.5}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-model",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "provider-model",
			IsModelMapped:     true,
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, priceData.UsePrice)
	require.Equal(t, 0.5, priceData.ModelPrice)
	require.Equal(t, "alias-model", info.OriginModelName)
	require.Equal(t, "provider-model", info.UpstreamModelName)
}

func TestModelPriceHelperUsesMappingResolvedBeforeChannelMeta(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"provider-model":0.5}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("model_mapping", `{"alias-model":"provider-model"}`)
	modelName, isMapped, err := ResolveModelMapping(ctx.GetString("model_mapping"), "alias-model")
	require.NoError(t, err)
	require.True(t, isMapped)

	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName:  "alias-model",
		BillingModelName: modelName,
		UserGroup:        "default",
		UsingGroup:       "default",
	}

	priceData, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Nil(t, info.ChannelMeta)
	require.True(t, priceData.UsePrice)
	require.Equal(t, 0.5, priceData.ModelPrice)
	require.Equal(t, "alias-model", info.OriginModelName)
}

func TestResolveModelMappingTreatsEmptyAndIdentityValuesAsUnmapped(t *testing.T) {
	target, mapped, err := ResolveModelMapping(`{"alias-model":""}`, "alias-model")
	require.NoError(t, err)
	require.False(t, mapped)
	require.Equal(t, "alias-model", target)

	target, mapped, err = ResolveModelMapping(`{"alias-model":"alias-model"}`, "alias-model")
	require.NoError(t, err)
	require.False(t, mapped)
	require.Equal(t, "alias-model", target)
}

func TestResolveRelayModelMappingUsesCompactModelBase(t *testing.T) {
	target, mapped, err := ResolveRelayModelMapping(
		`{"alias-model":"provider-model"}`,
		"alias-model"+ratio_setting.CompactModelSuffix,
		relayconstant.RelayModeResponsesCompact,
	)
	require.NoError(t, err)
	require.True(t, mapped)
	require.Equal(t, "provider-model"+ratio_setting.CompactModelSuffix, target)
}

func TestModelPriceHelperUsesCompactMappedTargetWildcardPricing(t *testing.T) {
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"*-openai-compact":0.5}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("model_mapping", `{"alias-model":"provider-model"}`)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses/compact", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-model" + ratio_setting.CompactModelSuffix,
		RelayMode:       relayconstant.RelayModeResponsesCompact,
		UserGroup:       "default",
		UsingGroup:      "default",
	}
	require.NoError(t, ModelMappedHelper(ctx, info, nil))
	require.Equal(t, "provider-model", info.UpstreamModelName)
	require.Equal(t, "provider-model"+ratio_setting.CompactModelSuffix, info.BillingModelName)

	priceData, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, priceData.UsePrice)
	require.Equal(t, 0.5, priceData.ModelPrice)
}

func TestResolveCandidateModelMappingRejectsConflictingTargets(t *testing.T) {
	_, _, err := ResolveCandidateModelMapping([]string{
		`{"alias-model":"provider-a"}`,
		`{"alias-model":"provider-b"}`,
	}, "alias-model", relayconstant.RelayModeChatCompletions)
	require.EqualError(t, err, "model_mapping_has_conflicting_targets")
}

func TestResolveCandidateModelMappingUsesSharedTarget(t *testing.T) {
	target, mapped, err := ResolveCandidateModelMapping([]string{
		`{"alias-model":"provider-model"}`,
		`{"alias-model":"provider-model"}`,
	}, "alias-model", relayconstant.RelayModeChatCompletions)
	require.NoError(t, err)
	require.True(t, mapped)
	require.Equal(t, "provider-model", target)
}

func TestValidateModelMappingCompositionRejectsCrossLayerCycle(t *testing.T) {
	err := ValidateModelMappingComposition(
		`{"client-alias":"provider-model"}`,
		`{"provider-model":"client-alias"}`,
		"client-alias",
		relayconstant.RelayModeChatCompletions,
	)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestValidateModelMappingCandidatesRejectsCrossLayerCycleBeforePreConsume(t *testing.T) {
	err := ValidateModelMappingCandidates(
		`{"client-alias":"provider-model"}`,
		"client-alias",
		relayconstant.RelayModeChatCompletions,
		[]string{`{"provider-model":"client-alias"}`},
	)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestModelMappedHelperRejectsCrossLayerCycle(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, `{"client-alias":"provider-model"}`)
	common.SetContextKey(ctx, constant.ContextKeyRequestedModel, "client-alias")
	ctx.Set("model_mapping", `{"provider-model":"client-alias"}`)

	err := ModelMappedHelper(ctx, &relaycommon.RelayInfo{OriginModelName: "provider-model"}, nil)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestValidateModelMappingCandidatesRejectsCompactCrossLayerCycleBeforePreConsume(t *testing.T) {
	err := ValidateModelMappingCandidates(
		`{"client-alias":"provider-model"}`,
		ratio_setting.WithCompactModelSuffix("client-alias"),
		relayconstant.RelayModeResponsesCompact,
		[]string{`{"provider-model":"client-alias"}`},
	)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestModelMappedHelperRejectsCompactCrossLayerCycle(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyTokenModelMapping, `{"client-alias":"provider-model"}`)
	common.SetContextKey(ctx, constant.ContextKeyRequestedModel, ratio_setting.WithCompactModelSuffix("client-alias"))
	ctx.Set("model_mapping", `{"provider-model":"client-alias"}`)

	err := ModelMappedHelper(ctx, &relaycommon.RelayInfo{
		OriginModelName: ratio_setting.WithCompactModelSuffix("provider-model"),
		RelayMode:       relayconstant.RelayModeResponsesCompact,
	}, nil)
	require.EqualError(t, err, "model_mapping_contains_cycle")
}

func TestModelPriceHelperUsesMappedTargetTieredExpression(t *testing.T) {
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"provider-tiered":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"provider-tiered":"tier(\"base\", p * 2)"}`,
	}))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "alias-tiered",
		UserGroup:       "default",
		UsingGroup:      "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "provider-tiered",
			IsModelMapped:     true,
		},
	}

	priceData, err := ModelPriceHelper(ctx, info, 100, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.Zero(t, priceData.ModelPrice)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, "alias-tiered", info.TieredBillingSnapshot.ModelName)
	require.Equal(t, `tier("base", p * 2)`, info.TieredBillingSnapshot.ExprString)
}

func TestBuildAutoRouteStateReservesMostExpensiveEffectiveGroup(t *testing.T) {
	oldPricingGroups := ratio_setting.PricingGroups2JSONString()
	oldGroupGroupRatio := ratio_setting.GroupGroupRatio2JSONString()
	oldModelRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldPricingGroups))
		require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(oldGroupGroupRatio))
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldModelRatio))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true}
	]`))
	require.NoError(t, ratio_setting.UpdateGroupGroupRatioByJSONString(`{
		"member":{"default":3,"vip":0.5}
	}`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"auto-route-model":2}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "auto-route-model",
		UserGroup:       "member",
		UsingGroup:      "auto",
	}
	vipGroup := ratio_setting.PricingGroupKey("vip")
	defaultGroup := ratio_setting.PricingGroupKey("default")

	state, err := BuildAutoRouteState(ctx, info, []string{vipGroup, defaultGroup}, 100, &types.TokenCountMeta{MaxTokens: 4096})

	require.NoError(t, err)
	require.Len(t, state.Candidates, 2)
	baseQuota := int(float64(common.Max(100, common.PreConsumedQuota)+4096) * 2)
	require.Equal(t, baseQuota/2, state.Candidates[0].EstimatedQuota)
	require.Equal(t, baseQuota*3, state.Candidates[1].EstimatedQuota)
	require.Equal(t, defaultGroup, state.ReserveGroup)
	require.Equal(t, baseQuota*3, state.ReservedQuota)
	require.True(t, state.HasTokenReserveEstimate)
	require.Equal(t, common.PreConsumedQuota, state.EstimatedInputTokens)
	require.Equal(t, 4096, state.MaxOutputTokens)
	require.Equal(t, vipGroup, state.InitialGroup)
	require.Equal(t, vipGroup, info.UsingGroup)
	require.Equal(t, 0.5, info.PriceData.GroupRatioInfo.GroupRatio)
}

func TestBuildAutoRouteStateFreezesTieredSnapshotPerGroup(t *testing.T) {
	savedConfig := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		savedConfig[key] = value
		return nil
	}))
	oldPricingGroups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(savedConfig))
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldPricingGroups))
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"auto-tiered-model":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"auto-tiered-model":"tier(\"base\", p * 2)"}`,
	}))
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true}
	]`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	info := &relaycommon.RelayInfo{
		OriginModelName: "auto-tiered-model",
		UserGroup:       "member",
		UsingGroup:      "auto",
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{}`),
		},
	}
	defaultGroup := ratio_setting.PricingGroupKey("default")
	vipGroup := ratio_setting.PricingGroupKey("vip")

	state, err := BuildAutoRouteState(ctx, info, []string{defaultGroup, vipGroup}, 1000, &types.TokenCountMeta{})

	require.NoError(t, err)
	require.Len(t, state.Candidates, 2)
	require.NotNil(t, state.Candidates[0].TieredSnapshot)
	require.NotNil(t, state.Candidates[1].TieredSnapshot)
	require.Equal(t, 1.0, state.Candidates[0].TieredSnapshot.GroupRatio)
	require.Equal(t, 2.0, state.Candidates[1].TieredSnapshot.GroupRatio)
	require.Equal(t, state.Candidates[1].EstimatedQuota, state.ReservedQuota)
	require.Equal(t, vipGroup, state.ReserveGroup)
	require.Equal(t, state.Candidates[0].TieredSnapshot, info.TieredBillingSnapshot)
}

func TestBuildAutoPerCallRouteStateReservesMaximumAndKeepsCurrentAttempt(t *testing.T) {
	oldPricingGroups := ratio_setting.PricingGroups2JSONString()
	oldModelPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldPricingGroups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldModelPrice))
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[
		{"id":1,"name":"default","ratio":1,"selectable":true},
		{"id":2,"name":"vip","ratio":2,"selectable":true}
	]`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"task-auto-model":0.01}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/videos", nil)
	defaultGroup := ratio_setting.PricingGroupKey("default")
	vipGroup := ratio_setting.PricingGroupKey("vip")
	info := &relaycommon.RelayInfo{
		OriginModelName: "task-auto-model",
		UserGroup:       "default",
		UsingGroup:      vipGroup,
		AutoRoute: relaycommon.AutoRouteState{
			FailedGroups: []relaycommon.AutoFailedGroup{{Group: defaultGroup}},
		},
	}

	state, err := BuildAutoPerCallRouteState(ctx, info, []string{defaultGroup, vipGroup}, map[string]float64{"seconds": 3})

	require.NoError(t, err)
	require.Len(t, state.Candidates, 2)
	require.Equal(t, defaultGroup, state.InitialGroup)
	require.Equal(t, vipGroup, state.UsedGroup)
	require.Equal(t, vipGroup, info.UsingGroup)
	require.Equal(t, state.Candidates[1].EstimatedQuota, state.ReservedQuota)
	require.Equal(t, state.Candidates[0].EstimatedQuota*2, state.Candidates[1].EstimatedQuota)
	require.Equal(t, 3.0, info.PriceData.OtherRatios["seconds"])
	require.Equal(t, []relaycommon.AutoFailedGroup{{Group: defaultGroup}}, state.FailedGroups)
}

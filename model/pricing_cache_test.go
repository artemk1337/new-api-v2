package model

import (
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPricingCacheConcurrentInvalidation(t *testing.T) {
	truncateTables(t)
	InvalidatePricingCache()
	t.Cleanup(InvalidatePricingCache)

	require.NotNil(t, GetPricing())

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)

	go func() {
		defer workers.Done()
		<-start
		for range 50 {
			_ = GetPricing()
			_ = GetVendors()
			_ = GetSupportedEndpointMap()
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range 50 {
			InvalidatePricingCache()
			_ = GetPricing()
		}
	}()

	close(start)
	workers.Wait()
}

func TestBuildPricingModelMapPreservesRuleMatching(t *testing.T) {
	models := []Model{
		{ModelName: "exact-model", NameRule: NameRuleExact},
		{ModelName: "gpt", NameRule: NameRulePrefix},
		{ModelName: "-latest", NameRule: NameRuleSuffix},
		{ModelName: "special", NameRule: NameRuleContains},
	}

	modelMap := buildPricingModelMap(models, []string{
		"exact-model",
		"gpt-4",
		"claude-latest",
		"my-special-model",
		"gpt-special-latest",
	})

	require.Same(t, &models[0], modelMap["exact-model"])
	require.Same(t, &models[1], modelMap["gpt-4"])
	require.Same(t, &models[2], modelMap["claude-latest"])
	require.Same(t, &models[3], modelMap["my-special-model"])
	// Prefix rules are evaluated first, matching the previous updatePricing order.
	require.Same(t, &models[1], modelMap["gpt-special-latest"])
}

func TestGetPricingIncludesAliasesWithMappedTargetPricing(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Option{}))
	require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
		"ModelPrice":           {Set: map[string]any{"provider-model": 0.5}},
		"ModelRatio":           {Set: map[string]any{"provider-ratio": 2}},
		"CompletionRatio":      {Set: map[string]any{"provider-ratio": 3}},
		"CacheRatio":           {Set: map[string]any{"provider-ratio": 4}},
		"CreateCacheRatio":     {Set: map[string]any{"provider-ratio": 5}},
		"ImageRatio":           {Set: map[string]any{"provider-ratio": 6}},
		"AudioRatio":           {Set: map[string]any{"provider-ratio": 7}},
		"AudioCompletionRatio": {Set: map[string]any{"provider-ratio": 8}},
	}))
	t.Cleanup(func() {
		require.NoError(t, ApplyJSONOptionPatches(map[string]JSONObjectPatch{
			"ModelPrice":           {Delete: []string{"provider-model"}},
			"ModelRatio":           {Delete: []string{"provider-ratio"}},
			"CompletionRatio":      {Delete: []string{"provider-ratio"}},
			"CacheRatio":           {Delete: []string{"provider-ratio"}},
			"CreateCacheRatio":     {Delete: []string{"provider-ratio"}},
			"ImageRatio":           {Delete: []string{"provider-ratio"}},
			"AudioRatio":           {Delete: []string{"provider-ratio"}},
			"AudioCompletionRatio": {Delete: []string{"provider-ratio"}},
		}))
		InvalidatePricingCache()
	})
	InvalidatePricingCache()

	mapping := `{"cursor-model": "provider-model", "cursor-model-duplicate": "provider-model", "cursor-ratio": "provider-ratio"}`
	channel := &Channel{
		Name:         "mapped-pricing",
		Key:          "test-key",
		Models:       "provider-model,cursor-model,cursor-model-duplicate,provider-ratio,cursor-ratio",
		Group:        "default",
		Status:       common.ChannelStatusEnabled,
		ModelMapping: &mapping,
	}
	require.NoError(t, channel.Insert())
	require.NoError(t, DB.Model(&Ability{}).
		Where("channel_id = ? AND model = ?", channel.Id, "provider-model").
		Update("group", "target-group").Error)
	require.NoError(t, DB.Model(&Ability{}).
		Where("channel_id = ? AND model IN ?", channel.Id, []string{"cursor-model", "cursor-model-duplicate"}).
		Update("group", "alias-group").Error)
	InvalidatePricingCache()

	pricingByModel := make(map[string]Pricing)
	for _, pricing := range GetPricing() {
		pricingByModel[pricing.ModelName] = pricing
	}

	target, ok := pricingByModel["provider-model"]
	require.True(t, ok)
	require.Equal(t, 0.5, target.ModelPrice)
	for _, alias := range []string{"cursor-model", "cursor-model-duplicate"} {
		pricing, ok := pricingByModel[alias]
		require.True(t, ok)
		assert.Equal(t, target.ModelPrice, pricing.ModelPrice)
		assert.Equal(t, target.QuotaType, pricing.QuotaType)
		assert.Equal(t, []string{"alias-group"}, pricing.EnableGroup)
	}
	ratioTarget, ok := pricingByModel["provider-ratio"]
	require.True(t, ok)
	require.Equal(t, 2.0, ratioTarget.ModelRatio)
	ratioAlias, ok := pricingByModel["cursor-ratio"]
	require.True(t, ok)
	assert.Equal(t, ratioTarget.ModelRatio, ratioAlias.ModelRatio)
	assert.Equal(t, ratioTarget.QuotaType, ratioAlias.QuotaType)
	assert.Equal(t, ratioTarget.CompletionRatio, ratioAlias.CompletionRatio)
	require.NotNil(t, ratioTarget.CacheRatio)
	require.NotNil(t, ratioAlias.CacheRatio)
	assert.Equal(t, *ratioTarget.CacheRatio, *ratioAlias.CacheRatio)
	require.NotNil(t, ratioTarget.CreateCacheRatio)
	require.NotNil(t, ratioAlias.CreateCacheRatio)
	assert.Equal(t, *ratioTarget.CreateCacheRatio, *ratioAlias.CreateCacheRatio)
	require.NotNil(t, ratioTarget.ImageRatio)
	require.NotNil(t, ratioAlias.ImageRatio)
	assert.Equal(t, *ratioTarget.ImageRatio, *ratioAlias.ImageRatio)
	require.NotNil(t, ratioTarget.AudioRatio)
	require.NotNil(t, ratioAlias.AudioRatio)
	assert.Equal(t, *ratioTarget.AudioRatio, *ratioAlias.AudioRatio)
	require.NotNil(t, ratioTarget.AudioCompletionRatio)
	require.NotNil(t, ratioAlias.AudioCompletionRatio)
	assert.Equal(t, *ratioTarget.AudioCompletionRatio, *ratioAlias.AudioCompletionRatio)
}

func TestResolveChannelModelMappingTarget(t *testing.T) {
	mapping := `{"alias": "intermediate", "intermediate": "provider-model", "cycle-a": "cycle-b", "cycle-b": "cycle-a", "identity": "identity"}`

	require.Equal(t, "provider-model", resolveChannelModelMappingTarget("alias", &mapping))
	require.Empty(t, resolveChannelModelMappingTarget("cycle-a", &mapping))
	require.Empty(t, resolveChannelModelMappingTarget("identity", &mapping))
	emptyMapping := `{"alias": ""}`
	target, mapped := resolveChannelModelPricingTarget("alias", &emptyMapping)
	require.Empty(t, target)
	require.False(t, mapped)
}

func TestBuildModelPricingSourcesCollectsMultipleChannelTargets(t *testing.T) {
	firstMapping := `{"alias": "provider-b"}`
	secondMapping := `{"alias": "provider-a"}`

	sources, hasMapping := buildModelPricingSources([]AbilityWithChannel{
		{Ability: Ability{Model: "alias"}, ChannelModelMapping: &firstMapping},
		{Ability: Ability{Model: "alias"}, ChannelModelMapping: &secondMapping},
	})

	assert.ElementsMatch(t, []string{"provider-a", "provider-b"}, sources["alias"])
	assert.True(t, hasMapping["alias"])
}

func TestResolveModelPricingSourcePrefersTargetAndHidesConflicts(t *testing.T) {
	original := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(original))
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"alias":9,"provider-a":1,"provider-b":2}`))

	require.Equal(t, "provider-a", resolveModelPricingSource("alias", []string{"provider-a"}, true))
	require.Empty(t, resolveModelPricingSource("alias", []string{"provider-a", "provider-b"}, true))
	require.Empty(t, resolveModelPricingSource("alias", []string{"missing-provider"}, true))
}

func TestGetPricingInheritsMappedTargetTieredExpression(t *testing.T) {
	truncateTables(t)
	saved := map[string]string{}
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		saved[key] = value
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		InvalidatePricingCache()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"billing_setting.billing_mode": `{"provider-tiered":"tiered_expr"}`,
		"billing_setting.billing_expr": `{"provider-tiered":"tier(\"base\", p * 2)"}`,
	}))

	mapping := `{"alias-tiered":"provider-tiered"}`
	channel := &Channel{
		Name:         "mapped-tiered-pricing",
		Key:          "test-key",
		Models:       "provider-tiered,alias-tiered",
		Group:        "default",
		Status:       common.ChannelStatusEnabled,
		ModelMapping: &mapping,
	}
	require.NoError(t, channel.Insert())

	pricingByModel := make(map[string]Pricing)
	for _, pricing := range GetPricing() {
		pricingByModel[pricing.ModelName] = pricing
	}
	target, ok := pricingByModel["provider-tiered"]
	require.True(t, ok)
	alias, ok := pricingByModel["alias-tiered"]
	require.True(t, ok)
	assert.Equal(t, target.BillingMode, alias.BillingMode)
	assert.Equal(t, target.BillingExpr, alias.BillingExpr)
}

func TestInitDefaultVendorMappingCreatesVendorOnce(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Model{}, &Vendor{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM vendors")
		DB.Exec("DELETE FROM models")
	})

	metaMap := make(map[string]*Model)
	vendorMap := make(map[int]*Vendor)
	vendorNameMap := make(map[string]int)
	abilities := []AbilityWithChannel{
		{Ability: Ability{Model: "gpt-4"}},
		{Ability: Ability{Model: "gpt-4-mini"}},
	}

	initDefaultVendorMapping(metaMap, vendorMap, vendorNameMap, abilities)

	require.Len(t, vendorMap, 1)
	require.Len(t, vendorNameMap, 1)
	require.Equal(t, vendorNameMap["OpenAI"], metaMap["gpt-4"].VendorID)
	require.Equal(t, metaMap["gpt-4"].VendorID, metaMap["gpt-4-mini"].VendorID)
}

func TestInitDefaultVendorMappingAssignsExistingUnassignedMidjourneyModel(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Model{}, &Vendor{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM vendors")
		DB.Exec("DELETE FROM models")
	})

	metaMap := map[string]*Model{
		"mj_imagine": {ModelName: "mj_imagine", VendorID: 0},
	}
	vendorMap := make(map[int]*Vendor)
	vendorNameMap := make(map[string]int)

	initDefaultVendorMapping(metaMap, vendorMap, vendorNameMap, []AbilityWithChannel{
		{Ability: Ability{Model: "mj_imagine"}},
	})

	vendorID := metaMap["mj_imagine"].VendorID
	require.NotZero(t, vendorID)
	require.Equal(t, vendorNameMap["Midjourney"], vendorID)
	require.Equal(t, "Midjourney", vendorMap[vendorID].Icon)
}

func TestInitDefaultVendorMappingAssignsVeoToGoogle(t *testing.T) {
	truncateTables(t)
	require.NoError(t, DB.AutoMigrate(&Model{}, &Vendor{}))
	t.Cleanup(func() {
		DB.Exec("DELETE FROM vendors")
		DB.Exec("DELETE FROM models")
	})

	metaMap := map[string]*Model{
		"veo_3_1-fast": {ModelName: "veo_3_1-fast"},
	}
	vendorMap := make(map[int]*Vendor)
	vendorNameMap := make(map[string]int)

	initDefaultVendorMapping(metaMap, vendorMap, vendorNameMap, []AbilityWithChannel{
		{Ability: Ability{Model: "veo_3_1-fast"}},
	})

	vendorID := metaMap["veo_3_1-fast"].VendorID
	require.NotZero(t, vendorID)
	require.Equal(t, vendorNameMap["Google"], vendorID)
	require.Equal(t, "Gemini.Color", vendorMap[vendorID].Icon)
}

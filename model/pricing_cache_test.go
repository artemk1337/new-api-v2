package model

import (
	"sync"
	"testing"

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

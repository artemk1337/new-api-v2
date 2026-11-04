package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func preservePublicDisplayPrices(t *testing.T) {
	t.Helper()
	ratios, completions, prices := ratio_setting.ModelRatio2JSONString(), ratio_setting.CompletionRatio2JSONString(), ratio_setting.ModelPrice2JSONString()
	billing := make(map[string]string)
	for field, value := range map[string]any{
		billing_setting.BillingModeField: billing_setting.GetBillingModeCopy(),
		billing_setting.BillingExprField: billing_setting.GetBillingExprCopy(),
	} {
		data, err := common.Marshal(value)
		require.NoError(t, err)
		billing[field] = string(data)
	}
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(ratios))
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(completions))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(prices))
		require.NoError(t, billing_setting.UpdateFromMap(billing))
	})
}

func TestPublicPricingCatalogRestoresProviderMetadataWithoutProcurement(t *testing.T) {
	profile := supplierPricingTestDB(t)
	require.NoError(t, DB.AutoMigrate(&Channel{}, &Ability{}, &Model{}, &Vendor{}))
	preservePublicDisplayPrices(t)
	groups := ratio_setting.PricingGroups2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(groups)) })
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1},{"id":2,"name":"Retail","ratio":2,"selectable":true}]`))
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"gpt-display":2,"claude-display":3,"gpt-image-1":4}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"gpt-display":4,"claude-display":5,"gpt-image-1":2}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
		billing_setting.BillingModeField: `{"gpt-display":"tiered_expr"}`,
		billing_setting.BillingExprField: `{"gpt-display":"len<=100 ? tier(\"short\",p*50+c*100) : tier(\"long\",p*100+c*200)"}`,
	}))
	require.NoError(t, DB.Create(&[]Vendor{
		{Id: 10, Name: "OpenAI", Icon: "OpenAI", Status: 1},
		{Id: 11, Name: "Anthropic", Icon: "Claude.Color", Status: 1},
		{Id: 12, Name: "Custom Developer", Description: "Developer description", Icon: "Custom", Status: 1},
	}).Error)
	require.NoError(t, DB.Create(&[]Model{
		{ModelName: "gpt-", NameRule: NameRulePrefix, Status: 1, Icon: "CustomModelIcon"},
		{ModelName: "custom-model", NameRule: NameRuleExact, Status: 1, VendorID: 12},
	}).Error)
	channel := Channel{Id: 1, Type: 1, Status: common.ChannelStatusEnabled, SupplierID: &profile.SupplierID, SupplierProfileID: &profile.ID}
	require.NoError(t, DB.Create(&channel).Error)
	names := []string{"gpt-display", "claude-display", "custom-model", "supplier-only", "gpt-image-1"}
	tariffs := make([]types.SupplierModelTariff, 0, len(names))
	for _, name := range names {
		require.NoError(t, DB.Create(&Ability{Group: "2", Model: name, ChannelId: 1, Enabled: true}).Error)
		tariffs = append(tariffs, supplierTestTariff(name, 0.1))
	}
	_, err := PublishSupplierProfileTariffs(profile.ID, 1, 0, tariffs)
	require.NoError(t, err)
	require.NoError(t, DB.Create(&PricingGroupSupplierBinding{GroupID: 2, SupplierID: profile.SupplierID, ProfileID: profile.ID}).Error)
	writes := 0
	require.NoError(t, DB.Callback().Create().Before("gorm:create").Register("public-display-no-write", func(*gorm.DB) { writes++ }))

	catalog, err := GetPublicPricingCatalog(map[string]string{"2": "Retail"}, map[string]float64{"2": 2})

	require.NoError(t, err)
	require.Zero(t, writes, "provider inference must not write model or vendor records")
	byName := make(map[string]PublicPricingModel, len(catalog))
	for _, item := range catalog {
		byName[item.ModelName] = item
	}
	require.Equal(t, 10, byName["gpt-display"].VendorID)
	require.Equal(t, "OpenAI", byName["gpt-display"].VendorName)
	require.Equal(t, "OpenAI", byName["gpt-display"].VendorIcon)
	require.Equal(t, "CustomModelIcon", byName["gpt-display"].Icon)
	require.Equal(t, 11, byName["claude-display"].VendorID)
	require.Equal(t, "Anthropic", byName["claude-display"].VendorName)
	require.Equal(t, "Claude.Color", byName["claude-display"].VendorIcon)
	require.Equal(t, "Custom Developer", byName["custom-model"].VendorName)
	require.Equal(t, "Developer description", byName["custom-model"].VendorDescription)
	require.Equal(t, 4.0, byName["gpt-display"].BaseReference.InputUSD)
	require.Equal(t, 16.0, byName["gpt-display"].BaseReference.OutputUSD)
	require.Equal(t, 0.2, *byName["gpt-display"].GroupPrices[0].InputUSD)
	require.Nil(t, byName["supplier-only"].BaseReference)
	require.Nil(t, byName["gpt-image-1"].BaseReference)
	data, err := common.Marshal(catalog)
	require.NoError(t, err)
	for _, private := range []string{"supplier_id", "profile_id", "usd_per_credit", "revision", "\"expression\""} {
		require.NotContains(t, string(data), private)
	}
	// A public reference must not make a missing supplier tariff billable.
	require.NoError(t, DB.Where("profile_id = ? AND model_name = ?", profile.ID, "gpt-display").Delete(&SupplierModelPrice{}).Error)
	catalog, err = GetPublicPricingCatalog(map[string]string{"2": "Retail"}, map[string]float64{"2": 2})
	require.NoError(t, err)
	for _, item := range catalog {
		if item.ModelName == "gpt-display" {
			require.NotNil(t, item.BaseReference)
			require.True(t, item.GroupPrices[0].Unavailable)
		}
	}
}

func TestPublicBaseReferenceKeepsComparableGlobalTokenRates(t *testing.T) {
	preservePublicDisplayPrices(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"flat":2,"embedding":3,"free":0,"fixed":2}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"flat":4,"embedding":0,"free":0}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed":0.1}`))
	require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
		billing_setting.BillingModeField: `{"expr-flat":"tiered_expr","expr-tiered":"tiered_expr"}`,
		billing_setting.BillingExprField: `{"expr-flat":"p*2+c*3","expr-tiered":"len<=100 ? tier(\"short\",p*2+c*3) : tier(\"long\",p*4+c*6)"}`,
	}))
	require.Equal(t, &PublicBaseReference{InputUSD: 4, OutputUSD: 16, PriceUnit: "million_tokens"}, publicBaseReference("flat"))
	require.Equal(t, &PublicBaseReference{InputUSD: 6, OutputUSD: 0, PriceUnit: "million_tokens"}, publicBaseReference("embedding"))
	require.Equal(t, &PublicBaseReference{InputUSD: 2, OutputUSD: 3, PriceUnit: "million_tokens"}, publicBaseReference("expr-flat"))
	require.Equal(t, &PublicBaseReference{InputUSD: 2, OutputUSD: 3, PriceUnit: "million_tokens"}, publicBaseReference("expr-tiered"))
	for _, name := range []string{"", "unknown", "fixed", "free"} {
		require.Nil(t, publicBaseReference(name), name)
	}
}

func TestPublicTieredBaseReferenceUsesKnownLegacyPairOrFirstGlobalTier(t *testing.T) {
	preservePublicDisplayPrices(t)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"tiered-with-legacy":4,"tiered-without-completion":9,"unconfigured-global":9}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"tiered-with-legacy":5}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"fixed-with-expression":0.1}`))
	expression := `len<=100 ? tier("short",p*2+c*3) : tier("long",p*4+c*6)`
	mode := map[string]string{}
	expressions := map[string]string{}
	for _, name := range []string{"tiered-with-legacy", "tiered-without-completion", "tiered-without-ratio"} {
		mode[name] = "tiered_expr"
		expressions[name] = expression
	}
	modeData, err := common.Marshal(mode)
	require.NoError(t, err)
	expressionData, err := common.Marshal(expressions)
	require.NoError(t, err)
	require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
		billing_setting.BillingModeField: string(modeData), billing_setting.BillingExprField: string(expressionData),
	}))
	require.Equal(t, &PublicBaseReference{InputUSD: 8, OutputUSD: 40, PriceUnit: "million_tokens"}, publicBaseReference("tiered-with-legacy"))
	for _, name := range []string{"tiered-without-completion", "tiered-without-ratio"} {
		require.Equal(t, &PublicBaseReference{InputUSD: 2, OutputUSD: 3, PriceUnit: "million_tokens"}, publicBaseReference(name), name)
	}
	require.Nil(t, publicBaseReference("fixed-with-expression"))
	require.Nil(t, publicBaseReference("supplier-only-without-global"))
}

func TestPublicTieredBaseReferenceUsesConfiguredQuotaUnits(t *testing.T) {
	preservePublicDisplayPrices(t)
	previousQuotaPerUnit := common.GetQuotaPerUnit()
	t.Cleanup(func() { common.SetQuotaPerUnit(previousQuotaPerUnit) })
	common.SetQuotaPerUnit(1_000_000)
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"tiered-custom-unit":4,"ratio-custom-unit":4}`))
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"tiered-custom-unit":5,"ratio-custom-unit":5}`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{}`))
	require.NoError(t, billing_setting.UpdateFromMap(map[string]string{
		billing_setting.BillingModeField: `{"tiered-custom-unit":"tiered_expr"}`,
		billing_setting.BillingExprField: `{"tiered-custom-unit":"len<=100 ? tier(\"short\",p*2+c*3) : tier(\"long\",p*4+c*6)"}`,
	}))

	reference := publicBaseReference("tiered-custom-unit")

	require.Equal(t, &PublicBaseReference{InputUSD: 4, OutputUSD: 20, PriceUnit: "million_tokens"}, reference)
	require.Equal(t, publicBaseReference("ratio-custom-unit"), reference, "both global display paths must use the same quota-to-USD scale")
}

func TestPublicPricingProviderFallbackRequiresNoStoredMetadata(t *testing.T) {
	byID, byName := publicPricingVendorMaps(nil)
	openai := publicPricingVendor("gpt-6.1-sol", 0, byID, byName)
	claude := publicPricingVendor("Claude-Sonnet-5.5", 0, byID, byName)
	require.Equal(t, "OpenAI", openai.Name)
	require.Equal(t, "OpenAI", openai.Icon)
	require.Equal(t, "Anthropic", claude.Name)
	require.Equal(t, "Claude.Color", claude.Icon)
	require.Equal(t, []PricingVendor{
		{Name: "Anthropic", Icon: "Claude.Color"}, {Name: "OpenAI", Icon: "OpenAI"},
	}, GetPublicPricingVendors([]PublicPricingModel{
		{ModelName: "gpt-6.1-sol", VendorName: openai.Name, VendorIcon: openai.Icon},
		{ModelName: "claude-sonnet", VendorName: claude.Name, VendorIcon: claude.Icon},
	}))
}

package helper

import (
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMain(m *testing.M) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		panic(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		panic(err)
	}
	sqlDB.SetMaxOpenConns(1)
	model.DB = db
	if err := db.AutoMigrate(&model.User{}, &model.Supplier{}, &model.SupplierPricingProfile{}, &model.PricingGroupSupplierBinding{}, &model.SupplierPricingState{}, &model.SupplierPricingRevision{}, &model.SupplierModelPrice{}); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func supplierTestPrice(value float64) *float64 { return &value }

func supplierTestContext(body string) (*gin.Context, *relaycommon.RelayInfo) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	return ctx, &relaycommon.RelayInfo{
		OriginModelName: "shared-model", UsingGroup: "default", UserGroup: "default",
		BillingRequestInput: &billingexpr.RequestInput{Body: []byte(body)},
	}
}

func TestSupplierPriceKeepsCacheAndOutputIndependentOfDefaultRates(t *testing.T) {
	ctx, info := supplierTestContext(`{}`)
	tariff := types.SupplierModelTariff{
		SupplierID: 11, ProfileID: 12, Revision: 3,
		BillingMode: types.SupplierBillingModeToken,
		InputUSD:    supplierTestPrice(2), OutputUSD: supplierTestPrice(8),
		CacheReadUSD: supplierTestPrice(0.2), ReasoningUSD: supplierTestPrice(8),
	}
	price, err := modelPriceHelperSupplier(ctx, info, 1000, &types.TokenCountMeta{MaxTokens: 100}, types.GroupRatioInfo{GroupRatio: 2}, tariff)
	require.NoError(t, err)
	require.NotNil(t, info.TieredBillingSnapshot)
	require.Equal(t, 0.1, price.CacheRatio)
	require.Equal(t, 11, price.SupplierTariff.SupplierID)
	params := billingexpr.TokenParams{P: 100, CR: 900, C: 100, Len: 1000}
	cost, _, err := billingexpr.RunExprWithRequest(info.TieredBillingSnapshot.ExprString, params, billingexpr.RequestInput{})
	require.NoError(t, err)
	// (100*2 + 900*0.2 + 100*8) USD/1M, then exactly x2.
	require.Equal(t, 1180.0, cost)
	require.Equal(t, 1180, billingexpr.QuotaRound(cost/1_000_000*common.GetQuotaPerUnit()*2))
	*tariff.CacheReadUSD = 99
	tariff.Tools = map[string]float64{"web_search": 99}
	require.Equal(t, 0.2, *price.SupplierTariff.CacheReadUSD)
}

func TestSupplierPriceSupportsFreeInputWithPaidOutput(t *testing.T) {
	ctx, info := supplierTestContext(`{}`)
	price, err := modelPriceHelperSupplier(ctx, info, 1000, &types.TokenCountMeta{MaxTokens: 100}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
		BillingMode: types.SupplierBillingModeToken,
		InputUSD:    supplierTestPrice(0), OutputUSD: supplierTestPrice(5), ReasoningUSD: supplierTestPrice(5),
	})
	require.NoError(t, err)
	require.Equal(t, 500, price.QuotaToPreConsume)
	require.False(t, price.FreeModel)
}

func TestSupplierRequestRejectsUnpricedFeaturesBeforeDispatch(t *testing.T) {
	for _, body := range []string{
		`{"tools":[{"type":"web_search_preview"}]}`,
		`{"messages":[{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,a"}}]}]}`,
		`{"messages":[{"content":[{"type":"text","cache_control":{"type":"ephemeral"}}]}]}`,
		`{"modalities":["audio"]}`,
		`{"messages":[{"content":[{"type":"text","cache_control":{"type":"ephemeral","ttl":"12h"}}]}]}`,
		`{"prompt_cache_retention":"24h"}`,
		`{"messages":[{"content":[{"type":"text","cache_control":{"type":"ephemeral","scope":"global"}}]}]}`,
	} {
		t.Run(body, func(t *testing.T) {
			ctx, info := supplierTestContext(body)
			_, err := modelPriceHelperSupplier(ctx, info, 1000, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
				BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2),
			})
			require.Error(t, err)
			require.Nil(t, info.PriceData.SupplierTariff)
		})
	}
}

func TestSupplierExplicitFreeFeatureIsSupported(t *testing.T) {
	ctx, info := supplierTestContext(`{"tools":[{"type":"web_search_preview"}]}`)
	_, err := modelPriceHelperSupplier(ctx, info, 100, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 1}, types.SupplierModelTariff{
		BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2),
		Tools: map[string]float64{"web_search_preview": 0},
	})
	require.NoError(t, err)
}

func TestSupplierServerToolsRequireSupportedUsageCounters(t *testing.T) {
	for _, kind := range []string{"code_interpreter", "computer", "image_generation"} {
		t.Run(kind, func(t *testing.T) {
			body, err := common.Marshal(map[string]any{"tools": []map[string]string{{"type": kind}}})
			require.NoError(t, err)
			ctx, info := supplierTestContext(string(body))
			_, err = modelPriceHelperSupplier(ctx, info, 10, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
				BillingMode: types.SupplierBillingModeToken, InputUSD: supplierTestPrice(1), OutputUSD: supplierTestPrice(2),
				Tools: map[string]float64{kind: 0.01},
			})
			require.Error(t, err)
			require.False(t, info.AttemptDispatched)
			require.Nil(t, info.TieredBillingSnapshot)
		})
	}
	for _, body := range []string{
		`{"tools":[{"type":"function","name":"client_tool","parameters":{"type":"object"}}]}`,
		`{"tools":[{"name":"client_tool","input_schema":{"type":"object"}}]}`,
		`{"tools":[{"functionDeclarations":[{"name":"client_tool"}]}]}`,
	} {
		ctx, info := supplierTestContext(body)
		require.NoError(t, ValidateSupplierRequest(ctx, info, types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken}))
	}
}

func TestSupplierRequestNeverReloadsPriceAfterSnapshot(t *testing.T) {
	ctx, info := supplierTestContext(`{}`)
	price, err := modelPriceHelperSupplier(ctx, info, 100, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
		SupplierID: 3, ProfileID: 4, Revision: 7, BillingMode: types.SupplierBillingModeFixed,
		PriceUSD: supplierTestPrice(0.01), PriceUnit: "request",
	})
	require.NoError(t, err)
	info.UsingGroup = "group-removed-during-request"
	refreshed, err := ModelPriceHelper(ctx, info, 100000, &types.TokenCountMeta{MaxTokens: 100000})
	require.NoError(t, err)
	require.Equal(t, price.QuotaToPreConsume, refreshed.QuotaToPreConsume)
	require.Equal(t, int64(7), refreshed.SupplierTariff.Revision)
	require.Equal(t, 2.0, refreshed.GroupRatioInfo.GroupRatio)
}

func TestSupplierUnexpectedCacheUsageIsNotPricedWithDefaults(t *testing.T) {
	_, info := supplierTestContext(`{}`)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken}
	require.Error(t, ValidateSupplierUsage(info, &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 50}}))
}

func TestSupplierRealtimeSessionCannotEnableUnsettledServerTools(t *testing.T) {
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{
		BillingMode:   types.SupplierBillingModeToken,
		AudioInputUSD: supplierTestPrice(1), AudioOutputUSD: supplierTestPrice(2),
		Tools: map[string]float64{"web_search_preview": 0.01},
	}}}
	require.Error(t, ValidateSupplierRealtimeMessage(info, []byte(`{"type":"session.update","session":{"tools":[{"type":"web_search_preview"}]}}`)))
	require.NoError(t, ValidateSupplierRealtimeMessage(info, []byte(`{"type":"session.update","session":{"tools":[{"type":"function","name":"client_tool"}]}}`)))
}

func TestSupplierTaskDurationIsAppliedBeforeQuotaRounding(t *testing.T) {
	quotaPerUnit := common.GetQuotaPerUnit()
	t.Cleanup(func() { common.SetQuotaPerUnit(quotaPerUnit) })
	price := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
		QuotaPerUnit:   quotaPerUnit,
		SupplierTariff: &types.SupplierModelTariff{PriceUSD: supplierTestPrice(0.0000006), PriceUnit: "per_second"},
	}
	common.SetQuotaPerUnit(quotaPerUnit * 2)
	require.NoError(t, ApplySupplierTaskPrice(&price, map[string]float64{"seconds": 10, "size": 99}))
	require.Equal(t, 6, price.Quota)
	require.Equal(t, map[string]float64{"seconds": 10}, price.OtherRatios)
	price.SupplierTariff.PriceUnit = "request"
	require.NoError(t, ApplySupplierTaskPrice(&price, map[string]float64{"seconds": 10, "size": 99}))
	require.Equal(t, 1, price.Quota)
	require.Empty(t, price.OtherRatios)
}

func TestSupplierInvalidQuotaCannotReachUpstream(t *testing.T) {
	ctx, info := supplierTestContext(`{}`)
	_, err := modelPriceHelperSupplier(ctx, info, 100, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
		BillingMode: types.SupplierBillingModeFixed, PriceUnit: "request", PriceUSD: supplierTestPrice(math.MaxFloat64),
	})
	require.Error(t, err)
	require.Nil(t, info.PriceData.SupplierTariff)
}

func TestSupplierFixedImageRequestFailsBeforeDispatch(t *testing.T) {
	ctx, info := supplierTestContext(`{"model":"image-model","n":1}`)
	info.RelayFormat = types.RelayFormatOpenAIImage
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	_, err := modelPriceHelperSupplier(ctx, info, 100, &types.TokenCountMeta{}, types.GroupRatioInfo{GroupRatio: 2}, types.SupplierModelTariff{
		BillingMode: types.SupplierBillingModeFixed, PriceUSD: supplierTestPrice(0.01), PriceUnit: "request",
	})
	require.Error(t, err)
	require.False(t, info.AttemptDispatched)
	require.Nil(t, info.PriceData.SupplierTariff)
}

func TestBoundGroupWithoutTariffDoesNotFallBackToDefaultPrice(t *testing.T) {
	oldGroups := ratio_setting.PricingGroups2JSONString()
	oldPrice := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(oldGroups))
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrice))
		require.NoError(t, model.DB.Delete(&model.PricingGroupSupplierBinding{}, "group_id = ?", 92).Error)
	})
	require.NoError(t, ratio_setting.UpdatePricingGroupsByJSONString(`[{"id":1,"name":"default","ratio":1},{"id":92,"name":"bound","ratio":2}]`))
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"shared-model":0.01}`))
	require.NoError(t, model.DB.Create(&model.PricingGroupSupplierBinding{GroupID: 92, SupplierID: 92, ProfileID: 92}).Error)
	ctx, info := supplierTestContext(`{}`)
	info.UsingGroup = "92"
	_, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.Error(t, err)
	require.Nil(t, info.PriceData.SupplierTariff)
	info.UsingGroup = "default"
	price, err := ModelPriceHelper(ctx, info, 1000, &types.TokenCountMeta{})
	require.NoError(t, err)
	require.True(t, price.UsePrice)
	require.Equal(t, 0.01, price.ModelPrice)
}

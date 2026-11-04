package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierChargedErrorRetainsKnownSubtotalAboveReserve(t *testing.T) {
	truncate(t)
	const userID, initialQuota, reserve = 9511, 1000000, 10
	seedUser(t, userID, initialQuota)
	seedToken(t, userID, userID, "supplier-partial-usage", initialQuota)
	seedChannel(t, userID)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Set(common.RequestIdKey, "supplier-partial-usage")
	ctx.Set("username", "test_user")
	input, output := 1.0, 10.0
	tariff := types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken, InputUSD: &input, OutputUSD: &output, ReasoningUSD: &output}
	expression, err := helper.SupplierTariffExpression(tariff)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{
		UserId: userID, TokenId: userID, IsPlayground: true, OriginModelName: "partial-usage", UsingGroup: "default",
		RequestId: "supplier-partial-usage", StartTime: time.Now(), ChannelMeta: &relaycommon.ChannelMeta{ChannelId: userID},
		FinalPreConsumedQuota: reserve, AttemptDispatched: true,
		PriceData: types.PriceData{ModelRatio: 0.5, CompletionRatio: 10, QuotaToPreConsume: reserve,
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}, QuotaPerUnit: common.GetQuotaPerUnit(), SupplierTariff: &tariff},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression,
			ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 2, QuotaPerUnit: common.GetQuotaPerUnit(), EstimatedQuotaAfterGroup: reserve},
	}
	funding := &WalletFunding{userId: userID}
	require.NoError(t, funding.PreConsume(reserve))
	info.Billing = &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: reserve}
	usage := &dto.Usage{PromptTokens: 10, CompletionTokens: 10000, TotalTokens: 10010,
		PromptTokensDetails:    dto.InputTokenDetails{CachedTokens: 1},
		CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokensPresent: true}}
	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	require.False(t, info.AttemptActualQuotaKnown)
	knownQuota := billingexpr.QuotaRound((9*input + 10000*output) / 1_000_000 * common.GetQuotaPerUnit() * 2)
	require.Greater(t, knownQuota, reserve)
	require.Equal(t, knownQuota, info.AttemptActualQuota)
	upstreamErr := types.NewErrorWithStatusCode(errors.New("upstream stopped after generation"), types.ErrorCodeBadResponseStatusCode, http.StatusBadGateway)
	settlementErr, logErr := SettleChargedAttemptError(ctx, info, upstreamErr, ClassifyAttempt(info, upstreamErr))
	require.NoError(t, settlementErr)
	require.NoError(t, logErr)
	require.Equal(t, knownQuota, info.SettledQuota)
	require.Equal(t, initialQuota-knownQuota, getUserQuota(t, userID))
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", userID, model.LogTypeConsume).First(&log).Error)
	require.Equal(t, knownQuota, log.Quota)
}

func TestToolSurchargeOverflowNeverRefundsUserFunds(t *testing.T) {
	saved := make(map[string]string)
	require.NoError(t, config.GlobalConfig.SaveToDB(func(key, value string) error {
		if key == "tool_price_setting.prices" {
			saved[key] = value
		}
		return nil
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(saved))
		operation_setting.RebuildToolPriceIndex()
	})
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"tool_price_setting.prices": `{"web_search_preview":1e16}`}))
	operation_setting.RebuildToolPriceIndex()
	for _, mode := range []string{"supplier", "legacy-ratio", "legacy-fixed", "legacy-tiered"} {
		t.Run(mode, func(t *testing.T) {
			truncate(t)
			const id, initialQuota, reserve = 9512, 10000, 125
			seedUser(t, id, initialQuota)
			seedChannel(t, id)
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			info := &relaycommon.RelayInfo{UserId: id, IsPlayground: true, UsingGroup: "default", OriginModelName: "overflow-model", StartTime: time.Now(),
				ChannelMeta: &relaycommon.ChannelMeta{ChannelId: id}, FinalPreConsumedQuota: reserve,
				PriceData:          types.PriceData{ModelRatio: 0.5, CompletionRatio: 1, QuotaToPreConsume: reserve, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
				ResponsesUsageInfo: &relaycommon.ResponsesUsageInfo{BuiltInTools: map[string]*relaycommon.BuildInToolInfo{dto.BuildInToolWebSearchPreview: {CallCount: 3}}},
			}
			if mode == "supplier" {
				input, output := 1.0, 1.0
				tariff := types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken, InputUSD: &input, OutputUSD: &output, ReasoningUSD: &output, Tools: map[string]float64{"web_search_preview": 1e13}}
				expression, err := helper.SupplierTariffExpression(tariff)
				require.NoError(t, err)
				info.PriceData.SupplierTariff = &tariff
				info.PriceData.QuotaPerUnit = common.GetQuotaPerUnit()
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, ExprHash: billingexpr.ExprHashString(expression), GroupRatio: 1, QuotaPerUnit: common.GetQuotaPerUnit()}
			}
			if mode == "legacy-tiered" {
				info.TieredBillingSnapshot = &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: "p + c", GroupRatio: 1, QuotaPerUnit: common.GetQuotaPerUnit()}
			}
			if mode == "legacy-fixed" {
				info.PriceData.UsePrice = true
				info.PriceData.ModelPrice = 0.000002
			}
			funding := &WalletFunding{userId: id}
			require.NoError(t, funding.PreConsume(reserve))
			info.Billing = &BillingSession{relayInfo: info, funding: funding, preConsumedQuota: reserve}
			usage := &dto.Usage{PromptTokens: 1, TotalTokens: 1, CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokensPresent: true}}
			err := PostTextConsumeQuota(ctx, info, usage, nil)
			require.ErrorContains(t, err, "supported settlement range")
			require.Equal(t, reserve, info.SettledQuota)
			require.Equal(t, initialQuota-reserve, getUserQuota(t, id))
			var log model.Log
			require.NoError(t, model.LOG_DB.Where("user_id = ? AND type = ?", id, model.LogTypeConsume).First(&log).Error)
			require.Equal(t, reserve, log.Quota)
		})
	}
}

func TestSupplierFrozenQuotaScaleDoesNotUseLegacyRatioRange(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	input, output := 1e18, 1.0
	tariff := types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken, InputUSD: &input, OutputUSD: &output, ReasoningUSD: &output}
	expression, err := helper.SupplierTariffExpression(tariff)
	require.NoError(t, err)
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{ModelRatio: input / 2, SupplierTariff: &tariff, QuotaPerUnit: 1e-15, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{BillingMode: "tiered_expr", ExprString: expression, GroupRatio: 1, QuotaPerUnit: 1e-15},
	}
	require.True(t, CaptureAttemptUsageQuota(ctx, info, &dto.Usage{PromptTokens: 100, TotalTokens: 100, CompletionTokenDetails: dto.OutputTokenDetails{ReasoningTokensPresent: true}}))
	require.True(t, info.AttemptActualQuotaKnown)
	require.Zero(t, info.AttemptActualQuota)
	require.NoError(t, info.SupplierPricingError)
}

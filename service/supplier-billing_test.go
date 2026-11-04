package service

import (
	"context"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierToolPriceUsesFrozenCashPerCall(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("claude_web_search_requests", 2)
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{SupplierTariff: &types.SupplierModelTariff{
		BillingMode: types.SupplierBillingModeToken, Tools: map[string]float64{"web_search": 0.003},
	}}}
	summary := textQuotaSummary{GroupRatio: 2}
	quota := calculateTextToolCallSurcharge(ctx, info, &summary)
	require.Equal(t, 6000.0, quota.InexactFloat64())
	require.Equal(t, 3.0, summary.ClaudeWebSearchPrice)
	require.NoError(t, summary.PricingError)
	info.PriceData.QuotaPerUnit = 100000
	summary = textQuotaSummary{GroupRatio: 2}
	require.Equal(t, 1200.0, calculateTextToolCallSurcharge(ctx, info, &summary).InexactFloat64())
	delete(info.PriceData.SupplierTariff.Tools, "web_search")
	summary = textQuotaSummary{GroupRatio: 2}
	require.True(t, calculateTextToolCallSurcharge(ctx, info, &summary).IsZero())
	require.Error(t, summary.PricingError)
}

func TestSupplierUnknownCacheUsageRetainsReserve(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 800,
		PriceData: types.PriceData{
			ModelRatio: 1, CacheRatio: 0, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
			SupplierTariff: &types.SupplierModelTariff{BillingMode: types.SupplierBillingModeToken},
		},
	}
	require.True(t, CaptureAttemptUsageQuota(ctx, info, &dto.Usage{
		PromptTokens: 100, TotalTokens: 100,
		PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 100},
	}))
	require.False(t, info.AttemptActualQuotaKnown)
	require.Equal(t, types.AttemptFinancialOutcomeAmbiguous, info.AttemptFinancialOutcome)
	require.Equal(t, 800, info.AttemptActualQuota)
}

func TestSupplierCreditCostIsNotAssumedToBeRealUSD(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{PriceData: types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}, SupplierTariff: &types.SupplierModelTariff{},
	}}
	require.True(t, CaptureAttemptUsageQuota(ctx, info, &dto.Usage{Cost: 1000.0}))
	require.False(t, info.AttemptActualQuotaKnown)
	require.Zero(t, info.AttemptActualQuota)
}

func TestSupplierAudioSettlementUsesFrozenRatios(t *testing.T) {
	oldCompletion := ratio_setting.CompletionRatio2JSONString()
	oldAudio := ratio_setting.AudioRatio2JSONString()
	oldAudioCompletion := ratio_setting.AudioCompletionRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(oldCompletion))
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudio))
		require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(oldAudioCompletion))
	})
	require.NoError(t, ratio_setting.UpdateCompletionRatioByJSONString(`{"snapshot-audio":99}`))
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"snapshot-audio":99}`))
	require.NoError(t, ratio_setting.UpdateAudioCompletionRatioByJSONString(`{"snapshot-audio":99}`))
	price := types.PriceData{
		CompletionRatio: 2, AudioRatio: 3, AudioCompletionRatio: 4,
		SupplierTariff: &types.SupplierModelTariff{},
	}
	quota := calculateAudioQuota(QuotaInfo{
		PriceData: &price, BillingModelName: "snapshot-audio", ModelRatio: 1, GroupRatio: 2,
		InputDetails:  TokenDetails{TextTokens: 10, AudioTokens: 5},
		OutputDetails: TokenDetails{TextTokens: 20, AudioTokens: 7},
	})
	require.Equal(t, 298, quota)
}

func TestSupplierOverflowSettlementCannotBecomeARefund(t *testing.T) {
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 125,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: 1},
		PriceData:             types.PriceData{SupplierTariff: &types.SupplierModelTariff{}},
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr", ExprString: "p * 1e308", GroupRatio: 2,
			QuotaPerUnit: 500000, EstimatedQuotaAfterGroup: 125,
		},
	}
	ok, quota, result := TryTieredSettle(info, billingexpr.TokenParams{P: math.MaxFloat64})
	require.True(t, ok)
	require.Equal(t, 125, quota)
	require.Nil(t, result)
	require.Error(t, info.SupplierPricingError)
}

func TestTaskTokenSettlementUsesPersistedSnapshot(t *testing.T) {
	truncate(t)
	oldRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":99}`))
	const id = 911
	seedUser(t, id, 10000)
	seedToken(t, id, id, "snapshot-task-key", 10000)
	seedChannel(t, id)
	task := makeTask(id, id, 10, id, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.GroupRatio = 2
	task.PrivateData.BillingContext.PriceSnapshot = &types.PriceData{ModelRatio: 0.5, GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2}}
	require.NoError(t, model.DB.Create(task).Error)
	RecalculateTaskQuotaByTokens(context.Background(), task, 100)
	require.Equal(t, 100, task.Quota)
	require.Equal(t, 9910, getUserQuota(t, id))
}

func TestFixedTaskSnapshotDoesNotBecomeTokenPriced(t *testing.T) {
	oldRatio := ratio_setting.ModelRatio2JSONString()
	t.Cleanup(func() { require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(oldRatio)) })
	require.NoError(t, ratio_setting.UpdateModelRatioByJSONString(`{"test-model":99}`))
	task := makeTask(0, 0, 123, 0, BillingSourceWallet, 0)
	task.PrivateData.BillingContext.PriceSnapshot = &types.PriceData{UsePrice: true}
	RecalculateTaskQuotaByTokens(context.Background(), task, 100)
	require.Equal(t, 123, task.Quota)
}

package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func missingUsageTestInfo(modelName string, estimatedQuota int) (*relaycommon.RelayInfo, *autoTestSettler) {
	now := time.Now()
	info := &relaycommon.RelayInfo{
		UserId:                  9001,
		TokenId:                 9002,
		UsingGroup:              "premium",
		OriginModelName:         modelName,
		StartTime:               now,
		FirstResponseTime:       now,
		IsPlayground:            true,
		BillingSource:           BillingSourceWallet,
		FinalPreConsumedQuota:   estimatedQuota,
		AttemptDispatched:       true,
		AttemptActualQuotaKnown: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:         9003,
			UpstreamModelName: modelName,
		},
		PriceData: types.PriceData{
			ModelRatio:      1,
			CompletionRatio: 1,
			GroupRatioInfo:  types.GroupRatioInfo{GroupRatio: 1},
		},
		AutoRoute: relaycommon.AutoRouteState{
			Candidates: []relaycommon.AutoRouteCandidate{{
				Group:          "premium",
				EstimatedQuota: estimatedQuota,
			}},
		},
	}
	settler := &autoTestSettler{info: info, preConsumed: estimatedQuota}
	info.Billing = settler
	return info, settler
}

func assertEstimatedMissingUsageLog(t *testing.T, modelName string, estimatedQuota int) {
	t.Helper()
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("model_name = ?", modelName).Order("id desc").First(&log).Error)
	assert.Equal(t, estimatedQuota, log.Quota)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(log.Other), &other))
	assert.Equal(t, true, other["usage_missing"])
	assert.Equal(t, true, other["estimated_billing"])
	assert.Equal(t, string(types.AttemptFinancialOutcomeAmbiguous), other["financial_outcome"])
}

func TestPostTextConsumeQuotaMissingUsageKeepsEstimate(t *testing.T) {
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info, settler := missingUsageTestInfo("missing-usage-text", 37)

	require.NoError(t, PostTextConsumeQuota(ctx, info, &dto.Usage{}, nil))

	assert.Equal(t, 37, settler.actualQuota)
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, info.AttemptFinancialOutcome)
	assertEstimatedMissingUsageLog(t, info.OriginModelName, 37)
}

func TestPostAudioConsumeQuotaMissingUsageKeepsEstimate(t *testing.T) {
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info, settler := missingUsageTestInfo("missing-usage-audio", 41)

	require.NoError(t, PostAudioConsumeQuota(ctx, info, &dto.Usage{}, ""))

	assert.Equal(t, 41, settler.actualQuota)
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, info.AttemptFinancialOutcome)
	assertEstimatedMissingUsageLog(t, info.OriginModelName, 41)
}

func TestPostAudioConsumeQuotaUsesMappedBillingTargetRatios(t *testing.T) {
	oldAudioRatio := ratio_setting.AudioRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudioRatio))
	})
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"provider-model":3}`))

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info, settler := missingUsageTestInfo("alias-model", 0)
	info.BillingModelName = "provider-model"
	info.IsModelMapped = true
	info.UpstreamModelName = "provider-model"

	usage := &dto.Usage{PromptTokensDetails: dto.InputTokenDetails{AudioTokens: 10}}
	require.NoError(t, PostAudioConsumeQuota(ctx, info, usage, ""))
	assert.Equal(t, 30, settler.actualQuota)
}

func TestCalculateAudioQuotaKeepsOriginIdentityAndUsesBillingTarget(t *testing.T) {
	oldAudioRatio := ratio_setting.AudioRatio2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(oldAudioRatio))
	})
	require.NoError(t, ratio_setting.UpdateAudioRatioByJSONString(`{"provider-model":3}`))

	quotaInfo := QuotaInfo{
		ModelName:        "alias-model",
		BillingModelName: "provider-model",
		InputDetails:     TokenDetails{AudioTokens: 10},
		ModelRatio:       1,
		GroupRatio:       1,
	}
	assert.Equal(t, "alias-model", quotaInfo.ModelName)
	assert.Equal(t, 30, calculateAudioQuota(quotaInfo))
}

func TestPostWssConsumeQuotaMissingUsageKeepsEstimate(t *testing.T) {
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info, settler := missingUsageTestInfo("missing-usage-wss", 43)

	require.NoError(t, PostWssConsumeQuota(ctx, info, info.OriginModelName, &dto.RealtimeUsage{}, ""))

	assert.Equal(t, 43, settler.actualQuota)
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, info.AttemptFinancialOutcome)
	assertEstimatedMissingUsageLog(t, info.OriginModelName, 43)
}

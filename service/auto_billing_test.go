package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type autoTestFunding struct {
	source        string
	preConsumeErr error
	settleErr     error
	refundErr     error
	settleDelta   int
	settleCalls   int
	refundCalls   int
}

type autoTestSettler struct {
	info        *relaycommon.RelayInfo
	preConsumed int
	settleErr   error
	partial     bool
	actualQuota int
	reserve     int
	onSettle    func(int)
}

func (s *autoTestSettler) Settle(actualQuota int) error {
	s.actualQuota = actualQuota
	if s.onSettle != nil {
		s.onSettle(actualQuota)
	}
	if s.partial {
		s.info.BillingSettled = true
		s.info.SettledQuota = actualQuota
	}
	return s.settleErr
}

func (s *autoTestSettler) Refund(*gin.Context) error {
	return nil
}

func (s *autoTestSettler) NeedsRefund() bool {
	return false
}

func (s *autoTestSettler) GetPreConsumedQuota() int {
	return s.preConsumed
}

func (s *autoTestSettler) Reserve(quota int) error {
	s.reserve = quota
	return nil
}

func (f *autoTestFunding) Source() string {
	return f.source
}

func (f *autoTestFunding) PreConsume(int) error {
	return f.preConsumeErr
}

func (f *autoTestFunding) Settle(delta int) error {
	f.settleCalls++
	f.settleDelta = delta
	return f.settleErr
}

func (f *autoTestFunding) Refund() error {
	f.refundCalls++
	return f.refundErr
}

func TestSettleChargedAttemptErrorRetriesFailedConsumeLogFromMainDBOutbox(t *testing.T) {
	truncate(t)

	const userID, tokenID, channelID, heldQuota = 81, 81, 81, 1200
	seedUser(t, userID, 5000)
	seedToken(t, tokenID, userID, "sk-charged-outbox", 5000)
	seedChannel(t, channelID)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-charged-outbox")
	c.Set("username", "test_user")
	c.Set("token_name", "test_token")
	info := &relaycommon.RelayInfo{
		UserId:                userID,
		TokenId:               tokenID,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelId: channelID},
		RequestId:             "req-charged-outbox",
		OriginModelName:       "test-model",
		UsingGroup:            "default",
		FinalPreConsumedQuota: heldQuota,
		AttemptDispatched:     true,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: heldQuota,
		},
	}
	intentSeenBeforeSettlement := false
	intentDeferredBeforeSettlement := false
	settler := &autoTestSettler{info: info, preConsumed: heldQuota}
	settler.onSettle = func(_ int) {
		var event model.BillingOutbox
		if err := model.DB.Where("kind = ?", model.BillingOutboxKindLog).First(&event).Error; err != nil {
			return
		}
		intentSeenBeforeSettlement = strings.Contains(event.Payload, "settlement_pending")
		intentDeferredBeforeSettlement = event.NextAttemptAt > common.GetTimestamp()
	}
	info.Billing = settler
	relayErr := types.NewErrorWithStatusCode(errors.New("upstream timeout"), types.ErrorCodeDoRequestFailed, http.StatusGatewayTimeout)

	originalLogDB := model.LOG_DB
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	missingLogDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = missingLogDB

	settleErr, logErr := SettleChargedAttemptError(c, info, relayErr, types.AttemptFinancialOutcomeAmbiguous)
	require.NoError(t, settleErr)
	require.Error(t, logErr)
	assert.Equal(t, heldQuota, settler.actualQuota)
	assert.True(t, intentSeenBeforeSettlement)
	assert.True(t, intentDeferredBeforeSettlement)

	var event model.BillingOutbox
	require.NoError(t, model.DB.Where("kind = ?", model.BillingOutboxKindLog).First(&event).Error)
	assert.NotEmpty(t, event.EventID)

	model.LOG_DB = originalLogDB
	require.NoError(t, model.DB.Model(&model.BillingOutbox{}).Where("id = ?", event.ID).Update("next_attempt_at", 0).Error)
	result := model.ProcessBillingOutbox(context.Background(), 10)
	assert.Equal(t, 1, result.Processed)

	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", event.EventID).First(&stored).Error)
	assert.Equal(t, heldQuota, stored.Quota)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(stored.Other, &other))
	assert.Equal(t, "req-charged-outbox", other["original_request_id"])
	assert.Equal(t, true, other["charged_on_error"])
	_, settlementPending := other["settlement_pending"]
	assert.False(t, settlementPending)
}

func TestFinalChargedLogReplacesGraceDeliveredIntent(t *testing.T) {
	truncate(t)

	const userID = 82
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-final-wins")
	c.Set("username", "test_user")
	eventID := "event-final-wins"

	pending := model.RecordConsumeLogParams{
		Quota:   1200,
		Content: "settlement pending",
		Group:   "default",
		Other: map[string]interface{}{
			"settlement_pending": true,
		},
		Force: true,
	}
	require.NoError(t, model.StageConsumeLogOutboxIntent(c, userID, eventID, pending))
	require.NoError(t, model.DB.Model(&model.BillingOutbox{}).Where("event_id = ?", eventID).Update("next_attempt_at", 0).Error)
	assert.Equal(t, 1, model.ProcessBillingOutbox(context.Background(), 10).Processed)

	final := pending
	final.Quota = 700
	final.Content = "settled"
	final.Other = map[string]interface{}{"charged_on_error": true}
	require.NoError(t, model.UpsertConsumeLogOutboxIntent(c, userID, eventID, final))
	require.NoError(t, model.DeliverBillingOutboxEvent(eventID))

	var count int64
	require.NoError(t, model.LOG_DB.Model(&model.Log{}).Where("request_id = ?", eventID).Count(&count).Error)
	assert.Equal(t, int64(1), count)
	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", eventID).First(&stored).Error)
	assert.Equal(t, 700, stored.Quota)
	assert.Equal(t, "settled", stored.Content)
	assert.NotContains(t, stored.Other, "settlement_pending")
}

func TestChargedLogFallsBackWhileBillingOutboxMigrationIsPending(t *testing.T) {
	truncate(t)
	require.NoError(t, model.DB.Migrator().DropTable(&model.BillingOutbox{}))
	t.Cleanup(func() {
		require.NoError(t, model.DB.AutoMigrate(&model.BillingOutbox{}))
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "req-pre-migration")
	pending := model.RecordConsumeLogParams{
		Quota: 900,
		Other: map[string]interface{}{
			"settlement_pending": true,
		},
		Force: true,
	}
	require.NoError(t, model.StageConsumeLogOutboxIntent(c, 83, "event-pre-migration", pending))

	final := pending
	final.Quota = 600
	final.Other = map[string]interface{}{"charged_on_error": true}
	require.NoError(t, model.UpsertConsumeLogOutboxIntent(c, 83, "event-pre-migration", final))
	require.NoError(t, model.DeliverBillingOutboxEvent("event-pre-migration"))

	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", "req-pre-migration").First(&stored).Error)
	assert.Equal(t, 600, stored.Quota)
}

func TestClassifyAttemptSeparatesFinancialOutcomeFromRetryability(t *testing.T) {
	freeErr := types.NewError(
		errors.New("rate limited"),
		types.ErrorCodeBadResponseStatusCode,
		types.ErrOptionWithFinancialOutcome(types.AttemptFinancialOutcomeNonBillable),
	)
	unknownErr := types.NewError(errors.New("transport reset"), types.ErrorCodeDoRequestFailed)
	billableErr := types.NewError(
		errors.New("accepted task failed"),
		types.ErrorCodeBadResponseStatusCode,
		types.ErrOptionWithFinancialOutcome(types.AttemptFinancialOutcomeBillable),
	)

	tests := []struct {
		name    string
		info    relaycommon.RelayInfo
		err     *types.NewAPIError
		outcome types.AttemptFinancialOutcome
	}{
		{
			name:    "explicit free response after dispatch",
			info:    relaycommon.RelayInfo{AttemptDispatched: true},
			err:     freeErr,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "unknown transport result after dispatch",
			info:    relaycommon.RelayInfo{AttemptDispatched: true},
			err:     unknownErr,
			outcome: types.AttemptFinancialOutcomeAmbiguous,
		},
		{
			name:    "local failure before dispatch",
			info:    relaycommon.RelayInfo{},
			err:     unknownErr,
			outcome: types.AttemptFinancialOutcomeNonBillable,
		},
		{
			name:    "partial response overrides free marker",
			info:    relaycommon.RelayInfo{AttemptDispatched: true, ReceivedResponseCount: 1},
			err:     freeErr,
			outcome: types.AttemptFinancialOutcomeAmbiguous,
		},
		{
			name:    "known actual usage is billable",
			info:    relaycommon.RelayInfo{AttemptDispatched: true, AttemptActualQuota: 25},
			err:     unknownErr,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
		{
			name: "partial response overrides known zero",
			info: relaycommon.RelayInfo{
				AttemptDispatched:       true,
				AttemptActualQuotaKnown: true,
				ReceivedResponseCount:   1,
			},
			err:     freeErr,
			outcome: types.AttemptFinancialOutcomeAmbiguous,
		},
		{
			name: "explicit billable error overrides known zero",
			info: relaycommon.RelayInfo{
				AttemptDispatched:       true,
				AttemptActualQuotaKnown: true,
			},
			err:     billableErr,
			outcome: types.AttemptFinancialOutcomeBillable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.outcome, ClassifyAttempt(&tt.info, tt.err))
		})
	}
}

func TestShouldRetryAttemptRequiresProvenFreeOutcome(t *testing.T) {
	assert.True(t, ShouldRetryAttempt(types.AttemptFinancialOutcomeNonBillable, true, true))
	assert.False(t, ShouldRetryAttempt(types.AttemptFinancialOutcomeNonBillable, false, true))
	assert.False(t, ShouldRetryAttempt(types.AttemptFinancialOutcomeNonBillable, true, false))
	assert.False(t, ShouldRetryAttempt(types.AttemptFinancialOutcomeBillable, true, true))
	assert.False(t, ShouldRetryAttempt(types.AttemptFinancialOutcomeAmbiguous, true, true))
}

func TestShouldChargeAttemptRequiresConfirmedBilling(t *testing.T) {
	assert.True(t, ShouldChargeAttempt(types.AttemptFinancialOutcomeBillable))
	assert.False(t, ShouldChargeAttempt(types.AttemptFinancialOutcomeNonBillable))
	assert.False(t, ShouldChargeAttempt(types.AttemptFinancialOutcomeAmbiguous))
	assert.False(t, ShouldChargeAttempt(types.AttemptFinancialOutcomeUnknown))
}

func TestShouldRetryAutoAttemptUsesAnyProvenFreeDispatchedError(t *testing.T) {
	assert.True(t, ShouldRetryAutoAttempt(types.AttemptFinancialOutcomeNonBillable, true, false, true))
	assert.False(t, ShouldRetryAutoAttempt(types.AttemptFinancialOutcomeNonBillable, true, false, false))
	assert.False(t, ShouldRetryAutoAttempt(types.AttemptFinancialOutcomeBillable, true, true, true))
}

func TestShouldRetryAutoAttemptKeepsPredispatchRetryPolicy(t *testing.T) {
	assert.False(t, ShouldRetryAutoAttempt(types.AttemptFinancialOutcomeNonBillable, false, false, true))
	assert.True(t, ShouldRetryAutoAttempt(types.AttemptFinancialOutcomeNonBillable, false, true, true))
}

func TestClassifyTaskAttemptKeepsAcceptedSideEffectBillable(t *testing.T) {
	taskErr := &dto.TaskError{
		FinancialOutcome: types.AttemptFinancialOutcomeNonBillable,
	}
	info := &relaycommon.RelayInfo{AttemptDispatched: true}

	assert.Equal(t, types.AttemptFinancialOutcomeNonBillable, ClassifyTaskAttempt(info, taskErr))

	info.MarkAttemptTaskAccepted()
	assert.Equal(t, types.AttemptFinancialOutcomeBillable, ClassifyTaskAttempt(info, taskErr))
}

func TestCaptureAttemptUsageTreatsExplicitZeroCostAsKnownFree(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		AttemptDispatched: true,
		StartTime:         time.Now(),
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
		},
	}
	usage := &dto.Usage{Cost: float64(0)}
	relayErr := types.NewError(errors.New("upstream rejected request"), types.ErrorCodeBadResponseStatusCode)

	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	assert.True(t, info.AttemptActualQuotaKnown)
	assert.Zero(t, info.AttemptActualQuota)
	assert.False(t, info.AttemptHasBillingEvidence)
	assert.False(t, info.AttemptUsageBillingEvidence)
	assert.Equal(t, types.AttemptFinancialOutcomeNonBillable, ClassifyAttempt(info, relayErr))
}

func TestCaptureAttemptUsageConvertsNonzeroCostWithoutTokens(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		AttemptDispatched: true,
		StartTime:         time.Now(),
		PriceData: types.PriceData{
			GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 2},
		},
	}
	usage := &dto.Usage{Cost: float64(0.002)}
	relayErr := types.NewError(errors.New("upstream rejected request"), types.ErrorCodeBadResponseStatusCode)

	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	assert.True(t, info.AttemptActualQuotaKnown)
	assert.Equal(t, 2000, info.AttemptActualQuota)
	assert.True(t, info.AttemptUsageBillingEvidence)
	assert.Equal(t, types.AttemptFinancialOutcomeBillable, ClassifyAttempt(info, relayErr))
	assert.Equal(t, 2000, chargedAttemptQuota(info))
}

func TestCaptureAttemptUsageKeepsUpstreamTokensBillableWhenUserQuotaIsZero(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		AttemptDispatched: true,
		StartTime:         time.Now(),
	}
	usage := &dto.Usage{PromptTokens: 10, TotalTokens: 10}
	relayErr := types.NewError(errors.New("upstream rejected request"), types.ErrorCodeBadResponseStatusCode)

	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	assert.True(t, info.AttemptActualQuotaKnown)
	assert.Zero(t, info.AttemptActualQuota)
	assert.True(t, info.AttemptUsageBillingEvidence)
	assert.Equal(t, types.AttemptFinancialOutcomeBillable, ClassifyAttempt(info, relayErr))
	assert.Zero(t, chargedAttemptQuota(info))
}

func TestCaptureAttemptUsageFallsBackToEstimateForUnknownCostType(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		AttemptDispatched: true,
		StartTime:         time.Now(),
		UsingGroup:        "premium",
		AutoRoute: relaycommon.AutoRouteState{
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "premium", EstimatedQuota: 100},
			},
		},
	}
	usage := &dto.Usage{Cost: map[string]any{"amount": 0.002}}

	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	assert.False(t, info.AttemptActualQuotaKnown)
	assert.True(t, info.AttemptUsageBillingEvidence)
	assert.Equal(t, 100, chargedAttemptQuota(info))
}

func TestCaptureAttemptUsageFallsBackToEstimateWithoutTokenBreakdown(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		AttemptDispatched: true,
		StartTime:         time.Now(),
		UsingGroup:        "premium",
		AutoRoute: relaycommon.AutoRouteState{
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "premium", EstimatedQuota: 100},
			},
		},
	}
	usage := &dto.Usage{TotalTokens: 10}

	require.True(t, CaptureAttemptUsageQuota(ctx, info, usage))
	assert.False(t, info.AttemptActualQuotaKnown)
	assert.True(t, info.AttemptUsageBillingEvidence)
	assert.Equal(t, 100, chargedAttemptQuota(info))
}

func TestPreWssConsumeQuotaExtendsReserveWithoutIncrementalCharge(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		OriginModelName: "realtime-model",
		PriceData: types.PriceData{
			ModelRatio: 1,
			GroupRatioInfo: types.GroupRatioInfo{
				GroupRatio: 1,
			},
		},
	}
	settler := &autoTestSettler{info: info, preConsumed: 5}
	info.Billing = settler
	usage := &dto.RealtimeUsage{
		TotalTokens: 10,
		InputTokens: 10,
		InputTokenDetails: dto.InputTokenDetails{
			TextTokens: 10,
		},
	}

	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))

	assert.Equal(t, 10, info.RealtimeConsumedQuota)
	assert.Equal(t, 10, settler.reserve)
	assert.Equal(t, 10, info.AttemptActualQuota)
	assert.True(t, info.AttemptUsageBillingEvidence)
}

func TestPreWssConsumeQuotaUsesCumulativeTieredUsage(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := makeRelayInfo(`tier("default", p + cr * 0.1)`, 2, 0, 0)
	settler := &autoTestSettler{info: info}
	info.Billing = settler

	first := &dto.RealtimeUsage{
		InputTokens: 100,
		InputTokenDetails: dto.InputTokenDetails{
			CachedTokens: 40,
		},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, info, first))
	// (60 + 40*0.1) / 1M * 500K * 2 = 64 quota.
	require.Equal(t, 64, info.RealtimeConsumedQuota)
	require.Equal(t, 64, settler.reserve)

	second := &dto.RealtimeUsage{
		InputTokens: 100,
		InputTokenDetails: dto.InputTokenDetails{
			CachedTokens: 20,
		},
	}
	require.NoError(t, PreWssConsumeQuota(ctx, info, second))
	// Cumulative params are P=140 and CR=60, not two independently rounded chunks.
	// (140 + 60*0.1) / 1M * 500K * 2 = 146 quota.
	require.Equal(t, 146, info.RealtimeConsumedQuota)
	require.Equal(t, 146, settler.reserve)
}

func TestPreWssConsumeQuotaAccumulatesReasoningOutput(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := makeRelayInfo(`tier("default", rt * 10)`, 1, 0, 0)
	settler := &autoTestSettler{info: info}
	info.Billing = settler

	usage := &dto.RealtimeUsage{
		OutputTokens: 10,
		OutputTokenDetails: dto.OutputTokenDetails{
			ReasoningTokens:        10,
			ReasoningTokensPresent: true,
		},
	}

	require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	require.Equal(t, 10.0, info.RealtimeTieredTokenParams.RT)
	require.Equal(t, 50, info.RealtimeConsumedQuota)
	require.Equal(t, 50, settler.reserve)
}

func TestPreWssConsumeQuotaPreservesMissingReasoningFallbackAcrossChunks(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := makeRelayInfo(`tier("default", c * 10 + rt * 2)`, 1, 0, 0)
	settler := &autoTestSettler{info: info}
	info.Billing = settler

	for range 2 {
		usage := &dto.RealtimeUsage{OutputTokens: 10}
		require.NoError(t, PreWssConsumeQuota(ctx, info, usage))
	}

	require.True(t, info.RealtimeTieredTokenParams.ReasoningTokensFallback)
	require.Equal(t, 20.0, info.RealtimeTieredTokenParams.ReasoningTokensUnknown)
	// Missing reasoning counters must use the higher c*10 rate for 20 tokens.
	require.Equal(t, 100, info.RealtimeConsumedQuota)
}

func TestPreWssConsumeQuotaKeepsKnownReasoningWhenLaterChunkOmitsCounter(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := makeRelayInfo(`tier("default", c * 10 + rt * 2)`, 1, 0, 0)
	settler := &autoTestSettler{info: info}
	info.Billing = settler

	require.NoError(t, PreWssConsumeQuota(ctx, info, &dto.RealtimeUsage{
		OutputTokens: 10,
		OutputTokenDetails: dto.OutputTokenDetails{
			ReasoningTokens:        5,
			ReasoningTokensPresent: true,
		},
	}))
	require.NoError(t, PreWssConsumeQuota(ctx, info, &dto.RealtimeUsage{OutputTokens: 10}))

	// Known C=5/RT=5 plus unknown output=10; worst case is C=15, RT=5.
	require.Equal(t, 5.0, info.RealtimeTieredTokenParams.C)
	require.Equal(t, 15.0, info.RealtimeTieredTokenParams.RT)
	require.Equal(t, 10.0, info.RealtimeTieredTokenParams.ReasoningTokensUnknown)
	require.Equal(t, 80, info.RealtimeConsumedQuota)
}

func TestChargedAttemptQuotaDoesNotReplaceKnownZeroWithEstimate(t *testing.T) {
	info := &relaycommon.RelayInfo{
		AttemptActualQuotaKnown: true,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "premium", EstimatedQuota: 100},
			},
		},
		UsingGroup: "premium",
	}

	assert.Zero(t, chargedAttemptQuota(info))

	info.MarkAttemptBillingEvidence()
	assert.Equal(t, 100, chargedAttemptQuota(info))
}

func TestDescribeAutoReserveFailureExplainsHowToReduceReserve(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("token_quota", 10)
	info := &relaycommon.RelayInfo{
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota:           50,
			ReserveGroup:            "premium",
			HasTokenReserveEstimate: true,
			EstimatedInputTokens:    1432,
			MaxOutputTokens:         4096,
		},
	}
	cause := types.NewError(errors.New("token quota exceeded"), types.ErrorCodePreConsumeTokenQuotaFailed)

	failure := DescribeAutoReserveFailure(ctx, info, cause)

	assert.Equal(t, 50, failure.Required)
	assert.Equal(t, 10, failure.Available)
	assert.Equal(t, "premium", failure.Group)
	assert.Equal(t, "api_key_limit", failure.Source)
	assert.Equal(t, "Auto needs $0.000100 to reserve this request in the most expensive available group \"premium\", but only $0.000020 is available. Reserve estimate: 1,432 input tokens + up to 4,096 output tokens. No funds were charged. Increase this API key's quota limit or keep only cheaper Auto groups.", failure.Message)
}

func TestDescribeAutoReserveFailureOmitsTokenEstimateForPerCallReserve(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("token_quota", 10)
	info := &relaycommon.RelayInfo{
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 50,
			ReserveGroup:  "premium",
		},
	}
	cause := types.NewError(errors.New("token quota exceeded"), types.ErrorCodePreConsumeTokenQuotaFailed)

	failure := DescribeAutoReserveFailure(ctx, info, cause)

	assert.NotContains(t, failure.Message, "Reserve estimate:")
	assert.NotContains(t, failure.Message, "0 input tokens")
	assert.NotContains(t, failure.Message, "0 output tokens")
}

func TestDescribeAutoReserveFailureUsesSelectedSubscriptionSource(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	info := &relaycommon.RelayInfo{
		UserId:        999999,
		BillingSource: BillingSourceSubscription,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 50,
			ReserveGroup:  "premium",
		},
	}
	cause := types.NewErrorWithStatusCode(
		errors.New("订阅额度不足或未配置订阅"),
		types.ErrorCodeInsufficientUserQuota,
		http.StatusForbidden,
	)

	failure := DescribeAutoReserveFailure(ctx, info, cause)

	assert.Equal(t, BillingSourceSubscription, failure.Source)
	assert.Zero(t, failure.Available)
}

func TestBillingSessionSettlementReleasesReserveRemainder(t *testing.T) {
	funding := &autoTestFunding{source: BillingSourceWallet}
	info := &relaycommon.RelayInfo{
		IsPlayground: true,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
		},
	}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		tokenConsumed:    100,
	}

	require.NoError(t, session.Settle(40))

	assert.Equal(t, -60, funding.settleDelta)
	assert.Equal(t, 60, info.AutoRoute.ReleasedQuota)
	assert.Equal(t, 40, info.SettledQuota)
	assert.True(t, info.BillingSettled)
}

func TestSettleSuccessfulBillingLogsHeldReserveWhenFundingDoesNotCommit(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	settleErr := errors.New("database unavailable")
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 80,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
			ReleasedQuota: 20,
		},
	}
	info.Billing = &autoTestSettler{
		info:        info,
		preConsumed: 100,
		settleErr:   settleErr,
	}

	outcome := SettleSuccessfulBilling(ctx, info, 40)

	require.ErrorIs(t, outcome.Err, settleErr)
	assert.Equal(t, 100, outcome.Quota)
	assert.True(t, outcome.HeldReserve)
	assert.False(t, outcome.Partial)
	assert.Zero(t, info.AutoRoute.ReleasedQuota)
	assert.True(t, info.ChargedOnError)
}

func TestSettleSuccessfulBillingKeepsActualQuotaAfterPartialSettlement(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	tokenErr := errors.New("token quota update failed")
	info := &relaycommon.RelayInfo{
		FinalPreConsumedQuota: 100,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 100,
		},
	}
	info.Billing = &autoTestSettler{
		info:        info,
		preConsumed: 100,
		settleErr:   tokenErr,
		partial:     true,
	}

	outcome := SettleSuccessfulBilling(ctx, info, 40)

	require.ErrorIs(t, outcome.Err, tokenErr)
	assert.Equal(t, 40, outcome.Quota)
	assert.False(t, outcome.HeldReserve)
	assert.True(t, outcome.Partial)
	assert.Equal(t, 40, info.SettledQuota)
}

func TestBillingSessionSubscriptionOverageDebitsWallet(t *testing.T) {
	funding := &autoTestFunding{
		source:    BillingSourceSubscription,
		settleErr: fmt.Errorf("limit: %w", model.ErrSubscriptionQuotaExceeded),
	}
	info := &relaycommon.RelayInfo{
		IsPlayground:  true,
		BillingSource: BillingSourceSubscription,
	}
	walletDelta := 0
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		walletOverageDebit: func(delta int) error {
			walletDelta = delta
			return nil
		},
	}

	require.NoError(t, session.Settle(135))

	assert.Equal(t, 35, walletDelta)
	assert.Equal(t, BillingSourceWallet, info.BillingOverageSource)
	assert.Equal(t, 35, info.BillingOverageQuota)
	assert.Equal(t, 135, info.SettledQuota)

	other := map[string]interface{}{}
	appendBillingInfo(info, other)
	assert.Equal(t, 35, other["wallet_quota_deducted"])
}

func TestBillingSessionSubscriptionOnlyStillDebitsPostDispatchOverage(t *testing.T) {
	funding := &autoTestFunding{
		source:    BillingSourceSubscription,
		settleErr: fmt.Errorf("limit: %w", model.ErrSubscriptionQuotaExceeded),
	}
	info := &relaycommon.RelayInfo{IsPlayground: true}
	walletCalls := 0
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		walletOverageDebit: func(int) error {
			walletCalls++
			return nil
		},
	}

	err := session.Settle(135)

	require.NoError(t, err)
	assert.Equal(t, 1, walletCalls)
	assert.True(t, info.BillingSettled)
	assert.Equal(t, BillingSourceWallet, info.BillingOverageSource)
	assert.Equal(t, 35, info.BillingOverageQuota)
}

func TestBillingSessionSubscriptionDatabaseErrorDoesNotDebitWallet(t *testing.T) {
	dbErr := errors.New("database unavailable")
	funding := &autoTestFunding{
		source:    BillingSourceSubscription,
		settleErr: dbErr,
	}
	info := &relaycommon.RelayInfo{IsPlayground: true}
	walletCalls := 0
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		walletOverageDebit: func(int) error {
			walletCalls++
			return nil
		},
	}

	err := session.Settle(135)

	require.ErrorIs(t, err, dbErr)
	assert.Zero(t, walletCalls)
	assert.False(t, info.BillingSettled)
}

func TestBillingSessionRefundIsSynchronous(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	funding := &autoTestFunding{source: BillingSourceWallet}
	info := &relaycommon.RelayInfo{
		IsPlayground: true,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 75,
		},
	}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 75,
		tokenConsumed:    75,
	}

	require.NoError(t, session.Refund(ctx))

	assert.Equal(t, 1, funding.refundCalls)
	assert.Equal(t, 75, info.AutoRoute.ReleasedQuota)
	assert.Equal(t, 0, info.SettledQuota)
}

func TestBillingSessionRefundFailureKeepsReserveHeld(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	funding := &autoTestFunding{
		source:    BillingSourceWallet,
		refundErr: errors.New("database unavailable"),
	}
	info := &relaycommon.RelayInfo{
		IsPlayground: true,
		AutoRoute: relaycommon.AutoRouteState{
			ReservedQuota: 75,
		},
	}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 75,
		tokenConsumed:    75,
	}

	require.Error(t, session.Refund(ctx))
	assert.Zero(t, info.AutoRoute.ReleasedQuota)
	assert.False(t, info.BillingSettled)

	funding.refundErr = nil
	require.NoError(t, session.Refund(ctx))
	assert.Equal(t, 75, info.AutoRoute.ReleasedQuota)
	assert.True(t, info.BillingSettled)
}

func TestBillingSessionPartialRefundReportsOnlyTokenQuotaHeld(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	tokenErr := errors.New("token database unavailable")
	funding := &autoTestFunding{source: BillingSourceWallet}
	info := &relaycommon.RelayInfo{}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 75,
		tokenConsumed:    75,
		tokenQuotaRefund: func(int) error {
			return tokenErr
		},
	}

	require.ErrorIs(t, session.Refund(ctx), tokenErr)
	financialHold, tokenHold := session.RemainingHolds()
	assert.Zero(t, financialHold)
	assert.Equal(t, 75, tokenHold)
	assert.Equal(t, 1, funding.refundCalls)

	session.tokenQuotaRefund = func(int) error { return nil }
	require.NoError(t, session.Refund(ctx))
	assert.Equal(t, 1, funding.refundCalls)
}

func TestBillingSessionPartialSubscriptionRefundReportsOnlyExtraReserveHeld(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	extraErr := errors.New("subscription database unavailable")
	funding := &autoTestFunding{source: BillingSourceSubscription}
	info := &relaycommon.RelayInfo{IsPlayground: true, SubscriptionId: 1}
	session := &BillingSession{
		relayInfo:        info,
		funding:          funding,
		preConsumedQuota: 100,
		extraReserved:    40,
		extraReserveRefund: func(int) error {
			return extraErr
		},
	}

	require.ErrorIs(t, session.Refund(ctx), extraErr)
	financialHold, tokenHold := session.RemainingHolds()
	assert.Equal(t, 40, financialHold)
	assert.Zero(t, tokenHold)
	assert.Equal(t, 1, funding.refundCalls)
}

func TestBillingSessionReserveKeepsFundingHoldWhenTokenAndRollbackFail(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	tokenErr := errors.New("token reserve failed")
	rollbackErr := errors.New("funding rollback failed")
	funding := &autoTestFunding{source: BillingSourceSubscription}
	info := &relaycommon.RelayInfo{SubscriptionId: 1}
	extraRefunded := 0
	session := &BillingSession{
		relayInfo: info,
		funding:   funding,
		fundingReserve: func(int) error {
			return nil
		},
		tokenQuotaReserve: func(int) error {
			return tokenErr
		},
		fundingRollback: func(int) error {
			return rollbackErr
		},
		extraReserveRefund: func(quota int) error {
			extraRefunded += quota
			return nil
		},
	}

	err := session.Reserve(40)

	require.ErrorIs(t, err, tokenErr)
	require.ErrorIs(t, err, rollbackErr)
	financialHold, tokenHold := session.RemainingHolds()
	assert.Equal(t, 40, financialHold)
	assert.Zero(t, tokenHold)
	assert.True(t, session.NeedsRefund())

	info.SubscriptionId = 1
	require.NoError(t, session.Refund(ctx))
	assert.Equal(t, 40, extraRefunded)
	financialHold, tokenHold = session.RemainingHolds()
	assert.Zero(t, financialHold)
	assert.Zero(t, tokenHold)
}

func TestBillingSessionUnresolvedReserveSettlesWithoutSecondFundingDebit(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	tokenErr := errors.New("token reserve failed")
	rollbackErr := errors.New("funding rollback failed")
	funding := &autoTestFunding{source: BillingSourceWallet}
	info := &relaycommon.RelayInfo{}
	session := &BillingSession{
		relayInfo: info,
		funding:   funding,
		fundingReserve: func(int) error {
			return nil
		},
		tokenQuotaReserve: func(int) error {
			return tokenErr
		},
		fundingRollback: func(int) error {
			return rollbackErr
		},
	}
	info.Billing = session

	require.Error(t, session.Reserve(40))
	info.IsPlayground = true
	outcome := SettleSuccessfulBilling(ctx, info, 40)

	require.ErrorIs(t, outcome.Err, tokenErr)
	require.ErrorIs(t, outcome.Err, rollbackErr)
	assert.True(t, outcome.Partial)
	assert.False(t, outcome.HeldReserve)
	assert.Equal(t, 40, outcome.Quota)
	assert.Zero(t, funding.settleCalls)
	assert.True(t, info.BillingSettled)
	assert.Equal(t, 40, info.SettledQuota)
}

func TestBillingSessionUnresolvedReserveRejectsSecondReserve(t *testing.T) {
	tokenErr := errors.New("token reserve failed")
	rollbackErr := errors.New("funding rollback failed")
	fundingReserveCalls := 0
	session := &BillingSession{
		relayInfo: &relaycommon.RelayInfo{},
		funding:   &autoTestFunding{source: BillingSourceWallet},
		fundingReserve: func(int) error {
			fundingReserveCalls++
			return nil
		},
		tokenQuotaReserve: func(int) error {
			return tokenErr
		},
		fundingRollback: func(int) error {
			return rollbackErr
		},
	}

	require.Error(t, session.Reserve(40))
	secondErr := session.Reserve(80)

	require.ErrorIs(t, secondErr, tokenErr)
	require.ErrorIs(t, secondErr, rollbackErr)
	assert.Equal(t, 1, fundingReserveCalls)
	assert.Equal(t, 40, session.GetPreConsumedQuota())
}

func TestBillingSessionPreConsumeRollbackFailureForcesHeldTokenAudit(t *testing.T) {
	require.NoError(t, model.LOG_DB.Exec("DELETE FROM logs").Error)
	originalLogConsumeEnabled := common.LogConsumeEnabled
	common.LogConsumeEnabled = false
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	fundingErr := errors.New("wallet reserve failed")
	refundErr := errors.New("token refund failed")
	info := &relaycommon.RelayInfo{
		UserId:          9101,
		TokenId:         9102,
		TokenKey:        "sk-held-token",
		UsingGroup:      "default",
		OriginModelName: "held-token-model",
		ForcePreConsume: true,
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: 9103},
	}
	session := &BillingSession{
		relayInfo: info,
		funding: &autoTestFunding{
			source:        BillingSourceWallet,
			preConsumeErr: fundingErr,
		},
		tokenQuotaReserve: func(int) error {
			return nil
		},
		tokenQuotaRefund: func(int) error {
			return refundErr
		},
	}

	apiErr := session.preConsume(ctx, 50)

	require.NotNil(t, apiErr)
	require.ErrorIs(t, apiErr.Err, fundingErr)
	require.ErrorIs(t, apiErr.Err, refundErr)
	assert.True(t, session.NeedsRefund())
	var log model.Log
	require.NoError(t, model.LOG_DB.Where("model_name = ?", info.OriginModelName).Order("id desc").First(&log).Error)
	assert.Zero(t, log.Quota)
	var other map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(log.Other), &other))
	assert.Equal(t, float64(50), other["held_token_quota"])
	assert.Equal(t, true, other["token_quota_held"])
	assert.Equal(t, true, other["refund_failed"])
	assert.Equal(t, string(types.AttemptFinancialOutcomeNonBillable), other["financial_outcome"])
}

func TestAppendAutoRoutingInfoReportsFrozenPriceComparison(t *testing.T) {
	info := &relaycommon.RelayInfo{
		AutoRoute: relaycommon.AutoRouteState{
			InitialGroup: "budget",
			UsedGroup:    "premium",
			Candidates: []relaycommon.AutoRouteCandidate{
				{Group: "budget", Ratio: 0.5},
				{Group: "premium", Ratio: 2},
			},
		},
	}
	other := map[string]interface{}{}

	AppendAutoRoutingInfo(info, other)

	assert.Equal(t, 0.5, other["auto_initial_ratio"])
	assert.Equal(t, float64(2), other["auto_used_ratio"])
	assert.Equal(t, true, other["auto_used_more_expensive"])
}

func TestBillingSessionForcePreConsumeDisablesTrustBypass(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Set("token_quota", common.GetTrustQuota()+1)
	info := &relaycommon.RelayInfo{
		TokenUnlimited: true,
		UserQuota:      common.GetTrustQuota() + 1,
	}
	session := &BillingSession{
		relayInfo: info,
		funding:   &autoTestFunding{source: BillingSourceWallet},
	}

	assert.True(t, session.shouldTrust(ctx))
	info.ForcePreConsume = true
	assert.False(t, session.shouldTrust(ctx))
}

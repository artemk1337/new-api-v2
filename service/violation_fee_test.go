package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type violationRefundFailureSettler struct {
	refundCalls int
}

func (*violationRefundFailureSettler) Settle(int) error { return nil }
func (s *violationRefundFailureSettler) Refund(*gin.Context) error {
	s.refundCalls++
	return errors.New("refund unavailable")
}
func (*violationRefundFailureSettler) NeedsRefund() bool        { return true }
func (*violationRefundFailureSettler) GetPreConsumedQuota() int { return 100 }
func (*violationRefundFailureSettler) Reserve(int) error        { return nil }

func TestNormalizeViolationFeeErrorPreservesFinancialOutcome(t *testing.T) {
	apiErr := types.NewError(
		errors.New(CSAMViolationMarker),
		types.ErrorCodeBadResponse,
		types.ErrOptionWithFinancialOutcome(types.AttemptFinancialOutcomeAmbiguous),
	)

	normalized := NormalizeViolationFeeError(apiErr)

	require.Equal(t, types.ErrorCodeViolationFeeGrokCSAM, normalized.GetErrorCode())
	assert.Equal(t, types.AttemptFinancialOutcomeAmbiguous, normalized.GetFinancialOutcome())
}

func TestShouldRefundBaseForViolationRegardlessOfUpstreamBillingEvidence(t *testing.T) {
	apiErr := types.NewError(errors.New(CSAMViolationMarker), types.ErrorCodeBadResponse)
	info := &relaycommon.RelayInfo{AttemptDispatched: true}

	assert.True(t, ShouldRefundBaseForViolation(info, apiErr))

	info.AttemptUsageBillingEvidence = true
	info.AttemptActualQuota = 42
	info.AttemptTaskAccepted = true
	info.AttemptHasBillingEvidence = true
	assert.True(t, ShouldRefundBaseForViolation(info, apiErr))

	nonViolation := types.NewError(errors.New("upstream error"), types.ErrorCodeBadResponse)
	assert.False(t, ShouldRefundBaseForViolation(info, nonViolation))
}

func TestRefundBaseFailureDoesNotChargeViolationFee(t *testing.T) {
	truncate(t)

	settler := &violationRefundFailureSettler{}
	info := &relaycommon.RelayInfo{
		UserId:      91,
		TokenId:     91,
		Billing:     settler,
		ChannelMeta: &relaycommon.ChannelMeta{},
		StartTime:   time.Now(),
		PriceData:   types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	apiErr := types.NewError(errors.New(CSAMViolationMarker), types.ErrorCodeViolationFeeGrokCSAM)

	charged, err := RefundBaseAndChargeViolationFeeIfNeeded(ctx, info, apiErr)

	require.Error(t, err)
	assert.False(t, charged)
	assert.Equal(t, 1, settler.refundCalls)
	var count int64
	require.NoError(t, model.DB.Model(&model.BillingOutbox{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestViolationFeeRetriesLogDeliveryFromDurableIntent(t *testing.T) {
	truncate(t)

	settings := model_setting.GetGrokSettings()
	originalSettings := *settings
	t.Cleanup(func() { *settings = originalSettings })
	settings.ViolationDeductionEnabled = true
	settings.ViolationDeductionAmount = 0.05
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, 1)
	require.Positive(t, feeQuota)

	const userID, tokenID, channelID = 92, 92, 92
	seedUser(t, userID, 10000)
	seedToken(t, tokenID, userID, "sk-violation-outbox", 10000)
	seedChannel(t, channelID)

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(common.RequestIdKey, "req-violation-outbox")
	ctx.Set("username", "test_user")
	ctx.Set("token_name", "test_token")
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         tokenID,
		TokenKey:        "sk-violation-outbox",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelId: channelID},
		RequestId:       "req-violation-outbox",
		OriginModelName: "test-model",
		UsingGroup:      "default",
		BillingSource:   BillingSourceWallet,
		StartTime:       time.Now(),
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	apiErr := types.NewError(errors.New(CSAMViolationMarker), types.ErrorCodeViolationFeeGrokCSAM)

	originalLogDB := model.LOG_DB
	t.Cleanup(func() { model.LOG_DB = originalLogDB })
	missingLogDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.LOG_DB = missingLogDB

	charged, chargeErr := ChargeViolationFeeIfNeeded(ctx, info, apiErr)
	require.Error(t, chargeErr)
	assert.True(t, charged)
	assert.Equal(t, 10000-feeQuota, getUserQuota(t, userID))
	assert.Equal(t, 10000-feeQuota, getTokenRemainQuota(t, tokenID))

	var event model.BillingOutbox
	require.NoError(t, model.DB.Where("kind = ?", model.BillingOutboxKindLog).First(&event).Error)
	model.LOG_DB = originalLogDB
	require.NoError(t, model.DB.Model(&model.BillingOutbox{}).Where("id = ?", event.ID).Update("next_attempt_at", 0).Error)
	result := model.ProcessBillingOutbox(context.Background(), 10)
	assert.Equal(t, 1, result.Processed)

	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id = ?", event.EventID).First(&stored).Error)
	assert.Equal(t, feeQuota, stored.Quota)
	var other map[string]interface{}
	require.NoError(t, common.UnmarshalJsonStr(stored.Other, &other))
	assert.Equal(t, true, other["violation_fee"])
	_, settlementPending := other["settlement_pending"]
	assert.False(t, settlementPending)
}

func TestViolationFeeRollsBackFundingWhenTokenDebitFails(t *testing.T) {
	truncate(t)

	settings := model_setting.GetGrokSettings()
	originalSettings := *settings
	t.Cleanup(func() { *settings = originalSettings })
	settings.ViolationDeductionEnabled = true
	settings.ViolationDeductionAmount = 0.05
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, 1)
	require.Positive(t, feeQuota)

	const userID = 93
	seedUser(t, userID, 10000)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Set(common.RequestIdKey, "req-violation-rollback")
	info := &relaycommon.RelayInfo{
		UserId:          userID,
		TokenId:         99999,
		TokenKey:        "missing-token",
		ChannelMeta:     &relaycommon.ChannelMeta{},
		RequestId:       "req-violation-rollback",
		OriginModelName: "test-model",
		UsingGroup:      "default",
		BillingSource:   BillingSourceWallet,
		StartTime:       time.Now(),
		PriceData:       types.PriceData{GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1}},
	}
	apiErr := types.NewError(errors.New(CSAMViolationMarker), types.ErrorCodeViolationFeeGrokCSAM)

	charged, chargeErr := ChargeViolationFeeIfNeeded(ctx, info, apiErr)
	require.Error(t, chargeErr)
	assert.False(t, charged)
	assert.Equal(t, 10000, getUserQuota(t, userID))

	var stored model.Log
	require.NoError(t, model.LOG_DB.Where("request_id <> ''").Order("id desc").First(&stored).Error)
	assert.Zero(t, stored.Quota)
	assert.Equal(t, "Violation fee was not charged", stored.Content)
}

package service

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/shopspring/decimal"

	"github.com/gin-gonic/gin"
)

const (
	ViolationFeeCodePrefix     = "violation_fee."
	CSAMViolationMarker        = "Failed check: SAFETY_CHECK_TYPE"
	ContentViolatesUsageMarker = "Content violates usage guidelines"
)

func IsViolationFeeCode(code types.ErrorCode) bool {
	return strings.HasPrefix(string(code), ViolationFeeCodePrefix)
}

func HasCSAMViolationMarker(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if strings.Contains(err.Error(), CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker) {
		return true
	}
	msg := err.ToOpenAIError().Message
	return strings.Contains(msg, CSAMViolationMarker) || strings.Contains(err.Error(), ContentViolatesUsageMarker)
}

func WrapAsViolationFeeGrokCSAM(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}
	oai := err.ToOpenAIError()
	oai.Type = string(types.ErrorCodeViolationFeeGrokCSAM)
	oai.Code = string(types.ErrorCodeViolationFeeGrokCSAM)
	options := []types.NewAPIErrorOptions{types.ErrOptionWithSkipRetry()}
	if outcome := err.GetFinancialOutcome(); outcome != types.AttemptFinancialOutcomeUnknown {
		options = append(options, types.ErrOptionWithFinancialOutcome(outcome))
	}
	return types.WithOpenAIError(oai, err.StatusCode, options...)
}

// NormalizeViolationFeeError ensures:
// - if the CSAM marker is present, error.code is set to a stable violation-fee code and skip-retry is enabled.
// - if error.code already has the violation-fee prefix, skip-retry is enabled.
//
// It must be called before retry decision logic.
func NormalizeViolationFeeError(err *types.NewAPIError) *types.NewAPIError {
	if err == nil {
		return nil
	}

	if HasCSAMViolationMarker(err) {
		return WrapAsViolationFeeGrokCSAM(err)
	}

	if IsViolationFeeCode(err.GetErrorCode()) {
		oai := err.ToOpenAIError()
		options := []types.NewAPIErrorOptions{types.ErrOptionWithSkipRetry()}
		if outcome := err.GetFinancialOutcome(); outcome != types.AttemptFinancialOutcomeUnknown {
			options = append(options, types.ErrOptionWithFinancialOutcome(outcome))
		}
		return types.WithOpenAIError(oai, err.StatusCode, options...)
	}

	return err
}

func ShouldRefundBaseForViolation(info *relaycommon.RelayInfo, err *types.NewAPIError) bool {
	return info != nil && shouldChargeViolationFee(err)
}

func shouldChargeViolationFee(err *types.NewAPIError) bool {
	if err == nil {
		return false
	}
	if err.GetErrorCode() == types.ErrorCodeViolationFeeGrokCSAM {
		return true
	}
	// In case some callers didn't normalize, keep a safety net.
	return HasCSAMViolationMarker(err)
}

func calcViolationFeeQuota(amount, groupRatio float64) int {
	if amount <= 0 {
		return 0
	}
	if groupRatio <= 0 {
		return 0
	}
	quota := decimal.NewFromFloat(amount).
		Mul(decimal.NewFromFloat(common.GetQuotaPerUnit())).
		Mul(decimal.NewFromFloat(groupRatio)).
		Round(0).
		IntPart()
	if quota <= 0 {
		return 0
	}
	return int(quota)
}

// ChargeViolationFeeIfNeeded charges the violation fee after the base request reserve is refunded.
// It uses Grok fee settings as the fee policy.
func ChargeViolationFeeIfNeeded(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, apiErr *types.NewAPIError) (bool, error) {
	if ctx == nil || relayInfo == nil || apiErr == nil {
		return false, nil
	}
	//if relayInfo.IsPlayground {
	//	return false
	//}
	if !shouldChargeViolationFee(apiErr) {
		return false, nil
	}

	settings := model_setting.GetGrokSettings()
	if settings == nil || !settings.ViolationDeductionEnabled {
		return false, nil
	}

	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	feeQuota := calcViolationFeeQuota(settings.ViolationDeductionAmount, groupRatio)
	if feeQuota <= 0 {
		return false, nil
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	tokenName := ctx.GetString("token_name")
	oai := apiErr.ToOpenAIError()
	other := map[string]any{
		"violation_fee":        true,
		"violation_fee_code":   string(types.ErrorCodeViolationFeeGrokCSAM),
		"fee_quota":            feeQuota,
		"base_amount":          settings.ViolationDeductionAmount,
		"group_ratio":          groupRatio,
		"status_code":          apiErr.StatusCode,
		"upstream_error_type":  oai.Type,
		"upstream_error_code":  fmt.Sprintf("%v", oai.Code),
		"violation_fee_marker": CSAMViolationMarker,
	}
	requestID := relayInfo.RequestId
	if requestID == "" {
		requestID = ctx.GetString(common.RequestIdKey)
	}
	if requestID == "" {
		requestID = common.NewRequestId()
	}
	eventHash := sha256.Sum256([]byte(fmt.Sprintf("violation_fee:%s:%d", requestID, relayInfo.TokenId)))
	eventID := fmt.Sprintf("%x", eventHash)
	pendingOther := make(map[string]any, len(other)+1)
	for key, value := range other {
		pendingOther[key] = value
	}
	pendingOther["settlement_pending"] = true
	pendingLog := model.RecordConsumeLogParams{
		ChannelId:      relayInfo.ChannelId,
		ModelName:      relayInfo.OriginModelName,
		TokenName:      tokenName,
		Quota:          0,
		Content:        "Violation fee settlement pending",
		TokenId:        relayInfo.TokenId,
		UseTimeSeconds: int(useTimeSeconds),
		IsStream:       relayInfo.IsStream,
		Group:          relayInfo.UsingGroup,
		Other:          pendingOther,
		Force:          true,
	}
	if !model.HasBillingOutboxTable() {
		return chargeViolationFeeLegacy(ctx, relayInfo, feeQuota, pendingLog, other)
	}
	if err := model.StageConsumeLogOutboxIntent(ctx, relayInfo.UserId, eventID, pendingLog); err != nil {
		return false, fmt.Errorf("persist violation fee billing intent: %w", err)
	}

	finalLog := model.RecordConsumeLogParams{
		ChannelId:      relayInfo.ChannelId,
		ModelName:      relayInfo.OriginModelName,
		TokenName:      tokenName,
		Quota:          feeQuota,
		Content:        "Violation fee charged",
		TokenId:        relayInfo.TokenId,
		UseTimeSeconds: int(useTimeSeconds),
		IsStream:       relayInfo.IsStream,
		Group:          relayInfo.UsingGroup,
		Other:          other,
		Force:          true,
	}
	commitErr := model.CommitViolationFeeWithOutbox(ctx, eventID, model.ViolationFeeCommitParams{
		UserID:          relayInfo.UserId,
		TokenID:         relayInfo.TokenId,
		TokenKey:        relayInfo.TokenKey,
		Quota:           feeQuota,
		SubscriptionID:  relayInfo.SubscriptionId,
		UseSubscription: relayInfo.BillingSource == BillingSourceSubscription,
		SkipTokenQuota:  relayInfo.IsPlayground,
	}, finalLog)
	charged := commitErr == nil
	logQuota := feeQuota
	content := "Violation fee charged"
	if !charged {
		logger.LogError(ctx, fmt.Sprintf("failed to charge violation fee: %s", commitErr.Error()))
		logQuota = 0
		content = "Violation fee was not charged"
		other["charge_failed"] = true
		other["settlement_error"] = commitErr.Error()
	}

	if charged {
		if relayInfo.BillingSource == BillingSourceSubscription {
			relayInfo.SubscriptionPostDelta += int64(feeQuota)
			checkAndSendSubscriptionQuotaNotify(relayInfo)
		} else {
			checkAndSendQuotaNotify(relayInfo, feeQuota, 0)
		}
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, feeQuota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, feeQuota)
	}

	finalLog = model.RecordConsumeLogParams{
		ChannelId:      relayInfo.ChannelId,
		ModelName:      relayInfo.OriginModelName,
		TokenName:      tokenName,
		Quota:          logQuota,
		Content:        content,
		TokenId:        relayInfo.TokenId,
		UseTimeSeconds: int(useTimeSeconds),
		IsStream:       relayInfo.IsStream,
		Group:          relayInfo.UsingGroup,
		Other:          other,
		Force:          true,
	}
	var logErr error
	if !charged {
		logErr = model.UpsertConsumeLogOutboxIntent(ctx, relayInfo.UserId, eventID, finalLog)
	}
	if logErr == nil {
		logErr = model.DeliverBillingOutboxEvent(eventID)
	}

	return charged, errors.Join(commitErr, logErr)
}

func chargeViolationFeeLegacy(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, feeQuota int, logParams model.RecordConsumeLogParams, other map[string]any) (bool, error) {
	adjustment := PostConsumeQuotaWithResult(relayInfo, feeQuota, 0, true)
	charged := adjustment.FundingCommitted
	if charged {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, feeQuota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, feeQuota)
		logParams.Quota = feeQuota
		logParams.Content = "Violation fee charged"
	} else {
		logParams.Content = "Violation fee was not charged"
		other["charge_failed"] = true
	}
	if adjustment.Err != nil {
		other["settlement_error"] = adjustment.Err.Error()
		if adjustment.FundingCommitted != adjustment.TokenCommitted {
			other["settlement_partial"] = true
		}
	}
	logParams.Other = other
	return charged, errors.Join(adjustment.Err, model.RecordConsumeLog(ctx, relayInfo.UserId, logParams))
}

func RefundBaseAndChargeViolationFeeIfNeeded(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, apiErr *types.NewAPIError) (bool, error) {
	if err := RefundBilling(ctx, relayInfo, apiErr); err != nil {
		return false, err
	}
	return ChargeViolationFeeIfNeeded(ctx, relayInfo, apiErr)
}

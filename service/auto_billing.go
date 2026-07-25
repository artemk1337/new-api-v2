package service

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type AutoReserveFailure struct {
	Required  int
	Available int
	Group     string
	Source    string
	Message   string
}

func DescribeAutoReserveFailure(c *gin.Context, info *relaycommon.RelayInfo, cause *types.NewAPIError) AutoReserveFailure {
	failure := AutoReserveFailure{
		Required: info.AutoRoute.ReservedQuota,
		Group:    info.AutoRoute.ReserveGroup,
		Source:   BillingSourceWallet,
	}

	switch {
	case cause.GetErrorCode() == types.ErrorCodePreConsumeTokenQuotaFailed:
		failure.Source = "api_key_limit"
		if info.TokenUnlimited {
			failure.Available = failure.Required
		} else {
			failure.Available = c.GetInt("token_quota")
		}
	case info.BillingSource == BillingSourceSubscription:
		failure.Source = BillingSourceSubscription
		subscriptions, err := model.GetAllActiveUserSubscriptions(info.UserId)
		if err == nil {
			for _, summary := range subscriptions {
				if summary.Subscription == nil {
					continue
				}
				sub := summary.Subscription
				if sub.AmountTotal <= 0 {
					failure.Available = failure.Required
					break
				}
				failure.Available = max(failure.Available, int(sub.AmountTotal-sub.AmountUsed))
			}
		}
	default:
		quota, err := model.GetUserQuota(info.UserId, true)
		if err == nil {
			failure.Available = quota
		}
	}

	recommendation := "Top up your balance or restrict Auto to cheaper groups. Auto reserves quota using the most expensive group in the selected list."
	if len(common.GetContextKeyStringSlice(c, constant.ContextKeyTokenAutoGroupCandidates)) == 0 {
		recommendation = "Top up your balance or switch Auto to specific groups and keep only cheaper groups. Auto reserves quota using the most expensive available group."
	}
	if failure.Source == "api_key_limit" {
		recommendation += " You can also increase this API key's quota limit."
	}
	failure.Message = fmt.Sprintf(
		"Insufficient funds for Auto reserve: required %s for the most expensive available group %q, available %s. No funds were charged. %s",
		logger.FormatQuota(failure.Required),
		failure.Group,
		logger.FormatQuota(failure.Available),
		recommendation,
	)
	return failure
}

func RecordAutoReserveFailure(c *gin.Context, info *relaycommon.RelayInfo, failure AutoReserveFailure) {
	other := map[string]interface{}{
		"error_type":         types.ErrorTypeNewAPIError,
		"error_code":         types.ErrorCodeInsufficientAutoReserve,
		"required_quota":     failure.Required,
		"available_quota":    failure.Available,
		"reserve_group":      failure.Group,
		"limiting_source":    failure.Source,
		"reserved_quota":     0,
		"released_quota":     0,
		"auto_initial_group": info.AutoRoute.InitialGroup,
	}
	appendRequestPath(c, info, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = time.Now()
	}
	model.RecordErrorLog(
		c,
		info.UserId,
		0,
		info.OriginModelName,
		c.GetString("token_name"),
		failure.Message,
		info.TokenId,
		int(time.Since(startTime).Seconds()),
		info.IsStream,
		failure.Group,
		other,
	)
}

func ClassifyAttempt(info *relaycommon.RelayInfo, err *types.NewAPIError) types.AttemptFinancialOutcome {
	if info.AttemptTaskAccepted || info.AttemptHasBillingEvidence {
		return types.AttemptFinancialOutcomeBillable
	}
	if info.AttemptUsageBillingEvidence {
		return types.AttemptFinancialOutcomeBillable
	}
	if info.AttemptActualQuota > 0 {
		return types.AttemptFinancialOutcomeBillable
	}
	if err != nil {
		outcome := err.GetFinancialOutcome()
		if outcome == types.AttemptFinancialOutcomeBillable || outcome == types.AttemptFinancialOutcomeAmbiguous {
			return outcome
		}
	}
	if info.HasSendResponse() || info.SendResponseCount > 0 || info.ReceivedResponseCount > 0 {
		return types.AttemptFinancialOutcomeAmbiguous
	}
	if info.AttemptActualQuotaKnown {
		return types.AttemptFinancialOutcomeNonBillable
	}
	if err != nil {
		if outcome := err.GetFinancialOutcome(); outcome != types.AttemptFinancialOutcomeUnknown {
			return outcome
		}
	}
	if !info.AttemptDispatched {
		return types.AttemptFinancialOutcomeNonBillable
	}
	return types.AttemptFinancialOutcomeAmbiguous
}

func ClassifyTaskAttempt(info *relaycommon.RelayInfo, taskErr *dto.TaskError) types.AttemptFinancialOutcome {
	if info.AttemptTaskAccepted || info.AttemptHasBillingEvidence {
		return types.AttemptFinancialOutcomeBillable
	}
	if info.AttemptUsageBillingEvidence {
		return types.AttemptFinancialOutcomeBillable
	}
	if info.AttemptActualQuota > 0 {
		return types.AttemptFinancialOutcomeBillable
	}
	if taskErr != nil {
		outcome := taskErr.FinancialOutcome
		if outcome == types.AttemptFinancialOutcomeBillable || outcome == types.AttemptFinancialOutcomeAmbiguous {
			return outcome
		}
	}
	if info.HasSendResponse() || info.SendResponseCount > 0 || info.ReceivedResponseCount > 0 {
		return types.AttemptFinancialOutcomeAmbiguous
	}
	if info.AttemptActualQuotaKnown {
		return types.AttemptFinancialOutcomeNonBillable
	}
	if taskErr != nil && taskErr.FinancialOutcome != types.AttemptFinancialOutcomeUnknown {
		return taskErr.FinancialOutcome
	}
	if !info.AttemptDispatched {
		return types.AttemptFinancialOutcomeNonBillable
	}
	return types.AttemptFinancialOutcomeAmbiguous
}

func ShouldRetryAttempt(outcome types.AttemptFinancialOutcome, retryable bool, attemptsRemaining bool) bool {
	return outcome == types.AttemptFinancialOutcomeNonBillable && retryable && attemptsRemaining
}

func ShouldRetryAutoAttempt(outcome types.AttemptFinancialOutcome, dispatched bool, retryable bool, attemptsRemaining bool) bool {
	return outcome == types.AttemptFinancialOutcomeNonBillable &&
		attemptsRemaining &&
		(dispatched || retryable)
}

func CurrentAutoEstimate(info *relaycommon.RelayInfo) int {
	if candidate, ok := info.AutoRouteCandidate(info.UsingGroup); ok {
		return candidate.EstimatedQuota
	}
	if info.PriceData.QuotaToPreConsume > 0 {
		return info.PriceData.QuotaToPreConsume
	}
	if info.PriceData.Quota > 0 {
		return info.PriceData.Quota
	}
	return info.FinalPreConsumedQuota
}

func CaptureAttemptUsageQuota(c *gin.Context, info *relaycommon.RelayInfo, usage any) bool {
	typedUsage, ok := usage.(*dto.Usage)
	if !ok || typedUsage == nil {
		return false
	}
	hasTokenUsage := typedUsage.PromptTokens != 0 ||
		typedUsage.CompletionTokens != 0 ||
		typedUsage.TotalTokens != 0 ||
		typedUsage.InputTokens != 0 ||
		typedUsage.OutputTokens != 0
	hasTokenBreakdown := typedUsage.PromptTokens != 0 ||
		typedUsage.CompletionTokens != 0 ||
		typedUsage.InputTokens != 0 ||
		typedUsage.OutputTokens != 0
	cost, costKnown := numericUsageCost(typedUsage.Cost)
	if !hasTokenUsage && typedUsage.Cost == nil {
		return false
	}

	normalizedUsage := *typedUsage
	if normalizedUsage.PromptTokens == 0 && normalizedUsage.CompletionTokens == 0 {
		normalizedUsage.PromptTokens = normalizedUsage.InputTokens
		normalizedUsage.CompletionTokens = normalizedUsage.OutputTokens
	}
	summary := calculateTextQuotaSummary(c, info, &normalizedUsage)
	if snap := info.TieredBillingSnapshot; snap != nil {
		usedVars := billingexpr.UsedVars(snap.ExprString)
		if ok, quota, result := TryTieredSettle(info, BuildTieredTokenParams(&normalizedUsage, summary.IsClaudeUsageSemantic, usedVars)); ok {
			summary.Quota = composeTieredTextQuota(info, summary, quota, result)
		}
	}
	if costKnown && cost > 0 && summary.Quota == 0 && summary.TotalTokens == 0 {
		summary.Quota = int(math.Round(cost * common.QuotaPerUnit * info.PriceData.GroupRatioInfo.GroupRatio))
		if summary.Quota == 0 && info.PriceData.GroupRatioInfo.GroupRatio > 0 {
			summary.Quota = 1
		}
	}
	billableUsage := hasTokenUsage || typedUsage.Cost != nil && (!costKnown || cost > 0)
	actualQuotaKnown := hasTokenBreakdown || costKnown && cost > 0 || !hasTokenUsage && costKnown
	info.AttemptActualQuota = summary.Quota
	info.AttemptActualQuotaKnown = actualQuotaKnown
	info.AttemptUsageBillingEvidence = billableUsage
	if billableUsage {
		info.AttemptFinancialOutcome = types.AttemptFinancialOutcomeBillable
	} else {
		info.AttemptFinancialOutcome = types.AttemptFinancialOutcomeNonBillable
	}
	return true
}

func CaptureErrorBillingQuota(c *gin.Context, info *relaycommon.RelayInfo, apiErr *types.NewAPIError) bool {
	if c == nil || info == nil || apiErr == nil {
		return false
	}
	usageJSON, cost, costSet := apiErr.GetUpstreamBillingEvidence()
	if len(usageJSON) == 0 && !costSet {
		return false
	}

	usage := dto.Usage{}
	if len(usageJSON) > 0 {
		if err := common.Unmarshal(usageJSON, &usage); err != nil {
			return false
		}
	}
	if costSet {
		usage.Cost = cost
	}
	if !CaptureAttemptUsageQuota(c, info, &usage) {
		return false
	}
	apiErr.SetFinancialOutcome(info.AttemptFinancialOutcome)
	return true
}

func CaptureUpstreamErrorBodyQuota(c *gin.Context, info *relaycommon.RelayInfo, body []byte) bool {
	if c == nil || info == nil || !gjson.ValidBytes(body) {
		return false
	}
	usageJSON, cost, costSet := extractAuthoritativeBillingPayload(gjson.ParseBytes(body))
	if len(usageJSON) == 0 && !costSet {
		return false
	}
	apiErr := types.NewError(errors.New("upstream billing evidence"), types.ErrorCodeBadResponseStatusCode)
	apiErr.SetUpstreamBillingEvidence(usageJSON, cost, costSet)
	return CaptureErrorBillingQuota(c, info, apiErr)
}

func numericUsageCost(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case json.Number:
		cost, err := value.Float64()
		return cost, err == nil
	case string:
		cost, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return cost, err == nil
	default:
		return 0, false
	}
}

type BillingSettlementOutcome struct {
	Quota       int
	Err         error
	Partial     bool
	HeldReserve bool
}

func SettleSuccessfulBilling(c *gin.Context, info *relaycommon.RelayInfo, actualQuota int) BillingSettlementOutcome {
	outcome := BillingSettlementOutcome{Quota: actualQuota}
	outcome.Err = SettleBilling(c, info, actualQuota)
	if outcome.Err == nil {
		return outcome
	}
	if info.BillingSettled {
		outcome.Partial = true
		return outcome
	}

	outcome.HeldReserve = true
	outcome.Quota = max(info.AutoRoute.ReservedQuota, info.FinalPreConsumedQuota)
	if reporter, ok := info.Billing.(interface {
		RemainingHolds() (financialQuota int, tokenQuota int)
	}); ok {
		financialHold, _ := reporter.RemainingHolds()
		outcome.Quota = max(outcome.Quota, financialHold)
	}
	info.AutoRoute.ReleasedQuota = 0
	info.ChargedOnError = true
	return outcome
}

func AppendSettlementOutcome(info *relaycommon.RelayInfo, other map[string]interface{}, outcome BillingSettlementOutcome) {
	if outcome.Err == nil || other == nil {
		return
	}
	other["settlement_error"] = outcome.Err.Error()
	if outcome.Partial {
		other["settlement_partial"] = true
	}
	if outcome.HeldReserve {
		other["charged_on_error"] = true
		other["held_quota"] = outcome.Quota
		other["released_quota"] = 0
	}
	appendBillingInfo(info, other)
}

func chargedAttemptQuota(info *relaycommon.RelayInfo) int {
	if info.AttemptTaskAccepted || info.AttemptHasBillingEvidence {
		if info.AttemptActualQuota > 0 {
			return info.AttemptActualQuota
		}
		return CurrentAutoEstimate(info)
	}
	if info.AttemptUsageBillingEvidence {
		if info.AttemptActualQuotaKnown {
			return info.AttemptActualQuota
		}
		return CurrentAutoEstimate(info)
	}
	if info.AttemptActualQuotaKnown || info.AttemptActualQuota > 0 {
		return info.AttemptActualQuota
	}
	return CurrentAutoEstimate(info)
}

func SettleChargedAttemptError(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError, outcome types.AttemptFinancialOutcome) (error, error) {
	if info.BillingSettled {
		return nil, nil
	}

	quota := chargedAttemptQuota(info)
	info.ChargedOnError = true
	info.AttemptFinancialOutcome = outcome
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = info.StartTime
	}
	if startTime.IsZero() {
		startTime = time.Now()
	}
	requestID := info.RequestId
	if requestID == "" {
		requestID = c.GetString(common.RequestIdKey)
	}
	eventHash := sha256.Sum256([]byte(fmt.Sprintf("charged_error:%s:%d", requestID, info.TokenId)))
	eventID := fmt.Sprintf("%x", eventHash)
	pendingQuota := max(quota, info.AutoRoute.ReservedQuota, info.FinalPreConsumedQuota)
	pendingOther := map[string]interface{}{
		"charged_on_error":   true,
		"financial_outcome":  outcome,
		"error_code":         relayErr.GetErrorCode(),
		"status_code":        relayErr.StatusCode,
		"settlement_pending": true,
		"estimated_billing":  true,
	}
	appendRequestPath(c, info, pendingOther)
	appendBillingInfo(info, pendingOther)
	pendingLog := model.RecordConsumeLogParams{
		ChannelId:      info.ChannelId,
		PromptTokens:   info.GetEstimatePromptTokens(),
		ModelName:      info.OriginModelName,
		TokenName:      c.GetString("token_name"),
		Quota:          pendingQuota,
		Content:        fmt.Sprintf("Request failed after upstream dispatch; settlement is pending for the held %s reserve", logger.FormatQuota(pendingQuota)),
		TokenId:        info.TokenId,
		UseTimeSeconds: int(time.Since(startTime).Seconds()),
		IsStream:       info.IsStream,
		Group:          info.UsingGroup,
		Other:          pendingOther,
		Force:          true,
	}
	if err := model.StageConsumeLogOutboxIntent(c, info.UserId, eventID, pendingLog); err != nil {
		intentErr := fmt.Errorf("persist charged-attempt billing intent: %w", err)
		return intentErr, intentErr
	}

	settleErr := SettleBilling(c, info, quota)
	logQuota := quota
	if settleErr != nil && !info.BillingSettled {
		// Funding did not commit, so the caller keeps the maximum reserve held.
		logQuota = info.AutoRoute.ReservedQuota
		if logQuota == 0 {
			logQuota = info.FinalPreConsumedQuota
		}
		info.AutoRoute.ReleasedQuota = 0
	}

	if logQuota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(info.UserId, logQuota)
		model.UpdateChannelUsedQuota(info.ChannelId, logQuota)
	}
	other := map[string]interface{}{
		"charged_on_error":  true,
		"financial_outcome": outcome,
		"error_code":        relayErr.GetErrorCode(),
		"status_code":       relayErr.StatusCode,
	}
	if settleErr != nil {
		other["settlement_error"] = settleErr.Error()
		if info.BillingSettled {
			other["settlement_partial"] = true
		}
	}
	appendRequestPath(c, info, other)
	appendBillingInfo(info, other)
	finalLog := model.RecordConsumeLogParams{
		ChannelId:      info.ChannelId,
		PromptTokens:   info.GetEstimatePromptTokens(),
		ModelName:      info.OriginModelName,
		TokenName:      c.GetString("token_name"),
		Quota:          logQuota,
		Content:        fmt.Sprintf("Request failed after upstream dispatch; charged %s based on the current group estimate", logger.FormatQuota(logQuota)),
		TokenId:        info.TokenId,
		UseTimeSeconds: int(time.Since(startTime).Seconds()),
		IsStream:       info.IsStream,
		Group:          info.UsingGroup,
		Other:          other,
		Force:          true,
	}
	logErr := model.UpsertConsumeLogOutboxIntent(c, info.UserId, eventID, finalLog)
	if logErr == nil {
		logErr = model.DeliverBillingOutboxEvent(eventID)
	}
	return settleErr, logErr
}

func RefundBilling(c *gin.Context, info *relaycommon.RelayInfo, relayErr *types.NewAPIError) error {
	errorCode := ""
	statusCode := 0
	if relayErr != nil {
		errorCode = string(relayErr.GetErrorCode())
		statusCode = relayErr.StatusCode
	}
	return refundBilling(c, info, errorCode, statusCode)
}

func RefundTaskBilling(c *gin.Context, info *relaycommon.RelayInfo, taskErr *dto.TaskError) error {
	errorCode := ""
	statusCode := 0
	if taskErr != nil {
		errorCode = taskErr.Code
		statusCode = taskErr.StatusCode
	}
	return refundBilling(c, info, errorCode, statusCode)
}

func refundBilling(c *gin.Context, info *relaycommon.RelayInfo, errorCode string, statusCode int) error {
	if info == nil || info.Billing == nil {
		return nil
	}
	refundErr := info.Billing.Refund(c)
	if refundErr == nil {
		return nil
	}

	heldQuota := max(info.FinalPreConsumedQuota, info.AutoRoute.ReservedQuota)
	heldTokenQuota := heldQuota
	if reporter, ok := info.Billing.(interface {
		RemainingHolds() (financialQuota int, tokenQuota int)
	}); ok {
		heldQuota, heldTokenQuota = reporter.RemainingHolds()
	}
	info.ChargedOnError = heldQuota > 0
	other := map[string]interface{}{
		"refund_failed": true,
		"refund_error":  refundErr.Error(),
		"error_code":    errorCode,
		"status_code":   statusCode,
		"held_quota":    heldQuota,
	}
	if heldQuota > 0 {
		other["charged_on_error"] = true
	}
	if heldTokenQuota > 0 {
		other["held_token_quota"] = heldTokenQuota
	}
	appendRequestPath(c, info, other)
	appendBillingInfo(info, other)
	startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
	if startTime.IsZero() {
		startTime = info.StartTime
	}
	content := fmt.Sprintf("Request was not billable, but refunding the %s reserve failed; the reserve remains held", logger.FormatQuota(heldQuota))
	if heldQuota == 0 && heldTokenQuota > 0 {
		content = fmt.Sprintf("Request funds were refunded, but restoring %s of API key quota failed", logger.FormatQuota(heldTokenQuota))
	}
	logErr := model.RecordConsumeLog(c, info.UserId, model.RecordConsumeLogParams{
		ChannelId:      info.ChannelId,
		PromptTokens:   info.GetEstimatePromptTokens(),
		ModelName:      info.OriginModelName,
		TokenName:      c.GetString("token_name"),
		Quota:          heldQuota,
		Content:        content,
		TokenId:        info.TokenId,
		UseTimeSeconds: int(time.Since(startTime).Seconds()),
		IsStream:       info.IsStream,
		Group:          info.UsingGroup,
		Other:          other,
		Force:          true,
	})
	if logErr != nil {
		return errors.Join(refundErr, fmt.Errorf("record held-reserve log: %w", logErr))
	}
	return refundErr
}

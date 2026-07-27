package service

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type TokenDetails struct {
	TextTokens  int
	AudioTokens int
}

type QuotaInfo struct {
	InputDetails  TokenDetails
	OutputDetails TokenDetails
	ModelName     string
	UsePrice      bool
	ModelPrice    float64
	ModelRatio    float64
	GroupRatio    float64
}

func hasCustomModelRatio(modelName string, currentRatio float64) bool {
	defaultRatio, exists := ratio_setting.GetDefaultModelRatioMap()[modelName]
	if !exists {
		return true
	}
	return currentRatio != defaultRatio
}

func calculateAudioQuota(info QuotaInfo) int {
	if info.UsePrice {
		modelPrice := decimal.NewFromFloat(info.ModelPrice)
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		groupRatio := decimal.NewFromFloat(info.GroupRatio)

		quota := modelPrice.Mul(quotaPerUnit).Mul(groupRatio)
		return int(quota.IntPart())
	}

	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(info.ModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(info.ModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(info.ModelName))

	groupRatio := decimal.NewFromFloat(info.GroupRatio)
	modelRatio := decimal.NewFromFloat(info.ModelRatio)
	ratio := groupRatio.Mul(modelRatio)

	inputTextTokens := decimal.NewFromInt(int64(info.InputDetails.TextTokens))
	outputTextTokens := decimal.NewFromInt(int64(info.OutputDetails.TextTokens))
	inputAudioTokens := decimal.NewFromInt(int64(info.InputDetails.AudioTokens))
	outputAudioTokens := decimal.NewFromInt(int64(info.OutputDetails.AudioTokens))

	quota := decimal.Zero
	quota = quota.Add(inputTextTokens)
	quota = quota.Add(outputTextTokens.Mul(completionRatio))
	quota = quota.Add(inputAudioTokens.Mul(audioRatio))
	quota = quota.Add(outputAudioTokens.Mul(audioRatio).Mul(audioCompletionRatio))

	quota = quota.Mul(ratio)

	// If ratio is not zero and quota is less than or equal to zero, set quota to 1
	if !ratio.IsZero() && quota.LessThanOrEqual(decimal.Zero) {
		quota = decimal.NewFromInt(1)
	}

	return int(quota.Round(0).IntPart())
}

func PreWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage) error {
	if relayInfo.UsePrice {
		return nil
	}
	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}

	modelName := relayInfo.OriginModelName
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens
	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens
	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   relayInfo.UsePrice,
		ModelRatio: relayInfo.PriceData.ModelRatio,
		GroupRatio: relayInfo.PriceData.GroupRatioInfo.GroupRatio,
	}

	hasUsage := usage.TotalTokens > 0 ||
		usage.InputTokens > 0 ||
		usage.OutputTokens > 0 ||
		usage.InputTokenDetails.CachedTokens > 0 ||
		textInputTokens > 0 ||
		textOutTokens > 0 ||
		audioInputTokens > 0 ||
		audioOutTokens > 0
	if !hasUsage {
		return nil
	}

	quota := calculateAudioQuota(quotaInfo)
	if relayInfo.TieredBillingSnapshot != nil {
		params := BuildRealtimeTieredTokenParams(usage, tieredUsedVars)
		relayInfo.RealtimeTieredTokenParams.P += params.P
		relayInfo.RealtimeTieredTokenParams.C += params.C
		relayInfo.RealtimeTieredTokenParams.Len += params.Len
		relayInfo.RealtimeTieredTokenParams.CR += params.CR
		relayInfo.RealtimeTieredTokenParams.AI += params.AI
		relayInfo.RealtimeTieredTokenParams.AO += params.AO
		if tieredOK, tieredQuota, _ := TryTieredSettle(relayInfo, relayInfo.RealtimeTieredTokenParams); tieredOK {
			quota = tieredQuota
		}
		relayInfo.RealtimeConsumedQuota = quota
	} else {
		relayInfo.RealtimeConsumedQuota += quota
	}
	relayInfo.AttemptActualQuota = relayInfo.RealtimeConsumedQuota
	relayInfo.AttemptActualQuotaKnown = true
	relayInfo.AttemptUsageBillingEvidence = true

	if relayInfo.Billing != nil {
		targetQuota := max(relayInfo.Billing.GetPreConsumedQuota(), relayInfo.RealtimeConsumedQuota)
		if err := relayInfo.Billing.Reserve(targetQuota); err != nil {
			return err
		}
	} else if relayInfo.RealtimeConsumedQuota > 0 {
		if apiErr := PreConsumeBilling(ctx, relayInfo.RealtimeConsumedQuota, relayInfo); apiErr != nil {
			return apiErr
		}
	}
	logger.LogInfo(ctx, "realtime streaming reserve success, cumulative quota: "+fmt.Sprintf("%d", relayInfo.RealtimeConsumedQuota))
	return nil
}

func PostWssConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelName string,
	usage *dto.RealtimeUsage, extraContent string) error {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildRealtimeTieredTokenParams(usage, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.InputTokenDetails.TextTokens
	textOutTokens := usage.OutputTokenDetails.TextTokens

	audioInputTokens := usage.InputTokenDetails.AudioTokens
	audioOutTokens := usage.OutputTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(modelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(modelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  modelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota := calculateAudioQuota(quotaInfo)
	if tieredOk {
		quota = tieredQuota
	}

	hasUsage := usage.TotalTokens > 0 ||
		usage.InputTokens > 0 ||
		usage.OutputTokens > 0 ||
		textInputTokens > 0 ||
		textOutTokens > 0 ||
		audioInputTokens > 0 ||
		audioOutTokens > 0
	missingUsageEstimated := false
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	if !hasUsage {
		if relayInfo.AttemptActualQuotaKnown {
			quota = relayInfo.AttemptActualQuota
			logContent += "，上游终态未返回 usage，按已确认的实时用量结算"
		} else {
			quota = CurrentAutoEstimate(relayInfo)
			missingUsageEstimated = true
			relayInfo.AttemptFinancialOutcome = types.AttemptFinancialOutcomeAmbiguous
			logContent += "，上游终态未返回可确认的计费信息，按请求估算额度结算"
		}
		logger.LogError(ctx, fmt.Sprintf("realtime terminal usage is missing, settling quota %d, userId %d, channelId %d, "+
			"tokenId %d, model %s, pre-consumed quota %d", quota, relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, modelName, relayInfo.FinalPreConsumedQuota))
	}

	settlement := SettleSuccessfulBilling(ctx, relayInfo, quota)
	if settlement.Err != nil {
		logContent += fmt.Sprintf("，计费结算异常，日志按实际保留额度 %s 记录", logger.FormatQuota(settlement.Quota))
		logger.LogError(ctx, "error settling billing: "+settlement.Err.Error())
	}
	if settlement.Quota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, settlement.Quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, settlement.Quota)
	}

	logModel := modelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateWssOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	if missingUsageEstimated {
		other["usage_missing"] = true
		other["estimated_billing"] = true
	}
	AppendSettlementOutcome(relayInfo, other, settlement)
	logErr := model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.InputTokens,
		CompletionTokens: usage.OutputTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            settlement.Quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Force:            settlement.Err != nil || missingUsageEstimated,
	})
	if logErr != nil {
		common.SysLog("error recording realtime consume log: " + logErr.Error())
	}
	relayInfo.AttemptSettlementHandled = true
	return errors.Join(settlement.Err, logErr)
}

func CalcOpenRouterCacheCreateTokens(usage dto.Usage, priceData types.PriceData) int {
	if priceData.CacheCreationRatio == 1 {
		return 0
	}
	quotaPrice := priceData.ModelRatio / common.QuotaPerUnit
	promptCacheCreatePrice := quotaPrice * priceData.CacheCreationRatio
	promptCacheReadPrice := quotaPrice * priceData.CacheRatio
	completionPrice := quotaPrice * priceData.CompletionRatio

	cost, _ := usage.Cost.(float64)
	totalPromptTokens := float64(usage.PromptTokens)
	completionTokens := float64(usage.CompletionTokens)
	promptCacheReadTokens := float64(usage.PromptTokensDetails.CachedTokens)

	return int(math.Round((cost -
		totalPromptTokens*quotaPrice +
		promptCacheReadTokens*(quotaPrice-promptCacheReadPrice) -
		completionTokens*completionPrice) /
		(promptCacheCreatePrice - quotaPrice)))
}

func PostAudioConsumeQuota(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, extraContent string) error {

	var tieredUsedVars map[string]bool
	if snap := relayInfo.TieredBillingSnapshot; snap != nil {
		tieredUsedVars = billingexpr.UsedVars(snap.ExprString)
	}
	var tieredResult *billingexpr.TieredResult
	tieredOk, tieredQuota, tieredRes := TryTieredSettle(relayInfo, BuildTieredTokenParams(usage, false, tieredUsedVars))
	if tieredOk {
		tieredResult = tieredRes
	}

	useTimeSeconds := time.Now().Unix() - relayInfo.StartTime.Unix()
	textInputTokens := usage.PromptTokensDetails.TextTokens
	textOutTokens := usage.CompletionTokenDetails.TextTokens

	audioInputTokens := usage.PromptTokensDetails.AudioTokens
	audioOutTokens := usage.CompletionTokenDetails.AudioTokens

	tokenName := ctx.GetString("token_name")
	completionRatio := decimal.NewFromFloat(ratio_setting.GetCompletionRatio(relayInfo.OriginModelName))
	audioRatio := decimal.NewFromFloat(ratio_setting.GetAudioRatio(relayInfo.OriginModelName))
	audioCompletionRatio := decimal.NewFromFloat(ratio_setting.GetAudioCompletionRatio(relayInfo.OriginModelName))

	modelRatio := relayInfo.PriceData.ModelRatio
	groupRatio := relayInfo.PriceData.GroupRatioInfo.GroupRatio
	modelPrice := relayInfo.PriceData.ModelPrice
	usePrice := relayInfo.PriceData.UsePrice

	quotaInfo := QuotaInfo{
		InputDetails: TokenDetails{
			TextTokens:  textInputTokens,
			AudioTokens: audioInputTokens,
		},
		OutputDetails: TokenDetails{
			TextTokens:  textOutTokens,
			AudioTokens: audioOutTokens,
		},
		ModelName:  relayInfo.OriginModelName,
		UsePrice:   usePrice,
		ModelRatio: modelRatio,
		GroupRatio: groupRatio,
	}

	quota := calculateAudioQuota(quotaInfo)
	if tieredOk {
		quota = tieredQuota
	}

	hasUsage := usage.TotalTokens > 0 ||
		usage.PromptTokens > 0 ||
		usage.CompletionTokens > 0 ||
		textInputTokens > 0 ||
		textOutTokens > 0 ||
		audioInputTokens > 0 ||
		audioOutTokens > 0
	missingUsageEstimated := false
	var logContent string
	if !usePrice {
		logContent = fmt.Sprintf("模型倍率 %.2f，补全倍率 %.2f，音频倍率 %.2f，音频补全倍率 %.2f，分组倍率 %.2f",
			modelRatio, completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), groupRatio)
	} else {
		logContent = fmt.Sprintf("模型价格 %.2f，分组倍率 %.2f", modelPrice, groupRatio)
	}

	if !hasUsage {
		capturedExactBilling := CaptureAttemptUsageQuota(ctx, relayInfo, usage) &&
			relayInfo.AttemptActualQuotaKnown
		if capturedExactBilling {
			quota = relayInfo.AttemptActualQuota
			logContent += "，上游未返回 token usage，按明确 cost 结算"
		} else {
			quota = CurrentAutoEstimate(relayInfo)
			missingUsageEstimated = true
			relayInfo.AttemptFinancialOutcome = types.AttemptFinancialOutcomeAmbiguous
			logContent += "，上游未返回可确认的计费信息，按请求估算额度结算"
		}
		logger.LogError(ctx, fmt.Sprintf("audio usage is missing, settling quota %d, userId %d, channelId %d, "+
			"tokenId %d, model %s, pre-consumed quota %d", quota, relayInfo.UserId, relayInfo.ChannelId, relayInfo.TokenId, relayInfo.OriginModelName, relayInfo.FinalPreConsumedQuota))
	}

	settlement := SettleSuccessfulBilling(ctx, relayInfo, quota)
	if settlement.Err != nil {
		logContent += fmt.Sprintf("，计费结算异常，日志按实际保留额度 %s 记录", logger.FormatQuota(settlement.Quota))
		logger.LogError(ctx, "error settling billing: "+settlement.Err.Error())
	}
	if settlement.Quota > 0 {
		model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, settlement.Quota)
		model.UpdateChannelUsedQuota(relayInfo.ChannelId, settlement.Quota)
	}

	logModel := relayInfo.OriginModelName
	if extraContent != "" {
		logContent += ", " + extraContent
	}
	other := GenerateAudioOtherInfo(ctx, relayInfo, usage, modelRatio, groupRatio,
		completionRatio.InexactFloat64(), audioRatio.InexactFloat64(), audioCompletionRatio.InexactFloat64(), modelPrice, relayInfo.PriceData.GroupRatioInfo.GroupSpecialRatio)
	if tieredResult != nil {
		InjectTieredBillingInfo(other, relayInfo, tieredResult)
	}
	if missingUsageEstimated {
		other["usage_missing"] = true
		other["estimated_billing"] = true
	}
	AppendSettlementOutcome(relayInfo, other, settlement)
	logErr := model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{
		ChannelId:        relayInfo.ChannelId,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		ModelName:        logModel,
		TokenName:        tokenName,
		Quota:            settlement.Quota,
		Content:          logContent,
		TokenId:          relayInfo.TokenId,
		UseTimeSeconds:   int(useTimeSeconds),
		IsStream:         relayInfo.IsStream,
		Group:            relayInfo.UsingGroup,
		Other:            other,
		Force:            settlement.Err != nil || missingUsageEstimated,
	})
	if logErr != nil {
		common.SysLog("error recording audio consume log: " + logErr.Error())
	}
	gopool.Go(func() {
		perfmetrics.RecordRelaySample(relayInfo, true, int64(usage.CompletionTokens))
	})
	return errors.Join(settlement.Err, logErr)
}

func PreConsumeTokenQuota(relayInfo *relaycommon.RelayInfo, quota int) error {
	if quota < 0 {
		return errors.New("quota 不能为负数！")
	}
	if relayInfo.IsPlayground {
		return nil
	}
	//if relayInfo.TokenUnlimited {
	//	return nil
	//}
	return model.ReserveTokenQuotaForBilling(relayInfo.TokenId, relayInfo.TokenKey, quota, relayInfo.TokenUnlimited)
}

type QuotaAdjustmentResult struct {
	FundingCommitted bool
	TokenCommitted   bool
	Err              error
}

func PostConsumeQuotaWithResult(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) QuotaAdjustmentResult {
	var result QuotaAdjustmentResult

	// 1) Consume from wallet quota OR subscription item
	if relayInfo != nil && relayInfo.BillingSource == BillingSourceSubscription {
		if relayInfo.SubscriptionId == 0 {
			result.Err = errors.New("subscription id is missing")
			return result
		}
		delta := int64(quota)
		if delta != 0 {
			if err := model.PostConsumeUserSubscriptionDelta(relayInfo.SubscriptionId, delta); err != nil {
				result.Err = err
				return result
			}
			relayInfo.SubscriptionPostDelta += delta
		}
	} else {
		// Wallet
		var err error
		if quota > 0 {
			err = model.DebitUserQuotaForBilling(relayInfo.UserId, quota)
		} else {
			err = model.RefundUserQuotaForBilling(relayInfo.UserId, -quota)
		}
		if err != nil {
			result.Err = err
			return result
		}
	}
	result.FundingCommitted = true

	if !relayInfo.IsPlayground {
		var err error
		if quota > 0 {
			err = model.DebitTokenQuotaForBilling(relayInfo.TokenId, relayInfo.TokenKey, quota)
		} else {
			err = model.RefundTokenQuotaForBilling(relayInfo.TokenId, relayInfo.TokenKey, -quota)
		}
		if err != nil {
			result.Err = err
			return result
		}
	}
	result.TokenCommitted = true

	if sendEmail {
		if (quota + preConsumedQuota) != 0 {
			checkAndSendQuotaNotify(relayInfo, quota, preConsumedQuota)
		}
	}

	return result
}

func PostConsumeQuota(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int, sendEmail bool) error {
	return PostConsumeQuotaWithResult(relayInfo, quota, preConsumedQuota, sendEmail).Err
}

func checkAndSendQuotaNotify(relayInfo *relaycommon.RelayInfo, quota int, preConsumedQuota int) {
	gopool.Go(func() {
		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		//noMoreQuota := userCache.Quota-(quota+preConsumedQuota) <= 0
		quotaTooLow := false
		consumeQuota := quota + preConsumedQuota
		if relayInfo.UserQuota-consumeQuota < threshold {
			quotaTooLow = true
		}
		if quotaTooLow {
			prompt := "您的额度即将用尽"
			topUpLink := PaymentReturnURL("/console/topup")

			notifyType := userSetting.NotifyType
			if notifyType == "" {
				notifyType = dto.NotifyTypeEmail
			}
			content, values := quotaNotifyContent(notifyType, prompt, logger.FormatQuota(relayInfo.UserQuota), topUpLink)

			err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values))
			if err != nil {
				common.SysError(fmt.Sprintf("failed to send quota notify to user %d: %s", relayInfo.UserId, err.Error()))
			}
		}
	})
}

func checkAndSendSubscriptionQuotaNotify(relayInfo *relaycommon.RelayInfo) {
	gopool.Go(func() {
		if relayInfo == nil {
			return
		}
		if relayInfo.SubscriptionId == 0 || relayInfo.SubscriptionAmountTotal <= 0 {
			return
		}

		userSetting := relayInfo.UserSetting
		threshold := common.QuotaRemindThreshold
		if userSetting.QuotaWarningThreshold != 0 {
			threshold = int(userSetting.QuotaWarningThreshold)
		}

		usedAfter := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		remaining := relayInfo.SubscriptionAmountTotal - usedAfter
		if remaining >= int64(threshold) {
			return
		}

		prompt := "您的订阅额度即将用尽"
		topUpLink := PaymentReturnURL("/console/topup")

		notifyType := userSetting.NotifyType
		if notifyType == "" {
			notifyType = dto.NotifyTypeEmail
		}
		content, values := quotaNotifyContent(notifyType, prompt, logger.FormatQuota(int(remaining)), topUpLink)

		if err := NotifyUser(relayInfo.UserId, relayInfo.UserEmail, relayInfo.UserSetting, dto.NewNotify(dto.NotifyTypeQuotaExceed, prompt, content, values)); err != nil {
			common.SysError(fmt.Sprintf("failed to send subscription quota notify to user %d: %s", relayInfo.UserId, err.Error()))
		}
	})
}

func quotaNotifyContent(notifyType string, prompt string, remainingQuota string, topUpLink string) (string, []interface{}) {
	switch notifyType {
	case dto.NotifyTypeBark:
		return "{{value}}，剩余额度：{{value}}，请及时充值", []interface{}{prompt, remainingQuota}
	case dto.NotifyTypeGotify:
		return "{{value}}，当前剩余额度为 {{value}}，请及时充值。", []interface{}{prompt, remainingQuota}
	case dto.NotifyTypeTelegram:
		return "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。\n充值链接：{{value}}", []interface{}{prompt, remainingQuota, topUpLink}
	default:
		return "{{value}}，当前剩余额度为 {{value}}，为了不影响您的使用，请及时充值。<br/>充值链接：<a href='{{value}}'>{{value}}</a>", []interface{}{prompt, remainingQuota, topUpLink, topUpLink}
	}
}

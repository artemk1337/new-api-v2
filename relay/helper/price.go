package helper

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func modelPriceNotConfiguredError(modelName string, userId int) error {
	if model.IsAdmin(userId) {
		return fmt.Errorf(
			"模型 %s 的价格未配置。请前往「系统设置 → 运营设置」开启自用模式，或在「系统设置 → 分组与模型定价设置」中为该模型配置价格；"+
				"Model %s price not configured. Go to System Settings → Operation Settings to enable self-use mode, or configure the model price in System Settings → Group & Model Pricing.",
			modelName, modelName,
		)
	}
	return fmt.Errorf(
		"模型 %s 的价格尚未由管理员配置，暂时无法使用，请联系站点管理员开启该模型；"+
			"Model %s has not been priced by the administrator yet. Please contact the site administrator to enable this model.",
		modelName, modelName,
	)
}

// https://docs.claude.com/en/docs/build-with-claude/prompt-caching#1-hour-cache-duration
const claudeCacheCreation1hMultiplier = 6 / 3.75

// HandleGroupRatio checks for "auto_group" in the context and updates the group ratio and relayInfo.UsingGroup if present
func HandleGroupRatio(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) types.GroupRatioInfo {
	groupRatioInfo := types.GroupRatioInfo{
		GroupRatio:        1.0, // default ratio
		GroupSpecialRatio: -1,
	}

	// check auto group
	autoGroup, exists := ctx.Get("auto_group")
	if exists {
		logger.LogDebug(ctx, "final group: %s", autoGroup)
		relayInfo.UsingGroup = autoGroup.(string)
	}

	// check user group special ratio
	userGroupRatio, ok := ratio_setting.GetGroupGroupRatio(relayInfo.UserGroup, relayInfo.UsingGroup)
	if ok {
		// user group special ratio
		groupRatioInfo.GroupSpecialRatio = userGroupRatio
		groupRatioInfo.GroupRatio = userGroupRatio
		groupRatioInfo.HasSpecialRatio = true
	} else {
		// normal group ratio
		groupRatioInfo.GroupRatio = ratio_setting.GetGroupRatio(relayInfo.UsingGroup)
	}

	return groupRatioInfo
}

func ModelPriceHelper(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta) (types.PriceData, error) {
	unlockPricing := ratio_setting.LockPricingConfigRead()
	defer unlockPricing()
	billingModelName := GetBillingModelName(info)
	if !HasModelBillingConfig(billingModelName) {
		return types.PriceData{}, modelPriceNotConfiguredError(info.OriginModelName, info.UserId)
	}
	modelPrice, usePrice := ratio_setting.GetModelPrice(billingModelName, false)

	groupRatioInfo := HandleGroupRatio(c, info)

	// Check if this model uses tiered_expr billing
	if billing_setting.GetBillingMode(billingModelName) == billing_setting.BillingModeTieredExpr {
		return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
	}

	var preConsumedQuota int
	var modelRatio float64
	var completionRatio float64
	var cacheRatio float64
	var imageRatio float64
	var cacheCreationRatio float64
	var cacheCreationRatio5m float64
	var cacheCreationRatio1h float64
	var audioRatio float64
	var audioCompletionRatio float64
	var freeModel bool
	if !usePrice {
		preConsumedTokens := common.Max(promptTokens, common.PreConsumedQuota)
		if meta.MaxTokens != 0 {
			preConsumedTokens += meta.MaxTokens
		}
		var success bool
		var matchName string
		modelRatio, success, matchName = ratio_setting.GetModelRatio(billingModelName)
		if !success {
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
		completionRatio = ratio_setting.GetCompletionRatio(billingModelName)
		cacheRatio, _ = ratio_setting.GetCacheRatio(billingModelName)
		cacheCreationRatio, _ = ratio_setting.GetCreateCacheRatio(billingModelName)
		cacheCreationRatio5m = cacheCreationRatio
		// 固定1h和5min缓存写入价格的比例
		cacheCreationRatio1h = cacheCreationRatio * claudeCacheCreation1hMultiplier
		imageRatio, _ = ratio_setting.GetImageRatio(billingModelName)
		audioRatio = ratio_setting.GetAudioRatio(billingModelName)
		audioCompletionRatio = ratio_setting.GetAudioCompletionRatio(billingModelName)
		ratio := modelRatio * groupRatioInfo.GroupRatio
		preConsumedQuota = int(float64(preConsumedTokens) * ratio)
	} else {
		if meta.ImagePriceRatio != 0 {
			modelPrice = modelPrice * meta.ImagePriceRatio
		}
		preConsumedQuota = int(modelPrice * common.GetQuotaPerUnit() * groupRatioInfo.GroupRatio)
	}

	// check if free model pre-consume is disabled
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		// if model price or ratio is 0, do not pre-consume quota
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		} else if usePrice {
			if modelPrice == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		} else {
			if modelRatio == 0 {
				preConsumedQuota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:            freeModel,
		ModelPrice:           modelPrice,
		ModelRatio:           modelRatio,
		CompletionRatio:      completionRatio,
		GroupRatioInfo:       groupRatioInfo,
		UsePrice:             usePrice,
		CacheRatio:           cacheRatio,
		ImageRatio:           imageRatio,
		AudioRatio:           audioRatio,
		AudioCompletionRatio: audioCompletionRatio,
		CacheCreationRatio:   cacheCreationRatio,
		CacheCreation5mRatio: cacheCreationRatio5m,
		CacheCreation1hRatio: cacheCreationRatio1h,
		QuotaToPreConsume:    preConsumedQuota,
	}

	if common.DebugEnabled {
		logger.LogDebug(c, "model_price_helper result: %s", priceData.ToSetting())
	}
	info.PriceData = priceData
	return priceData, nil
}

func BuildAutoRouteState(c *gin.Context, info *relaycommon.RelayInfo, groups []string, promptTokens int, meta *types.TokenCountMeta) (relaycommon.AutoRouteState, error) {
	state := relaycommon.AutoRouteState{
		Candidates: make([]relaycommon.AutoRouteCandidate, 0, len(groups)),
	}
	if len(groups) == 0 {
		return state, nil
	}

	originalGroup := info.UsingGroup
	requestInput := info.BillingRequestInput
	for _, group := range groups {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
		candidateInfo := *info
		candidateInfo.UsingGroup = group
		candidateInfo.PriceData = types.PriceData{}
		candidateInfo.TieredBillingSnapshot = nil

		priceData, err := ModelPriceHelper(c, &candidateInfo, promptTokens, meta)
		if err != nil {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, originalGroup)
			return relaycommon.AutoRouteState{}, err
		}
		if requestInput == nil {
			requestInput = candidateInfo.BillingRequestInput
		}
		state.Candidates = append(state.Candidates, relaycommon.AutoRouteCandidate{
			Group:          group,
			Ratio:          priceData.GroupRatioInfo.GroupRatio,
			EstimatedQuota: priceData.QuotaToPreConsume,
			PriceData:      priceData,
			TieredSnapshot: candidateInfo.TieredBillingSnapshot,
		})
		if priceData.QuotaToPreConsume >= state.ReservedQuota {
			state.ReservedQuota = priceData.QuotaToPreConsume
			state.ReserveGroup = group
		}
	}

	state.InitialGroup = groups[0]
	state.UsedGroup = groups[0]
	info.AutoRoute = state
	info.BillingRequestInput = requestInput
	info.ApplyAutoRouteCandidate(state.InitialGroup)
	common.SetContextKey(c, constant.ContextKeyAutoGroup, state.InitialGroup)
	return state, nil
}

func BuildAutoPerCallRouteState(c *gin.Context, info *relaycommon.RelayInfo, groups []string, otherRatios map[string]float64) (relaycommon.AutoRouteState, error) {
	state := relaycommon.AutoRouteState{
		Candidates:   make([]relaycommon.AutoRouteCandidate, 0, len(groups)),
		FailedGroups: append([]relaycommon.AutoFailedGroup(nil), info.AutoRoute.FailedGroups...),
	}
	if len(groups) == 0 {
		return state, nil
	}

	originalGroup := info.UsingGroup
	for _, group := range groups {
		common.SetContextKey(c, constant.ContextKeyAutoGroup, group)
		candidateInfo := *info
		candidateInfo.UsingGroup = group
		candidateInfo.PriceData = types.PriceData{}

		priceData, err := ModelPriceHelperPerCall(c, &candidateInfo)
		if err != nil {
			common.SetContextKey(c, constant.ContextKeyAutoGroup, originalGroup)
			return relaycommon.AutoRouteState{}, err
		}
		priceData.OtherRatios = make(map[string]float64, len(otherRatios))
		for name, ratio := range otherRatios {
			priceData.OtherRatios[name] = ratio
		}
		if !common.StringsContains(constant.TaskPricePatches, GetBillingModelName(info)) {
			for _, ratio := range priceData.OtherRatios {
				if ratio != 1 {
					priceData.Quota = int(float64(priceData.Quota) * ratio)
				}
			}
		}

		state.Candidates = append(state.Candidates, relaycommon.AutoRouteCandidate{
			Group:          group,
			Ratio:          priceData.GroupRatioInfo.GroupRatio,
			EstimatedQuota: priceData.Quota,
			PriceData:      priceData,
		})
		if priceData.Quota >= state.ReservedQuota {
			state.ReservedQuota = priceData.Quota
			state.ReserveGroup = group
		}
	}

	state.InitialGroup = groups[0]
	state.UsedGroup = originalGroup
	info.AutoRoute = state
	if !info.ApplyAutoRouteCandidate(state.UsedGroup) {
		info.ApplyAutoRouteCandidate(state.InitialGroup)
		state.UsedGroup = state.InitialGroup
		info.AutoRoute.UsedGroup = state.UsedGroup
	}
	common.SetContextKey(c, constant.ContextKeyAutoGroup, state.UsedGroup)
	return state, nil
}

// ModelPriceHelperPerCall 按次/按量计费的 PriceHelper (MJ、Task)
func ModelPriceHelperPerCall(c *gin.Context, info *relaycommon.RelayInfo) (types.PriceData, error) {
	unlockPricing := ratio_setting.LockPricingConfigRead()
	defer unlockPricing()
	billingModelName := GetBillingModelName(info)
	if !HasModelBillingConfig(billingModelName) {
		return types.PriceData{}, modelPriceNotConfiguredError(info.OriginModelName, info.UserId)
	}
	groupRatioInfo := HandleGroupRatio(c, info)

	modelPrice, success := ratio_setting.GetModelPrice(billingModelName, true)
	usePrice := success
	var modelRatio float64

	if !success {
		defaultPrice, ok := ratio_setting.GetDefaultModelPriceMap()[billingModelName]
		if ok {
			modelPrice = defaultPrice
			usePrice = true
		} else {
			var ratioSuccess bool
			var matchName string
			modelRatio, ratioSuccess, matchName = ratio_setting.GetModelRatio(billingModelName)
			acceptUnsetRatio := false
			if info.UserSetting.AcceptUnsetRatioModel {
				acceptUnsetRatio = true
			}
			if !ratioSuccess && !acceptUnsetRatio {
				return types.PriceData{}, modelPriceNotConfiguredError(matchName, info.UserId)
			}
		}
	}

	var quota int
	freeModel := false

	if usePrice {
		quota = int(modelPrice * common.GetQuotaPerUnit() * groupRatioInfo.GroupRatio)
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelPrice == 0 {
				quota = 0
				freeModel = true
			}
		}
	} else {
		// 按量计费：以模型倍率的一半作为预扣额度
		quota = int(modelRatio / 2 * common.GetQuotaPerUnit() * groupRatioInfo.GroupRatio)
		modelPrice = -1
		if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
			if groupRatioInfo.GroupRatio == 0 || modelRatio == 0 {
				quota = 0
				freeModel = true
			}
		}
	}

	priceData := types.PriceData{
		FreeModel:      freeModel,
		ModelPrice:     modelPrice,
		ModelRatio:     modelRatio,
		UsePrice:       usePrice,
		Quota:          quota,
		GroupRatioInfo: groupRatioInfo,
	}
	return priceData, nil
}

func GetBillingModelName(info *relaycommon.RelayInfo) string {
	if info.BillingModelName != "" {
		return info.BillingModelName
	}
	if info.ChannelMeta != nil && info.IsModelMapped && info.UpstreamModelName != "" {
		return info.UpstreamModelName
	}
	return info.OriginModelName
}

func HasModelBillingConfig(modelName string) bool {
	if price, ok := ratio_setting.GetModelPrice(modelName, false); ok && price > 0 {
		return true
	}
	formattedName := ratio_setting.FormatMatchingModelName(modelName)
	ratioMap := ratio_setting.GetModelRatioCopy()
	if ratio, ok := ratioMap[formattedName]; ok && ratio > 0 {
		return true
	}
	if strings.HasSuffix(formattedName, ratio_setting.CompactModelSuffix) {
		if ratio, ok := ratioMap[ratio_setting.CompactWildcardModelKey]; ok && ratio > 0 {
			return true
		}
	}
	if billing_setting.GetBillingMode(modelName) != billing_setting.BillingModeTieredExpr {
		return false
	}
	expr, ok := billing_setting.GetBillingExpr(modelName)
	if !ok || strings.TrimSpace(expr) == "" {
		return false
	}
	if !billingexpr.ReasoningSplitIsSafe(expr) {
		return false
	}
	_, err := billingexpr.CompileFromCache(expr)
	return err == nil
}

func modelPriceHelperTiered(c *gin.Context, info *relaycommon.RelayInfo, promptTokens int, meta *types.TokenCountMeta, groupRatioInfo types.GroupRatioInfo) (types.PriceData, error) {
	billingModelName := GetBillingModelName(info)
	exprStr, ok := billing_setting.GetBillingExpr(billingModelName)
	if !ok {
		return types.PriceData{}, fmt.Errorf("model %s is configured as tiered_expr but has no billing expression", info.OriginModelName)
	}
	if !billingexpr.ReasoningSplitIsSafe(exprStr) {
		return types.PriceData{}, fmt.Errorf("model %s uses unsupported reasoning pricing expression", info.OriginModelName)
	}

	estimatedCompletionTokens := 0
	if meta.MaxTokens != 0 {
		estimatedCompletionTokens = meta.MaxTokens
	}

	requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)
	if err != nil {
		return types.PriceData{}, err
	}

	estimatedParams := billingexpr.TokenParams{
		P:   float64(promptTokens),
		C:   float64(estimatedCompletionTokens),
		Len: float64(promptTokens),
	}
	if billingexpr.UsedVars(exprStr)["rt"] {
		// Reasoning tokens are only known after the upstream response. Reserve
		// the whole estimated completion as reasoning, then settle by actual use.
		estimatedParams.C = 0
		estimatedParams.RT = float64(estimatedCompletionTokens)
	}
	rawCost, trace, err := billingexpr.RunExprWithRequest(exprStr, estimatedParams, requestInput)
	if err != nil {
		return types.PriceData{}, fmt.Errorf("model %s tiered expr run failed: %w", info.OriginModelName, err)
	}
	if billingexpr.UsedVars(exprStr)["rt"] {
		completionParams := estimatedParams
		completionParams.C = float64(estimatedCompletionTokens)
		completionParams.RT = 0
		completionCost, completionTrace, completionErr := billingexpr.RunExprWithRequest(exprStr, completionParams, requestInput)
		if completionErr != nil {
			return types.PriceData{}, fmt.Errorf("model %s tiered expr completion estimate failed: %w", info.OriginModelName, completionErr)
		}
		if completionCost > rawCost {
			rawCost, trace = completionCost, completionTrace
		}
	}

	// Expression coefficients are $/1M tokens prices; convert to quota the same way per-call billing does.
	quotaBeforeGroup := rawCost / 1_000_000 * common.GetQuotaPerUnit()
	preConsumedQuota := billingexpr.QuotaRound(quotaBeforeGroup * groupRatioInfo.GroupRatio)

	freeModel := false
	if !operation_setting.GetQuotaSetting().EnableFreeModelPreConsume {
		if groupRatioInfo.GroupRatio == 0 {
			preConsumedQuota = 0
			freeModel = true
		}
	}

	exprHash := billingexpr.ExprHashString(exprStr)
	snapshot := &billingexpr.BillingSnapshot{
		BillingMode:               billing_setting.BillingModeTieredExpr,
		ModelName:                 info.OriginModelName,
		ExprString:                exprStr,
		ExprHash:                  exprHash,
		GroupRatio:                groupRatioInfo.GroupRatio,
		EstimatedPromptTokens:     promptTokens,
		EstimatedCompletionTokens: estimatedCompletionTokens,
		EstimatedQuotaBeforeGroup: quotaBeforeGroup,
		EstimatedQuotaAfterGroup:  preConsumedQuota,
		EstimatedTier:             trace.MatchedTier,
		QuotaPerUnit:              common.GetQuotaPerUnit(),
		ExprVersion:               billingexpr.ExprVersion(exprStr),
	}
	info.TieredBillingSnapshot = snapshot
	info.BillingRequestInput = &requestInput

	priceData := types.PriceData{
		FreeModel:         freeModel,
		GroupRatioInfo:    groupRatioInfo,
		QuotaToPreConsume: preConsumedQuota,
	}

	logger.LogDebug(c, "model_price_helper_tiered result: model=%s preConsume=%d quotaBeforeGroup=%.2f groupRatio=%.2f tier=%s", info.OriginModelName, preConsumedQuota, quotaBeforeGroup, groupRatioInfo.GroupRatio, trace.MatchedTier)

	info.PriceData = priceData
	return priceData, nil
}

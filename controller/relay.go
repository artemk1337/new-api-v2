package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func relayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	switch info.RelayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
		err = relay.ImageHelper(c, info)
	case relayconstant.RelayModeAudioSpeech:
		fallthrough
	case relayconstant.RelayModeAudioTranslation:
		fallthrough
	case relayconstant.RelayModeAudioTranscription:
		err = relay.AudioHelper(c, info)
	case relayconstant.RelayModeRerank:
		err = relay.RerankHelper(c, info)
	case relayconstant.RelayModeEmbeddings:
		err = relay.EmbeddingHelper(c, info)
	case relayconstant.RelayModeResponses, relayconstant.RelayModeResponsesCompact:
		err = relay.ResponsesHelper(c, info)
	default:
		err = relay.TextHelper(c, info)
	}
	return err
}

func geminiRelayHandler(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError {
	var err *types.NewAPIError
	if strings.Contains(c.Request.URL.Path, "embed") {
		err = relay.GeminiEmbeddingHandler(c, info)
	} else {
		err = relay.GeminiHelper(c, info)
	}
	return err
}

func Relay(c *gin.Context, relayFormat types.RelayFormat) {

	requestId := c.GetString(common.RequestIdKey)
	//group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	//originalModel := common.GetContextKeyString(c, constant.ContextKeyOriginalModel)

	var (
		newAPIError *types.NewAPIError
		ws          *websocket.Conn
	)

	if relayFormat == types.RelayFormatOpenAIRealtime {
		var err error
		ws, err = upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			helper.WssError(c, ws, types.NewError(err, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry()).ToOpenAIError())
			return
		}
		defer ws.Close()
	}

	defer func() {
		if newAPIError != nil {
			prepareRelayResponseError(newAPIError, requestId)
			logger.LogError(c, fmt.Sprintf("relay error: %s", common.LocalLogPreview(newAPIError.Error())))
			switch relayFormat {
			case types.RelayFormatOpenAIRealtime:
				helper.WssError(c, ws, newAPIError.ToOpenAIError())
			case types.RelayFormatClaude:
				c.JSON(newAPIError.StatusCode, gin.H{
					"type":  "error",
					"error": newAPIError.ToClaudeError(),
				})
			default:
				c.JSON(newAPIError.StatusCode, gin.H{
					"error": newAPIError.ToOpenAIError(),
				})
			}
		}
	}()

	request, err := helper.GetAndValidateRequest(c, relayFormat)
	if err != nil {
		// Map "request body too large" to 413 so clients can handle it correctly
		if common.IsRequestBodyTooLargeError(err) || errors.Is(err, common.ErrRequestBodyTooLarge) {
			newAPIError = types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
		} else {
			newAPIError = types.NewError(err, types.ErrorCodeInvalidRequest)
		}
		return
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeGenRelayInfoFailed)
		return
	}

	needSensitiveCheck := setting.ShouldCheckPromptSensitive()
	needCountToken := constant.CountToken
	// Avoid building huge CombineText (strings.Join) when token counting and sensitive check are both disabled.
	var meta *types.TokenCountMeta
	if needSensitiveCheck || needCountToken {
		meta = request.GetTokenCountMeta()
	} else {
		meta = fastTokenCountMetaForPricing(request)
	}

	if needSensitiveCheck && meta != nil {
		contains, words := service.CheckSensitiveText(meta.CombineText)
		if contains {
			logger.LogWarn(c, fmt.Sprintf("user sensitive words detected: %s", strings.Join(words, ", ")))
			newAPIError = types.NewError(err, types.ErrorCodeSensitiveWordsDetected)
			return
		}
	}

	tokens, err := service.EstimateRequestToken(c, meta, relayInfo)
	if err != nil {
		newAPIError = types.NewError(err, types.ErrorCodeCountTokenFailed)
		return
	}

	relayInfo.SetEstimatePromptTokens(tokens)

	if relayInfo.TokenGroup == "" {
		if _, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup); !ok {
			channel, selectGroup, selectErr := service.CacheGetRandomSatisfiedChannel(&service.RetryParam{
				Ctx:         c,
				TokenGroup:  relayInfo.TokenGroup,
				ModelName:   relayInfo.OriginModelName,
				RequestPath: c.Request.URL.Path,
				Retry:       common.GetPointer(0),
			})
			if selectErr != nil {
				newAPIError = types.NewError(selectErr, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
				return
			}
			if channel == nil {
				newAPIError = types.NewError(
					fmt.Errorf("No available channel for model %s under group %s", relayInfo.OriginModelName, selectGroup),
					types.ErrorCodeGetChannelFailed,
					types.ErrOptionWithSkipRetry(),
				)
				return
			}
		}
	}

	// Every dispatched request needs a real reserve. Otherwise a trusted
	// fixed-group request can become financially ambiguous after dispatch with
	// no quota held to settle against.
	relayInfo.ForcePreConsume = true
	reserveQuota := 0
	if relayInfo.TokenGroup == "auto" {
		groups, snapshotErr := service.BuildAutoGroupSnapshot(&service.RetryParam{
			Ctx:         c,
			TokenGroup:  relayInfo.TokenGroup,
			ModelName:   relayInfo.OriginModelName,
			RequestPath: c.Request.URL.Path,
			Retry:       common.GetPointer(0),
		}, relayInfo.UserGroup)
		if snapshotErr != nil {
			newAPIError = types.NewError(snapshotErr, types.ErrorCodeGetChannelFailed, types.ErrOptionWithSkipRetry())
			return
		}
		if len(groups) == 0 {
			newAPIError = types.NewError(
				fmt.Errorf("no available concrete Auto group for model %s", relayInfo.OriginModelName),
				types.ErrorCodeGetChannelFailed,
				types.ErrOptionWithSkipRetry(),
			)
			return
		}
		autoRoute, priceErr := helper.BuildAutoRouteState(c, relayInfo, groups, tokens, meta)
		if priceErr != nil {
			newAPIError = types.NewError(priceErr, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
			return
		}
		reserveQuota = autoRoute.ReservedQuota
	} else {
		priceData, priceErr := helper.ModelPriceHelper(c, relayInfo, tokens, meta)
		if priceErr != nil {
			newAPIError = types.NewError(priceErr, types.ErrorCodeModelPriceError, types.ErrOptionWithStatusCode(http.StatusBadRequest))
			return
		}
		reserveQuota = priceData.QuotaToPreConsume
	}

	// common.SetContextKey(c, constant.ContextKeyTokenCountMeta, meta)

	if reserveQuota == 0 {
		logger.LogInfo(c, fmt.Sprintf("模型 %s 免费，跳过预扣费", relayInfo.OriginModelName))
	} else {
		newAPIError = service.PreConsumeBilling(c, reserveQuota, relayInfo)
		if newAPIError != nil {
			if relayInfo.TokenGroup == "auto" &&
				(newAPIError.GetErrorCode() == types.ErrorCodeInsufficientUserQuota ||
					newAPIError.GetErrorCode() == types.ErrorCodePreConsumeTokenQuotaFailed) {
				failure := service.DescribeAutoReserveFailure(c, relayInfo, newAPIError)
				service.RecordAutoReserveFailure(c, relayInfo, failure)
				newAPIError = types.NewErrorWithStatusCode(
					errors.New(failure.Message),
					types.ErrorCodeInsufficientAutoReserve,
					http.StatusForbidden,
					types.ErrOptionWithSkipRetry(),
					types.ErrOptionWithNoRecordErrorLog(),
				)
			}
			return
		}
	}

	defer func() {
		// Only return quota if downstream failed and quota was actually pre-consumed
		if newAPIError != nil {
			newAPIError = service.NormalizeViolationFeeError(newAPIError)
			if service.ShouldRefundBaseForViolation(relayInfo, newAPIError) {
				if _, feeErr := service.RefundBaseAndChargeViolationFeeIfNeeded(c, relayInfo, newAPIError); feeErr != nil {
					common.SysLog("error refunding base or charging violation fee: " + feeErr.Error())
				}
			} else if relayInfo.Billing != nil {
				if refundErr := service.RefundBilling(c, relayInfo, newAPIError); refundErr != nil {
					common.SysLog("error refunding failed relay request: " + refundErr.Error())
				}
			}
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}
	relayInfo.RetryIndex = 0
	relayInfo.LastError = nil
	maxRetry := common.RetryTimes
	if relayInfo.TokenGroup == "auto" {
		maxRetry = len(relayInfo.AutoRoute.Candidates) - 1
	}

	for ; retryParam.GetRetry() <= maxRetry; retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = retryParam.GetRetry()
		relayInfo.ResetAttempt()
		channel, channelErr := getChannel(c, relayInfo, retryParam)
		if channelErr != nil {
			logger.LogError(c, channelErr.Error())
			newAPIError = channelErr
			if relayInfo.TokenGroup == "auto" {
				outcome := types.AttemptFinancialOutcomeNonBillable
				newAPIError.SetFinancialOutcome(outcome)
				relayInfo.AttemptFinancialOutcome = outcome
				hasNextGroup := retryParam.GetRetry() < maxRetry
				if hasNextGroup {
					relayInfo.AutoRoute.FailedGroups = append(relayInfo.AutoRoute.FailedGroups, relaycommon.AutoFailedGroup{
						Group:      relayInfo.UsingGroup,
						ErrorCode:  newAPIError.GetErrorCode(),
						StatusCode: newAPIError.StatusCode,
					})
				}
				if channel == nil {
					channel = &model.Channel{}
				}
				processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", false), newAPIError, relayInfo)
				if hasNextGroup {
					continue
				}
			}
			break
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			// Ensure consistent 413 for oversized bodies even when error occurs later (e.g., retry path)
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusRequestEntityTooLarge, types.ErrOptionWithSkipRetry())
			} else {
				newAPIError = types.NewErrorWithStatusCode(bodyErr, types.ErrorCodeReadRequestBodyFailed, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		switch relayFormat {
		case types.RelayFormatOpenAIRealtime:
			newAPIError = relay.WssHelper(c, relayInfo)
		case types.RelayFormatClaude:
			newAPIError = relay.ClaudeHelper(c, relayInfo)
		case types.RelayFormatGemini:
			newAPIError = geminiRelayHandler(c, relayInfo)
		default:
			newAPIError = relayHandler(c, relayInfo)
		}

		if newAPIError == nil {
			relayInfo.LastError = nil
			return
		}

		service.CaptureErrorBillingQuota(c, relayInfo, newAPIError)
		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		relayInfo.LastError = newAPIError
		outcome := service.ClassifyAttempt(relayInfo, newAPIError)
		if service.ShouldRefundBaseForViolation(relayInfo, newAPIError) {
			outcome = types.AttemptFinancialOutcomeNonBillable
		}
		newAPIError.SetFinancialOutcome(outcome)
		relayInfo.AttemptFinancialOutcome = outcome

		if relayInfo.TokenGroup == "auto" {
			retryable := shouldRetry(c, newAPIError, maxRetry-retryParam.GetRetry())
			if service.ShouldRetryAutoAttempt(outcome, relayInfo.AttemptDispatched, retryable, retryParam.GetRetry() < maxRetry) {
				relayInfo.AutoRoute.FailedGroups = append(relayInfo.AutoRoute.FailedGroups, relaycommon.AutoFailedGroup{
					Group:      relayInfo.UsingGroup,
					ErrorCode:  newAPIError.GetErrorCode(),
					StatusCode: newAPIError.StatusCode,
				})
				processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError, relayInfo)
				continue
			}

			processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError, relayInfo)
			if service.ShouldChargeAttempt(outcome) && !relayInfo.AttemptSettlementHandled {
				settleErr, logErr := service.SettleChargedAttemptError(c, relayInfo, newAPIError, outcome)
				if logErr != nil {
					common.SysLog("error recording charged relay attempt: " + logErr.Error())
				}
				if settleErr != nil {
					common.SysLog("error settling charged relay attempt: " + settleErr.Error())
					if !relayInfo.BillingSettled {
						// Keep the maximum reserve held when funding settlement is uncertain.
						relayInfo.Billing = nil
					}
				}
			} else if relayInfo.AttemptSettlementHandled && !relayInfo.BillingSettled {
				relayInfo.Billing = nil
			}
			break
		}

		processChannelError(c, *types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()), newAPIError)
		if service.ShouldChargeAttempt(outcome) && !relayInfo.AttemptSettlementHandled {
			settleErr, logErr := service.SettleChargedAttemptError(c, relayInfo, newAPIError, outcome)
			if logErr != nil {
				common.SysLog("error recording charged relay attempt: " + logErr.Error())
			}
			if settleErr != nil {
				common.SysLog("error settling charged relay attempt: " + settleErr.Error())
				if !relayInfo.BillingSettled {
					relayInfo.Billing = nil
				}
			}
			break
		}

		retryable := shouldRetry(c, newAPIError, common.RetryTimes-retryParam.GetRetry())
		if !service.ShouldRetryAttempt(outcome, retryable, retryParam.GetRetry() < common.RetryTimes) {
			break
		}
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}
	if newAPIError != nil {
		gopool.Go(func() {
			perfmetrics.RecordRelaySample(relayInfo, false, 0)
		})
	}
}

var upgrader = websocket.Upgrader{
	Subprotocols: []string{"realtime"}, // WS 握手支持的协议，如果有使用 Sec-WebSocket-Protocol，则必须在此声明对应的 Protocol TODO add other protocol
	CheckOrigin: func(r *http.Request) bool {
		return true // 允许跨域
	},
}

func addUsedChannel(c *gin.Context, channelId int) {
	useChannel := c.GetStringSlice("use_channel")
	useChannel = append(useChannel, fmt.Sprintf("%d", channelId))
	c.Set("use_channel", useChannel)
}

func fastTokenCountMetaForPricing(request dto.Request) *types.TokenCountMeta {
	if request == nil {
		return &types.TokenCountMeta{}
	}
	meta := &types.TokenCountMeta{
		TokenType: types.TokenTypeTokenizer,
	}
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		maxCompletionTokens := lo.FromPtrOr(r.MaxCompletionTokens, uint(0))
		maxTokens := lo.FromPtrOr(r.MaxTokens, uint(0))
		if maxCompletionTokens > maxTokens {
			meta.MaxTokens = int(maxCompletionTokens)
		} else {
			meta.MaxTokens = int(maxTokens)
		}
	case *dto.OpenAIResponsesRequest:
		meta.MaxTokens = int(lo.FromPtrOr(r.MaxOutputTokens, uint(0)))
	case *dto.ClaudeRequest:
		meta.MaxTokens = int(lo.FromPtr(r.MaxTokens))
	case *dto.ImageRequest:
		// Pricing for image requests depends on ImagePriceRatio; safe to compute even when CountToken is disabled.
		return r.GetTokenCountMeta()
	default:
		// Best-effort: leave CombineText empty to avoid large allocations.
	}
	return meta
}

func getChannel(c *gin.Context, info *relaycommon.RelayInfo, retryParam *service.RetryParam) (*model.Channel, *types.NewAPIError) {
	if info.ChannelMeta == nil {
		autoBan := c.GetBool("auto_ban")
		autoBanInt := 1
		if !autoBan {
			autoBanInt = 0
		}
		return &model.Channel{
			Id:      c.GetInt("channel_id"),
			Type:    c.GetInt("channel_type"),
			Name:    c.GetString("channel_name"),
			AutoBan: &autoBanInt,
		}, nil
	}
	channel, selectGroup, err := service.CacheGetRandomSatisfiedChannel(retryParam)
	if info.TokenGroup == "auto" {
		if len(info.AutoRoute.Candidates) == 0 {
			// Task relays freeze per-call prices after the first channel has
			// initialized the provider-specific billing ratios.
			info.UsingGroup = selectGroup
		} else if !info.ApplyAutoRouteCandidate(selectGroup) {
			return nil, types.NewError(
				fmt.Errorf("Auto group %s is not present in the frozen route plan", selectGroup),
				types.ErrorCodeGetChannelFailed,
				types.ErrOptionWithSkipRetry(),
			)
		}
		info.AutoRoute.UsedGroup = selectGroup
	}

	if err != nil {
		return nil, types.NewError(
			fmt.Errorf("Failed to get available channel for model %s under group %s (retry): %s", info.OriginModelName, selectGroup, err.Error()),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if channel == nil {
		return nil, types.NewError(
			fmt.Errorf("No available channel for model %s under group %s (retry)", info.OriginModelName, selectGroup),
			types.ErrorCodeGetChannelFailed,
			types.ErrOptionWithSkipRetry(),
		)
	}
	if info.TokenGroup != "auto" {
		info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(c, info)
	}

	newAPIError := middleware.SetupContextForSelectedChannel(c, channel, info.OriginModelName)
	if newAPIError != nil {
		return channel, newAPIError
	}
	return channel, nil
}

func shouldRetry(c *gin.Context, openaiErr *types.NewAPIError, retryTimes int) bool {
	if openaiErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if types.IsChannelError(openaiErr) {
		return true
	}
	if types.IsSkipRetryError(openaiErr) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	code := openaiErr.StatusCode
	if code >= 200 && code < 300 {
		return false
	}
	if code < 100 || code > 599 {
		return true
	}
	if operation_setting.IsAlwaysSkipRetryCode(openaiErr.GetErrorCode()) {
		return false
	}
	return operation_setting.ShouldRetryByStatusCode(code)
}

func processChannelError(c *gin.Context, channelError types.ChannelError, err *types.NewAPIError, relayInfos ...*relaycommon.RelayInfo) {
	logger.LogError(c, fmt.Sprintf("channel error (channel #%d, status code: %d): %s", channelError.ChannelId, err.StatusCode, common.LocalLogPreview(err.Error())))
	// 不要使用context获取渠道信息，异步处理时可能会出现渠道信息不一致的情况
	// do not use context to get channel info, there may be inconsistent channel info when processing asynchronously
	if service.ShouldDisableChannel(err) && channelError.AutoBan {
		gopool.Go(func() {
			service.DisableChannel(channelError, err.ErrorWithStatusCode())
		})
	}

	if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) {
		// 保存错误日志到mysql中
		userId := c.GetInt("id")
		tokenName := c.GetString("token_name")
		modelName := c.GetString("original_model")
		tokenId := c.GetInt("token_id")
		pricingGroup := errorLogPricingGroup(c)
		channelId := channelError.ChannelId
		other := make(map[string]interface{})
		if c.Request != nil && c.Request.URL != nil {
			other["request_path"] = c.Request.URL.Path
		}
		other["error_type"] = err.GetErrorType()
		other["error_code"] = err.GetErrorCode()
		other["status_code"] = err.StatusCode
		other["channel_id"] = channelId
		other["channel_name"] = channelError.ChannelName
		other["channel_type"] = channelError.ChannelType
		if outcome := err.GetFinancialOutcome(); outcome != types.AttemptFinancialOutcomeUnknown {
			other["financial_outcome"] = outcome
		}
		if len(relayInfos) > 0 {
			service.AppendAutoRoutingInfo(relayInfos[0], other)
		}
		adminInfo := make(map[string]interface{})
		adminInfo["use_channel"] = c.GetStringSlice("use_channel")
		isMultiKey := common.GetContextKeyBool(c, constant.ContextKeyChannelIsMultiKey)
		if isMultiKey {
			adminInfo["is_multi_key"] = true
			adminInfo["multi_key_index"] = common.GetContextKeyInt(c, constant.ContextKeyChannelMultiKeyIndex)
		}
		service.AppendChannelAffinityAdminInfo(c, adminInfo)
		other["admin_info"] = adminInfo
		errorLogContent, rawError, normalized := relayErrorLogContent(err)
		if normalized {
			other["raw_error"] = rawError
		}
		startTime := common.GetContextKeyTime(c, constant.ContextKeyRequestStartTime)
		if startTime.IsZero() {
			startTime = time.Now()
		}
		useTimeSeconds := int(time.Since(startTime).Seconds())
		model.RecordErrorLog(c, userId, channelId, modelName, tokenName, errorLogContent, tokenId, useTimeSeconds, common.GetContextKeyBool(c, constant.ContextKeyIsStream), pricingGroup, other)
	}

}

func errorLogPricingGroup(c *gin.Context) string {
	if value, ok := common.GetContextKey(c, constant.ContextKeyAutoGroup); ok {
		if group, ok := value.(string); ok && strings.TrimSpace(group) != "" {
			return group
		}
	}
	if group := common.GetContextKeyString(c, constant.ContextKeyUsingGroup); strings.TrimSpace(group) != "" {
		return group
	}
	return c.GetString("group")
}

func RelayMidjourney(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatMjProxy, nil, nil)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"description": fmt.Sprintf("failed to generate relay info: %s", err.Error()),
			"type":        "upstream_error",
			"code":        4,
		})
		return
	}

	var mjErr *dto.MidjourneyResponse
	switch relayInfo.RelayMode {
	case relayconstant.RelayModeMidjourneyNotify:
		mjErr = relay.RelayMidjourneyNotify(c)
	case relayconstant.RelayModeMidjourneyTaskFetch, relayconstant.RelayModeMidjourneyTaskFetchByCondition:
		mjErr = relay.RelayMidjourneyTask(c, relayInfo.RelayMode)
	case relayconstant.RelayModeMidjourneyTaskImageSeed:
		mjErr = relay.RelayMidjourneyTaskImageSeed(c)
	case relayconstant.RelayModeSwapFace:
		mjErr = relay.RelaySwapFace(c, relayInfo)
	default:
		mjErr = relay.RelayMidjourneySubmit(c, relayInfo)
	}
	//err = relayMidjourneySubmit(c, relayMode)
	log.Println(mjErr)
	if mjErr != nil {
		statusCode := http.StatusBadRequest
		if mjErr.Code == 30 {
			mjErr.Result = "The current group load is saturated. Please try again later or switch to another group."
			statusCode = http.StatusTooManyRequests
		}
		c.JSON(statusCode, gin.H{
			"description": fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result),
			"type":        "upstream_error",
			"code":        mjErr.Code,
		})
		channelId := c.GetInt("channel_id")
		logger.LogError(c, fmt.Sprintf("relay error (channel #%d, status code %d): %s", channelId, statusCode, fmt.Sprintf("%s %s", mjErr.Description, mjErr.Result)))
	}
}

func RelayNotImplemented(c *gin.Context) {
	err := types.OpenAIError{
		Message: "API not implemented",
		Type:    "new_api_error",
		Param:   "",
		Code:    "api_not_implemented",
	}
	c.JSON(http.StatusNotImplemented, gin.H{
		"error": err,
	})
}

func RelayNotFound(c *gin.Context) {
	err := types.OpenAIError{
		Message: fmt.Sprintf("Invalid URL (%s %s)", c.Request.Method, c.Request.URL.Path),
		Type:    "invalid_request_error",
		Param:   "",
		Code:    "",
	}
	c.JSON(http.StatusNotFound, gin.H{
		"error": err,
	})
}

func RelayTaskFetch(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}
	if taskErr := relay.RelayTaskFetch(c, relayInfo.RelayMode); taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func RelayTask(c *gin.Context) {
	relayInfo, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, &dto.TaskError{
			Code:       "gen_relay_info_failed",
			Message:    err.Error(),
			StatusCode: http.StatusInternalServerError,
		})
		return
	}

	if taskErr := relay.ResolveOriginTask(c, relayInfo); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	var result *relay.TaskSubmitResult
	var taskErr *dto.TaskError
	defer func() {
		if taskErr != nil && relayInfo.Billing != nil {
			if refundErr := service.RefundTaskBilling(c, relayInfo, taskErr); refundErr != nil {
				common.SysLog("error refunding failed task relay request: " + refundErr.Error())
			}
		}
	}()

	retryParam := &service.RetryParam{
		Ctx:         c,
		TokenGroup:  relayInfo.TokenGroup,
		ModelName:   relayInfo.OriginModelName,
		RequestPath: c.Request.URL.Path,
		Retry:       common.GetPointer(0),
	}

	maxRetry := common.RetryTimes
	if relayInfo.TokenGroup == "auto" && relayInfo.LockedChannel == nil {
		groups, snapshotErr := service.BuildAutoGroupSnapshot(retryParam, relayInfo.UserGroup)
		if snapshotErr != nil {
			taskErr = service.TaskErrorWrapperLocal(snapshotErr, "get_channel_failed", http.StatusServiceUnavailable)
		} else if len(groups) == 0 {
			taskErr = service.TaskErrorWrapperLocal(
				fmt.Errorf("no available concrete Auto group for model %s", relayInfo.OriginModelName),
				"get_channel_failed",
				http.StatusServiceUnavailable,
			)
		} else if contractErr := validateAutoTaskPricingContract(
			groups,
			relayInfo.OriginModelName,
			constant.TaskPlatform(c.GetString("platform")),
		); contractErr != nil {
			taskErr = service.TaskErrorWrapperLocal(contractErr, "auto_task_pricing_contract_mismatch", http.StatusBadRequest)
		} else {
			maxRetry = len(groups) - 1
		}
	}

	for ; taskErr == nil && retryParam.GetRetry() <= maxRetry; retryParam.IncreaseRetry() {
		relayInfo.RetryIndex = retryParam.GetRetry()
		relayInfo.ResetAttempt()
		var channel *model.Channel

		if lockedCh, ok := relayInfo.LockedChannel.(*model.Channel); ok && lockedCh != nil {
			channel = lockedCh
			if retryParam.GetRetry() > 0 {
				if setupErr := middleware.SetupContextForSelectedChannel(c, channel, relayInfo.OriginModelName); setupErr != nil {
					taskErr = service.TaskErrorWrapperLocal(setupErr.Err, "setup_locked_channel_failed", http.StatusInternalServerError)
					break
				}
			}
		} else {
			var channelErr *types.NewAPIError
			channel, channelErr = getChannel(c, relayInfo, retryParam)
			if channelErr != nil {
				logger.LogError(c, channelErr.Error())
				taskErr = service.TaskErrorWrapperLocal(channelErr.Err, "get_channel_failed", http.StatusInternalServerError)
				taskErr.FinancialOutcome = types.AttemptFinancialOutcomeNonBillable
				relayInfo.AttemptFinancialOutcome = types.AttemptFinancialOutcomeNonBillable
				if relayInfo.TokenGroup == "auto" {
					hasNextGroup := retryParam.GetRetry() < maxRetry
					if hasNextGroup {
						relayInfo.AutoRoute.FailedGroups = append(relayInfo.AutoRoute.FailedGroups, relaycommon.AutoFailedGroup{
							Group:      relayInfo.UsingGroup,
							ErrorCode:  channelErr.GetErrorCode(),
							StatusCode: channelErr.StatusCode,
						})
					}
					if channel == nil {
						channel = &model.Channel{}
					}
					processChannelError(c,
						*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey, "", false),
						channelErr,
						relayInfo,
					)
					if hasNextGroup {
						taskErr = nil
						continue
					}
				}
				break
			}
		}

		addUsedChannel(c, channel.Id)
		bodyStorage, bodyErr := common.GetBodyStorage(c)
		if bodyErr != nil {
			if common.IsRequestBodyTooLargeError(bodyErr) || errors.Is(bodyErr, common.ErrRequestBodyTooLarge) {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusRequestEntityTooLarge)
			} else {
				taskErr = service.TaskErrorWrapperLocal(bodyErr, "read_request_body_failed", http.StatusBadRequest)
			}
			break
		}
		c.Request.Body = io.NopCloser(bodyStorage)

		result, taskErr = relay.RelayTaskSubmit(c, relayInfo)
		if taskErr == nil {
			break
		}

		outcome := service.ClassifyTaskAttempt(relayInfo, taskErr)
		taskErr.FinancialOutcome = outcome
		relayInfo.AttemptFinancialOutcome = outcome
		apiErr := types.NewOpenAIError(taskErr.Error, types.ErrorCodeBadResponseStatusCode, taskErr.StatusCode)
		apiErr.SetFinancialOutcome(outcome)
		if !taskErr.LocalError {
			processChannelError(c,
				*types.NewChannelError(channel.Id, channel.Type, channel.Name, channel.ChannelInfo.IsMultiKey,
					common.GetContextKeyString(c, constant.ContextKeyChannelKey), channel.GetAutoBan()),
				apiErr,
				relayInfo,
			)
		}

		retryable := shouldRetryTaskRelay(c, channel.Id, taskErr, maxRetry-retryParam.GetRetry())
		shouldRetryAttempt := service.ShouldRetryAttempt(outcome, retryable, retryParam.GetRetry() < maxRetry)
		if relayInfo.TokenGroup == "auto" {
			shouldRetryAttempt = service.ShouldRetryAutoAttempt(
				outcome,
				relayInfo.AttemptDispatched,
				retryable,
				retryParam.GetRetry() < maxRetry,
			)
		}
		if shouldRetryAttempt {
			if relayInfo.TokenGroup == "auto" {
				relayInfo.AutoRoute.FailedGroups = append(relayInfo.AutoRoute.FailedGroups, relaycommon.AutoFailedGroup{
					Group:      relayInfo.UsingGroup,
					ErrorCode:  types.ErrorCode(taskErr.Code),
					StatusCode: taskErr.StatusCode,
				})
			}
			taskErr = nil
			continue
		}

		if service.ShouldChargeAttempt(outcome) {
			settleErr, logErr := service.SettleChargedAttemptError(c, relayInfo, apiErr, outcome)
			if logErr != nil {
				common.SysLog("error recording charged task relay attempt: " + logErr.Error())
			}
			if settleErr != nil {
				common.SysLog("error settling charged task relay attempt: " + settleErr.Error())
				if !relayInfo.BillingSettled {
					relayInfo.Billing = nil
				}
			}
		} else if relayInfo.AttemptSettlementHandled && !relayInfo.BillingSettled {
			relayInfo.Billing = nil
		}
		break
	}

	useChannel := c.GetStringSlice("use_channel")
	if len(useChannel) > 1 {
		retryLogStr := fmt.Sprintf("重试：%s", strings.Trim(strings.Join(strings.Fields(fmt.Sprint(useChannel)), "->"), "[]"))
		logger.LogInfo(c, retryLogStr)
	}

	// ── 成功：结算 + 日志 + 插入任务 ──
	if taskErr == nil {
		settlement := service.SettleSuccessfulBilling(c, relayInfo, result.Quota)
		if billingErr := service.LogTaskConsumption(c, relayInfo, settlement); billingErr != nil {
			common.SysError("task billing or consume log error: " + billingErr.Error())
		}

		task := model.InitTask(result.Platform, relayInfo)
		task.PrivateData.UpstreamTaskID = result.UpstreamTaskID
		applyTaskFundingSnapshot(task, relayInfo)
		task.PrivateData.BillingContext = &model.TaskBillingContext{
			ModelPrice:      relayInfo.PriceData.ModelPrice,
			GroupRatio:      relayInfo.PriceData.GroupRatioInfo.GroupRatio,
			ModelRatio:      relayInfo.PriceData.ModelRatio,
			OtherRatios:     relayInfo.PriceData.OtherRatios,
			OriginModelName: relayInfo.OriginModelName,
			PerCallBilling:  common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice,
		}
		applyTaskSettlementBaseline(task, settlement)
		task.Data = result.TaskData
		task.Action = relayInfo.Action
		if insertErr := task.Insert(); insertErr != nil {
			common.SysError("insert task error: " + insertErr.Error())
		}
	}

	if taskErr != nil {
		respondTaskError(c, taskErr)
	}
}

func applyTaskFundingSnapshot(task *model.Task, relayInfo *relaycommon.RelayInfo) {
	task.PrivateData.BillingSource = relayInfo.BillingSource
	task.PrivateData.BillingOverageSource = relayInfo.BillingOverageSource
	task.PrivateData.BillingOverageQuota = relayInfo.BillingOverageQuota
	task.PrivateData.SubscriptionId = relayInfo.SubscriptionId
	task.PrivateData.TokenId = relayInfo.TokenId
	task.PrivateData.NodeName = common.NodeName
}

func applyTaskSettlementBaseline(task *model.Task, settlement service.BillingSettlementOutcome) {
	task.Quota = settlement.Quota
	if task.PrivateData.BillingContext == nil || settlement.Err == nil {
		return
	}
	task.PrivateData.BillingContext.SettlementError = settlement.Err.Error()
	task.PrivateData.BillingContext.HeldReserve = settlement.HeldReserve
}

func validateAutoTaskPricingContract(groups []string, modelName string, fallbackPlatform constant.TaskPlatform) error {
	abilities, err := model.GetAllEnableAbilityWithChannels()
	if err != nil {
		return err
	}
	return validateAutoTaskPricingContractWithAbilities(groups, modelName, fallbackPlatform, abilities)
}

func validateAutoTaskPricingContractWithAbilities(
	groups []string,
	modelName string,
	fallbackPlatform constant.TaskPlatform,
	abilities []model.AbilityWithChannel,
) error {
	requested := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		requested[ratio_setting.PricingGroupKey(group)] = struct{}{}
	}

	maxPriority := make(map[string]int64, len(groups))
	foundPriority := make(map[string]bool, len(groups))
	for _, ability := range abilities {
		group := ratio_setting.PricingGroupKey(ability.Group)
		if ability.Model != modelName {
			continue
		}
		if _, ok := requested[group]; !ok {
			continue
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		if !foundPriority[group] || priority > maxPriority[group] {
			maxPriority[group] = priority
			foundPriority[group] = true
		}
	}

	contracts := make(map[string]struct{})
	upstreamModels := make(map[string]struct{})
	coveredGroups := make(map[string]struct{}, len(groups))
	for _, ability := range abilities {
		group := ratio_setting.PricingGroupKey(ability.Group)
		if ability.Model != modelName || !foundPriority[group] {
			continue
		}
		priority := int64(0)
		if ability.Priority != nil {
			priority = *ability.Priority
		}
		if priority != maxPriority[group] {
			continue
		}

		platform := fallbackPlatform
		if platform == "" && ability.ChannelType > 0 {
			platform = constant.TaskPlatform(strconv.Itoa(ability.ChannelType))
		}
		contract := relay.TaskPricingContract(platform)
		if contract == "" {
			return fmt.Errorf("Auto task group %s has an unsupported task pricing contract", group)
		}
		upstreamModel, err := resolveTaskMappedModel(modelName, ability.ChannelModelMapping)
		if err != nil {
			return fmt.Errorf("Auto task group %s has invalid model mapping: %w", group, err)
		}
		contracts[contract] = struct{}{}
		upstreamModels[upstreamModel] = struct{}{}
		coveredGroups[group] = struct{}{}
	}
	if len(coveredGroups) != len(requested) {
		return errors.New("Auto task pricing contract could not be verified for every selected group")
	}
	if len(contracts) > 1 {
		return errors.New("Auto task fallback requires all selected groups to use the same task pricing contract")
	}
	if len(upstreamModels) > 1 {
		return errors.New("Auto task fallback requires all selected groups to use the same mapped upstream model")
	}
	return nil
}

func resolveTaskMappedModel(modelName string, mappingJSON *string) (string, error) {
	if mappingJSON == nil || strings.TrimSpace(*mappingJSON) == "" || strings.TrimSpace(*mappingJSON) == "{}" {
		return modelName, nil
	}
	modelMapping := make(map[string]string)
	if err := common.Unmarshal([]byte(*mappingJSON), &modelMapping); err != nil {
		return "", err
	}
	current := modelName
	visited := map[string]struct{}{current: {}}
	for {
		mapped, ok := modelMapping[current]
		if !ok || strings.TrimSpace(mapped) == "" || mapped == current {
			return current, nil
		}
		if _, exists := visited[mapped]; exists {
			return "", errors.New("model mapping contains cycle")
		}
		visited[mapped] = struct{}{}
		current = mapped
	}
}

// respondTaskError 统一输出 Task 错误响应（含 429 限流提示改写）
func respondTaskError(c *gin.Context, taskErr *dto.TaskError) {
	if taskErr.StatusCode == http.StatusTooManyRequests {
		taskErr.Message = relayGroupUpstreamSaturatedMessage
	}
	c.JSON(taskErr.StatusCode, taskErr)
}

func shouldRetryTaskRelay(c *gin.Context, channelId int, taskErr *dto.TaskError, retryTimes int) bool {
	if taskErr == nil {
		return false
	}
	if service.ShouldSkipRetryAfterChannelAffinityFailure(c) {
		return false
	}
	if retryTimes <= 0 {
		return false
	}
	if _, ok := c.Get("specific_channel_id"); ok {
		return false
	}
	if taskErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	if taskErr.StatusCode == 307 {
		return true
	}
	if taskErr.StatusCode/100 == 5 {
		// 超时不重试
		if operation_setting.IsAlwaysSkipRetryStatusCode(taskErr.StatusCode) {
			return false
		}
		return true
	}
	if taskErr.StatusCode == http.StatusBadRequest {
		return false
	}
	if taskErr.StatusCode == 408 {
		// azure处理超时不重试
		return false
	}
	if taskErr.LocalError {
		return false
	}
	if taskErr.StatusCode/100 == 2 {
		return false
	}
	return true
}

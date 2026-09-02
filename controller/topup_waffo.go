package controller

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
	waffo "github.com/waffo-com/waffo-go"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/core"
	"github.com/waffo-com/waffo-go/types/order"
)

func getWaffoSDK() (*waffo.Waffo, error) {
	env := config.Sandbox
	apiKey := setting.WaffoSandboxApiKey
	privateKey := setting.WaffoSandboxPrivateKey
	publicKey := setting.WaffoSandboxPublicCert
	if !setting.WaffoSandbox {
		env = config.Production
		apiKey = setting.WaffoApiKey
		privateKey = setting.WaffoPrivateKey
		publicKey = setting.WaffoPublicCert
	}
	builder := config.NewConfigBuilder().
		APIKey(apiKey).
		PrivateKey(privateKey).
		WaffoPublicKey(publicKey).
		Environment(env)
	if setting.WaffoMerchantId != "" {
		builder = builder.MerchantID(setting.WaffoMerchantId)
	}
	cfg, err := builder.Build()
	if err != nil {
		return nil, err
	}
	return waffo.New(cfg), nil
}

func getWaffoUserEmail(user *model.User) string {
	return fmt.Sprintf("%d@examples.com", user.Id)
}

func getWaffoCurrency() string {
	if setting.WaffoCurrency != "" {
		return setting.WaffoCurrency
	}
	return "USD"
}

func buildWaffoTopUpGoodsInfo(amount float64) *order.GoodsInfo {
	appName := strings.TrimSpace(common.SystemName)
	if appName == "" {
		appName = "New API"
	}
	return &order.GoodsInfo{
		GoodsName: fmt.Sprintf("Recharge %g credits", amount),
		AppName:   appName,
	}
}

func formatWaffoAmount(amount float64, currency string) string {
	decimals := service.PaymentAmountRoundingDecimals(model.PaymentMethodWaffo, currency)
	return strconv.FormatFloat(amount, 'f', decimals, 64)
}

func isWaffoPaymentAmountRepresentable(amount float64, currency string) bool {
	decimalPlaces := int32(service.PaymentAmountRoundingDecimals(model.PaymentMethodWaffo, currency))
	return isTopUpPaymentAmountRepresentable(amount, decimalPlaces)
}

func validateWaffoPaymentResult(result *core.PaymentNotificationResult, topUp *model.TopUp) error {
	if result == nil {
		return fmt.Errorf("missing Waffo payment result")
	}
	if strings.TrimSpace(result.OrderCurrency) == "" {
		return fmt.Errorf("missing Waffo payment currency")
	}
	actual, err := decimal.NewFromString(strings.TrimSpace(result.OrderAmount))
	if err != nil {
		return fmt.Errorf("invalid Waffo payment amount: %w", err)
	}
	return service.ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderWaffo, result.OrderCurrency, actual.InexactFloat64())
}

// getWaffoPayMoney converts the user-facing amount to Waffo's configured
// settlement currency using the registry rate for non-USD configurations.
func getWaffoPayMoney(amount float64, group string) float64 {
	paymentAmount := decimal.NewFromFloat(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		paymentAmount = paymentAmount.Div(decimal.NewFromFloat(common.GetQuotaPerUnit()))
	}
	topupGroupRatio := common.GetTopupGroupRatio(getPaymentTopupGroup(model.PaymentMethodWaffo, group))
	if topupGroupRatio <= 1 {
		topupGroupRatio = 1
	}
	result := paymentAmount.
		Mul(decimal.NewFromFloat(setting.WaffoUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		InexactFloat64()
	if currency := strings.ToUpper(strings.TrimSpace(getWaffoCurrency())); currency != "" && currency != "USD" {
		rate, err := service.GetPlatformCurrencyRate(currency)
		if err != nil {
			return 0
		}
		result *= rate
	}
	return result
}

type WaffoPayRequest struct {
	Amount         float64 `json:"amount"`
	PayMethodIndex *int    `json:"pay_method_index"` // 服务端支付方式列表的索引，nil 表示由 Waffo 自动选择
	PayMethodType  string  `json:"pay_method_type"`  // Deprecated: 兼容旧前端，优先使用 pay_method_index
	PayMethodName  string  `json:"pay_method_name"`  // Deprecated: 兼容旧前端，优先使用 pay_method_index
}

func RequestWaffoAmount(c *gin.Context) {
	if !paymentMethodAllowedForUser(c, model.PaymentMethodWaffo) {
		return
	}
	var req WaffoPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodWaffo, group, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	payMoney := quote.ChargedAmount
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isWaffoPaymentAmountRepresentable(payMoney, quote.Currency) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付金额必须精确到货币最小单位"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": formatWaffoAmount(payMoney, quote.Currency)})
}

// RequestWaffoPay 创建 Waffo 支付订单
func RequestWaffoPay(c *gin.Context) {
	if !paymentMethodAllowedForUser(c, model.PaymentMethodWaffo) {
		return
	}
	if !isWaffoTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Waffo 支付未启用或 webhook 未配置"})
		return
	}

	var req WaffoPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	// 从服务端配置查找支付方式，客户端只传索引或旧字段
	var resolvedPayMethodType, resolvedPayMethodName, resolvedDisplayName string
	methods := setting.GetWaffoPayMethods()
	if req.PayMethodIndex != nil {
		// 新协议：按索引查找
		idx := *req.PayMethodIndex
		if idx < 0 || idx >= len(methods) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 支付方式索引无效 user_id=%d pay_method_index=%d method_count=%d", id, idx, len(methods)))
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付方式"})
			return
		}
		resolvedPayMethodType = methods[idx].PayMethodType
		resolvedPayMethodName = methods[idx].PayMethodName
		resolvedDisplayName = methods[idx].Name
	} else if req.PayMethodType != "" {
		// 兼容旧前端：验证客户端传的值在服务端列表中
		valid := false
		for _, m := range methods {
			if m.PayMethodType == req.PayMethodType && m.PayMethodName == req.PayMethodName {
				valid = true
				resolvedPayMethodType = m.PayMethodType
				resolvedPayMethodName = m.PayMethodName
				resolvedDisplayName = m.Name
				break
			}
		}
		if !valid {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 支付方式无效 user_id=%d pay_method_type=%s pay_method_name=%q", id, req.PayMethodType, req.PayMethodName))
			c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付方式"})
			return
		}
	}
	// resolvedPayMethodType/Name 为空时，Waffo 自动选择支付方式

	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodWaffo, group, id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	payMoney := quote.ChargedAmount
	if payMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isWaffoPaymentAmountRepresentable(payMoney, quote.Currency) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付金额必须精确到货币最小单位"})
		return
	}
	// BuildPaymentQuote captures the configured currency and conversion rate.
	// Re-reading setting.WaffoCurrency here could pair the amount with a
	// different currency if an administrator updates settings concurrently.
	currency := quote.Currency
	providerAmountText := formatWaffoAmount(payMoney, currency)
	providerAmount, err := decimal.NewFromString(providerAmountText)
	if err != nil {
		common.ApiErrorMsg(c, "Waffo payment amount is invalid")
		return
	}
	// 生成唯一订单号，paymentRequestId 与 merchantOrderId 保持一致，简化追踪
	merchantOrderId := fmt.Sprintf("WAFFO-%d-%d-%s", id, time.Now().UnixMilli(), randstr.String(6))
	paymentRequestId := merchantOrderId

	// Token 模式下归一化 Amount（存等价美元/CNY 数量，避免 RechargeWaffo 双重放大）
	amount := int64(req.Amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount = int64(req.Amount / common.GetQuotaPerUnit())
		if amount < 1 {
			amount = 1
		}
	}

	// 创建本地订单
	topUp := &model.TopUp{
		UserId:            id,
		Amount:            amount,
		RequestedAmount:   req.Amount,
		Money:             providerAmount.InexactFloat64(),
		TradeNo:           merchantOrderId,
		PaymentMethod:     model.PaymentMethodWaffo,
		PaymentMethodName: resolvedDisplayName,
		PaymentProvider:   model.PaymentProviderWaffo,
		QuotaToAdd:        getTopUpQuotaToAdd(req.Amount),
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	// Keep the quote's immutable USD base/rate for accounting. Only the
	// provider charged amount is rounded; callbacks validate that exact
	// settlement amount separately.
	service.ApplyPaymentQuoteSnapshot(topUp, quote, providerAmount.InexactFloat64())
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 创建充值订单失败 user_id=%d trade_no=%s amount=%g error=%q", id, merchantOrderId, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo SDK 初始化失败 user_id=%d trade_no=%s error=%q", id, merchantOrderId, err.Error()))
		topUp.Status = common.TopUpStatusFailed
		_ = topUp.Update()
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付配置错误"})
		return
	}

	callbackAddr := service.GetCallbackAddress()
	notifyUrl := callbackAddr + "/api/waffo/webhook"
	if setting.WaffoNotifyUrl != "" {
		notifyUrl = setting.WaffoNotifyUrl
	}
	returnUrl := paymentReturnPath("/console/topup?show_history=true")
	if setting.WaffoReturnUrl != "" {
		returnUrl = setting.WaffoReturnUrl
	}

	goodsInfo := buildWaffoTopUpGoodsInfo(req.Amount)
	createParams := &order.CreateOrderParams{
		PaymentRequestID: paymentRequestId,
		MerchantOrderID:  merchantOrderId,
		OrderAmount:      providerAmountText,
		OrderCurrency:    currency,
		OrderDescription: goodsInfo.GoodsName,
		OrderRequestedAt: time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		NotifyURL:        notifyUrl,
		MerchantInfo: &order.MerchantInfo{
			MerchantID: setting.WaffoMerchantId,
		},
		UserInfo: &order.UserInfo{
			UserID:       strconv.Itoa(user.Id),
			UserEmail:    getWaffoUserEmail(user),
			UserTerminal: "WEB",
		},
		PaymentInfo: &order.PaymentInfo{
			ProductName:   "ONE_TIME_PAYMENT",
			PayMethodType: resolvedPayMethodType,
			PayMethodName: resolvedPayMethodName,
		},
		GoodsInfo:          goodsInfo,
		SuccessRedirectURL: returnUrl,
		FailedRedirectURL:  returnUrl,
	}
	resp, err := sdk.Order().Create(c.Request.Context(), createParams, nil)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 创建订单失败 user_id=%d trade_no=%s error=%q", id, merchantOrderId, err.Error()))
		// The request may have reached Waffo before the transport failed. Keep
		// the local order pending so a late webhook/reconciliation can settle it.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if resp == nil || !resp.IsSuccess() {
		if resp == nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 创建订单返回空响应 user_id=%d trade_no=%s", id, merchantOrderId))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 创建订单业务失败 user_id=%d trade_no=%s code=%s message=%q response=%q", id, merchantOrderId, resp.Code, resp.Message, common.GetJsonString(resp)))
		}
		// Waffo does not expose a typed terminal-rejection contract. Treat an
		// unsuccessful or incomplete response as ambiguous and retain pending.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	orderData := resp.GetData()
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo 充值订单创建成功 user_id=%d trade_no=%s amount=%g money=%.2f pay_method_type=%s pay_method_name=%q", id, merchantOrderId, req.Amount, payMoney, resolvedPayMethodType, resolvedPayMethodName))

	paymentUrl := orderData.FetchRedirectURL()
	if paymentUrl == "" {
		paymentUrl = orderData.OrderAction
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"payment_url": paymentUrl,
			"order_id":    merchantOrderId,
		},
	})
}

// webhookPayloadWithSubInfo 扩展 PAYMENT_NOTIFICATION，包含 SDK 未定义的 subscriptionInfo 字段
type webhookPayloadWithSubInfo struct {
	EventType string `json:"eventType"`
	Result    struct {
		core.PaymentNotificationResult
		SubscriptionInfo *webhookSubscriptionInfo `json:"subscriptionInfo,omitempty"`
	} `json:"result"`
}

type webhookSubscriptionInfo struct {
	Period              string `json:"period,omitempty"`
	MerchantRequest     string `json:"merchantRequest,omitempty"`
	SubscriptionID      string `json:"subscriptionId,omitempty"`
	SubscriptionRequest string `json:"subscriptionRequest,omitempty"`
}

// WaffoWebhook 处理 Waffo 回调通知（支付/退款/订阅）
func WaffoWebhook(c *gin.Context) {
	if !isWaffoWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	sdk, err := getWaffoSDK()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo webhook SDK 初始化失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	wh := sdk.Webhook()
	bodyStr := string(bodyBytes)
	signature := c.GetHeader("X-SIGNATURE")
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, bodyStr))

	// 验证请求签名
	if !wh.VerifySignature(bodyStr, signature) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo webhook 验签失败 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, bodyStr))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	var event core.WebhookEvent
	if err := common.Unmarshal(bodyBytes, &event); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo webhook 解析失败 path=%q client_ip=%s error=%q body=%q", c.Request.RequestURI, c.ClientIP(), err.Error(), bodyStr))
		sendWaffoWebhookResponse(c, wh, false, "invalid payload")
		return
	}

	switch event.EventType {
	case core.EventPayment:
		// 解析为扩展类型，区分普通支付和订阅支付
		var payload webhookPayloadWithSubInfo
		if err := common.Unmarshal(bodyBytes, &payload); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 支付回调载荷解析失败 event_type=%s client_ip=%s error=%q body=%q", event.EventType, c.ClientIP(), err.Error(), bodyStr))
			sendWaffoWebhookResponse(c, wh, false, "invalid payment payload")
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo webhook 验签并解析成功 event_type=%s merchant_order_id=%s order_status=%s client_ip=%s", event.EventType, payload.Result.MerchantOrderID, payload.Result.OrderStatus, c.ClientIP()))
		handleWaffoPayment(c, wh, &payload.Result.PaymentNotificationResult)
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo webhook 忽略事件 event_type=%s client_ip=%s", event.EventType, c.ClientIP()))
		sendWaffoWebhookResponse(c, wh, true, "")
	}
}

// handleWaffoPayment 处理支付完成通知
func handleWaffoPayment(c *gin.Context, wh *core.WebhookHandler, result *core.PaymentNotificationResult) {
	if result.OrderStatus != "PAY_SUCCESS" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo 订单状态非成功，忽略充值 trade_no=%s order_status=%s client_ip=%s", result.MerchantOrderID, result.OrderStatus, c.ClientIP()))
		// Intermediate statuses are acknowledgements only. The provider can
		// send PAY_SUCCESS after these callbacks, so never close the local order
		// until Waffo reports its explicit terminal ORDER_CLOSE status.
		if shouldMarkWaffoTopUpFailed(result.OrderStatus) && result.MerchantOrderID != "" {
			if err := model.UpdatePendingTopUpStatus(result.MerchantOrderID, model.PaymentProviderWaffo, common.TopUpStatusFailed); err != nil {
				if errors.Is(err, model.ErrPaymentMethodMismatch) {
					sendWaffoWebhookResponse(c, wh, true, "")
					return
				}
				if !errors.Is(err, model.ErrTopUpNotFound) && !errors.Is(err, model.ErrTopUpStatusInvalid) {
					logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 标记失败订单状态失败 trade_no=%s error=%q", result.MerchantOrderID, err.Error()))
					sendWaffoWebhookResponse(c, wh, false, "temporary order update failure")
					return
				}
			}
		}
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	}

	merchantOrderId := result.MerchantOrderID
	topUp, lookupErr := model.GetTopUpByTradeNoWithError(merchantOrderId)
	if lookupErr != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 充值订单不存在或支付网关不匹配 trade_no=%s client_ip=%s", merchantOrderId, c.ClientIP()))
		if errors.Is(lookupErr, model.ErrTopUpNotFound) {
			sendWaffoWebhookResponse(c, wh, false, "top-up not found or provider mismatch")
		} else {
			sendWaffoWebhookResponse(c, wh, false, "temporary order lookup failure")
		}
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderWaffo {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 充值订单不存在或支付网关不匹配 trade_no=%s client_ip=%s", merchantOrderId, c.ClientIP()))
		// The webhook signature is valid, but this order belongs to another
		// provider. Acknowledge the permanent mismatch without crediting it.
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	}
	if topUp.Status == common.TopUpStatusSuccess || topUp.Status == common.TopUpStatusFailed || topUp.Status == common.TopUpStatusExpired {
		sendWaffoWebhookResponse(c, wh, true, "")
		return
	}
	if err := validateWaffoPaymentResult(result, topUp); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Waffo 回调金额与订单快照不匹配 trade_no=%s client_ip=%s error=%q", merchantOrderId, c.ClientIP(), err.Error()))
		if service.IsPermanentPaymentSnapshotError(err) {
			// The signed callback is valid and the order exists, but the provider
			// reports a payment that can never satisfy this immutable snapshot.
			// Acknowledge it without crediting the account to stop endless retries.
			sendWaffoWebhookResponse(c, wh, true, "")
		} else {
			// Malformed payloads and transient validation/DB failures remain
			// negative acknowledgements so they can be corrected or retried.
			sendWaffoWebhookResponse(c, wh, false, err.Error())
		}
		return
	}

	LockOrder(merchantOrderId)
	defer UnlockOrder(merchantOrderId)

	if err := model.RechargeWaffo(merchantOrderId, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Waffo 充值处理失败 trade_no=%s client_ip=%s error=%q", merchantOrderId, c.ClientIP(), err.Error()))
		if model.IsPermanentTopUpError(err, topUp) {
			sendWaffoWebhookResponse(c, wh, true, "")
			return
		}
		sendWaffoWebhookResponse(c, wh, false, err.Error())
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Waffo 充值成功 trade_no=%s client_ip=%s", merchantOrderId, c.ClientIP()))
	sendWaffoWebhookResponse(c, wh, true, "")
}

func shouldMarkWaffoTopUpFailed(status string) bool {
	return strings.TrimSpace(status) == core.OrderStatusOrderClose
}

// sendWaffoWebhookResponse 发送签名响应
func sendWaffoWebhookResponse(c *gin.Context, wh *core.WebhookHandler, success bool, msg string) {
	var body, sig string
	if success {
		body, sig = wh.BuildSuccessResponse()
	} else {
		body, sig = wh.BuildFailedResponse(msg)
	}
	c.Header("X-SIGNATURE", sig)
	c.Data(http.StatusOK, "application/json", []byte(body))
}

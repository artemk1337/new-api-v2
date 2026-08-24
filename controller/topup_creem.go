package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
)

const CreemSignatureHeader = "creem-signature"

var creemAdaptor = &CreemAdaptor{}

type creemConfigContextKey struct{}

func withCreemConfig(ctx context.Context, config setting.CreemConfig) context.Context {
	return context.WithValue(ctx, creemConfigContextKey{}, config)
}

func creemConfigFromContext(ctx context.Context) setting.CreemConfig {
	if config, ok := ctx.Value(creemConfigContextKey{}).(setting.CreemConfig); ok {
		return config
	}
	return setting.GetCreemConfig()
}

// 生成HMAC-SHA256签名
func generateCreemSignature(payload string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// 验证Creem webhook签名
func verifyCreemSignature(payload string, signature string, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		config := setting.GetCreemConfig()
		logger.LogWarn(context.Background(), fmt.Sprintf("Creem webhook secret 未配置 test_mode=%t signature=%q body=%q", config.TestMode, signature, payload))
		return false
	}

	expectedSignature := generateCreemSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func validateCreemPayment(topUp *model.TopUp, event *CreemWebhookEvent) error {
	if topUp == nil || event == nil {
		return fmt.Errorf("missing Creem top-up or event")
	}
	if event.Object.Order.AmountPaid <= 0 {
		// A missing/invalid amount is a malformed callback, not a valid payment
		// for a different immutable snapshot. Keep it retryable/rejected rather
		// than acknowledging a payload that cannot be settled safely.
		return fmt.Errorf("Creem payment amount is not positive")
	}
	if strings.TrimSpace(event.Object.Order.Currency) == "" {
		return fmt.Errorf("Creem payment currency is missing")
	}
	expectedProductID := strings.TrimSpace(topUp.CreemProductID)
	// New orders persist the selected catalog product and must bind callbacks
	// to it. Older pending rows predate this snapshot field; retain their
	// established amount/currency-only validation so a valid payment is not
	// stranded solely because the migration cannot reconstruct its product.
	if expectedProductID != "" {
		orderProduct := strings.TrimSpace(event.Object.Order.Product)
		objectProduct := strings.TrimSpace(event.Object.Product.Id)
		if orderProduct == "" && objectProduct == "" {
			return fmt.Errorf("Creem payment product is missing")
		}
		if orderProduct != "" && objectProduct != "" && orderProduct != objectProduct {
			return fmt.Errorf("%w: Creem product mismatch", service.ErrPaymentSnapshotValidation)
		}
		callbackProductID := creemWebhookProductID(event)
		if callbackProductID != expectedProductID {
			return fmt.Errorf("%w: Creem product mismatch", service.ErrPaymentSnapshotValidation)
		}
	}
	// Creem reports monetary amounts in the smallest currency unit while the
	// product configuration and TopUp snapshot use major units. Use the shared
	// ISO precision mapping so zero- and three-decimal currencies are handled
	// identically by checkout validation and webhook settlement.
	actualAmount := model.SubscriptionProviderMajorAmount(int64(event.Object.Order.AmountPaid), event.Object.Order.Currency)
	return service.ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderCreem, event.Object.Order.Currency, actualAmount)
}

// validateCreemWebhookMode binds callbacks to the configured Creem
// environment. Creem historically omitted mode from some webhook payloads,
// so an empty mode remains compatible; when present, it must identify the
// same test/live environment as the current configuration. Accept the
// provider's common live aliases as equivalent for backwards compatibility.
func validateCreemWebhookMode(event *CreemWebhookEvent, config setting.CreemConfig) error {
	if event == nil {
		return errors.New("missing Creem webhook event")
	}
	modes := []string{
		strings.TrimSpace(event.Mode),
		strings.TrimSpace(event.Object.Mode),
		strings.TrimSpace(event.Object.Order.Mode),
		strings.TrimSpace(event.Object.Product.Mode),
		strings.TrimSpace(event.Object.Customer.Mode),
	}
	expected := "live"
	if config.TestMode {
		expected = "test"
	}
	for _, rawMode := range modes {
		if rawMode == "" {
			continue
		}
		mode := strings.ToLower(rawMode)
		matches := mode == expected
		if expected == "live" {
			matches = mode == "live" || mode == "prod" || mode == "production"
		}
		if !matches {
			return fmt.Errorf("Creem webhook mode %q does not match configured %s environment", rawMode, expected)
		}
	}
	return nil
}

// creemWebhookProductID accepts either product location used by Creem's
// webhook schema, but rejects a payload where both locations disagree.
func creemWebhookProductID(event *CreemWebhookEvent) string {
	if event == nil {
		return ""
	}
	orderProduct := strings.TrimSpace(event.Object.Order.Product)
	objectProduct := strings.TrimSpace(event.Object.Product.Id)
	if orderProduct != "" && objectProduct != "" && orderProduct != objectProduct {
		return ""
	}
	if objectProduct != "" {
		return objectProduct
	}
	return orderProduct
}

type CreemPayRequest struct {
	ProductId     string `json:"product_id"`
	PaymentMethod string `json:"payment_method"`
}

type CreemProduct struct {
	ProductId string  `json:"productId"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	Quota     int64   `json:"quota"`
}

type creemProviderProduct struct {
	Id       string `json:"id"`
	Price    int64  `json:"price"`
	Currency string `json:"currency"`
}

var creemProductLookup = fetchCreemProduct

type CreemAdaptor struct {
}

func validateCreemTopUpProduct(product *CreemProduct) error {
	if product == nil || strings.TrimSpace(product.ProductId) == "" {
		return fmt.Errorf("Creem product id is required")
	}
	if product.Quota <= 0 {
		return fmt.Errorf("Creem product quota must be greater than zero")
	}
	return nil
}

func applyCreemPaymentSnapshot(topUp *model.TopUp, product *CreemProduct, rate float64) {
	// Creem's catalog price is expressed in the provider currency. Keep the
	// accounting base in USD, like the other payment snapshots, while retaining
	// the exact provider amount for callback validation. The configured quota is
	// authoritative for catalog products and must not be recalculated from the
	// price or current exchange rate.
	topUp.RequestedAmount = product.Price
	topUp.CreemProductID = strings.TrimSpace(product.ProductId)
	service.ApplyPaymentSnapshot(topUp, product.Currency, rate, product.Price/rate, 1, product.Price)
	topUp.QuotaToAdd = int(product.Quota)
}

func (*CreemAdaptor) RequestPay(c *gin.Context, req *CreemPayRequest) {
	config := setting.GetCreemConfig()
	if !isCreemTopUpEnabledForConfig(config) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem payments are not enabled"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodCreem {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	if req.ProductId == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请选择产品"})
		return
	}

	// 解析产品列表
	var products []CreemProduct
	err := json.Unmarshal([]byte(config.Products), &products)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 产品配置解析失败 user_id=%d error=%q", c.GetInt("id"), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品配置错误"})
		return
	}

	// 查找对应的产品
	var selectedProduct *CreemProduct
	for _, product := range products {
		if product.ProductId == req.ProductId {
			selectedProduct = &product
			break
		}
	}

	if selectedProduct == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品不存在"})
		return
	}
	if err := validateCreemTopUpProduct(selectedProduct); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 产品充值额度无效 product_id=%s quota=%d", selectedProduct.ProductId, selectedProduct.Quota))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem 产品充值额度必须大于零"})
		return
	}
	providerProduct, err := creemProductLookup(withCreemConfig(c.Request.Context(), config), selectedProduct.ProductId)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 产品校验失败 product_id=%s error=%q", selectedProduct.ProductId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem 产品暂不可用"})
		return
	}
	if err := validateCreemProductContract(selectedProduct, providerProduct); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 产品配置与 provider 不匹配 product_id=%s error=%q", selectedProduct.ProductId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Creem 产品价格或货币不匹配"})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 用户不存在 user_id=%d error=%v", id, err))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	// 生成唯一的订单引用ID
	reference := fmt.Sprintf("creem-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	// 先创建订单记录，使用产品配置的金额和充值额度
	topUp := &model.TopUp{
		UserId: id,
		Amount: selectedProduct.Quota, // 充值额度
		// RequestedAmount is the provider-currency catalog price. It marks this
		// order as snapshot-backed so settlement can safely become terminal when
		// the user was hard-deleted before Creem's callback arrived.
		RequestedAmount:   selectedProduct.Price,
		Money:             selectedProduct.Price, // 支付金额
		TradeNo:           referenceId,
		PaymentMethod:     model.PaymentMethodCreem,
		PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodCreem),
		PaymentProvider:   model.PaymentProviderCreem,
		CreemProductID:    selectedProduct.ProductId,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	currency := strings.ToUpper(strings.TrimSpace(selectedProduct.Currency))
	rate, rateErr := service.GetPlatformCurrencyRate(currency)
	if rateErr != nil {
		common.ApiErrorMsg(c, "Creem product currency has no configured exchange rate")
		return
	}
	applyCreemPaymentSnapshot(topUp, selectedProduct, rate)
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建充值订单失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// 创建支付链接，传入用户邮箱
	checkoutUrl, err := genCreemLink(c.Request.Context(), config, referenceId, selectedProduct, user.Email, user.Username)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建支付链接失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		// A terminal provider response (for example an invalid product or
		// credentials) cannot later produce a valid checkout. Transport and
		// malformed responses remain pending because Creem may have accepted
		// the request before the client observed the error.
		if failErr := settleCreemTopUpCreateFailure(referenceId, err); failErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("Creem завершение неуспешного заказа не сохранено trade_no=%s error=%q", referenceId, failErr.Error()))
		}
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值订单创建成功 user_id=%d trade_no=%s product_id=%s product_name=%q quota=%d money=%.2f", id, referenceId, selectedProduct.ProductId, selectedProduct.Name, selectedProduct.Quota, selectedProduct.Price))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url": checkoutUrl,
			"order_id":     referenceId,
		},
	})
}

func RequestCreemPay(c *gin.Context) {
	var req CreemPayRequest

	// 读取body内容用于打印，同时保留原始数据供后续使用
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 支付请求读取失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 支付请求已收到 user_id=%d body=%q", c.GetInt("id"), string(bodyBytes)))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	err = c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	creemAdaptor.RequestPay(c, &req)
}

// 新的Creem Webhook结构体，匹配实际的webhook数据格式
type CreemWebhookEvent struct {
	Id        string `json:"id"`
	EventType string `json:"eventType"`
	Mode      string `json:"mode,omitempty"`
	CreatedAt int64  `json:"created_at"`
	Object    struct {
		Id        string `json:"id"`
		Object    string `json:"object"`
		RequestId string `json:"request_id"`
		Order     struct {
			Object      string `json:"object"`
			Id          string `json:"id"`
			Customer    string `json:"customer"`
			Product     string `json:"product"`
			Amount      int    `json:"amount"`
			Currency    string `json:"currency"`
			SubTotal    int    `json:"sub_total"`
			TaxAmount   int    `json:"tax_amount"`
			AmountDue   int    `json:"amount_due"`
			AmountPaid  int    `json:"amount_paid"`
			Status      string `json:"status"`
			Type        string `json:"type"`
			Transaction string `json:"transaction"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
			Mode        string `json:"mode"`
		} `json:"order"`
		Product struct {
			Id                string  `json:"id"`
			Object            string  `json:"object"`
			Name              string  `json:"name"`
			Description       string  `json:"description"`
			Price             int     `json:"price"`
			Currency          string  `json:"currency"`
			BillingType       string  `json:"billing_type"`
			BillingPeriod     string  `json:"billing_period"`
			Status            string  `json:"status"`
			TaxMode           string  `json:"tax_mode"`
			TaxCategory       string  `json:"tax_category"`
			DefaultSuccessUrl *string `json:"default_success_url"`
			CreatedAt         string  `json:"created_at"`
			UpdatedAt         string  `json:"updated_at"`
			Mode              string  `json:"mode"`
		} `json:"product"`
		Units    int `json:"units"`
		Customer struct {
			Id        string `json:"id"`
			Object    string `json:"object"`
			Email     string `json:"email"`
			Name      string `json:"name"`
			Country   string `json:"country"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Mode      string `json:"mode"`
		} `json:"customer"`
		Status   string            `json:"status"`
		Metadata map[string]string `json:"metadata"`
		Mode     string            `json:"mode"`
	} `json:"object"`
}

func CreemWebhook(c *gin.Context) {
	config := setting.GetCreemConfig()
	if !isCreemWebhookConfiguredForConfig(config) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// 读取body内容用于打印，同时保留原始数据供后续使用
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 获取签名头
	signature := c.GetHeader(CreemSignatureHeader)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes)))
	if signature == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少签名 path=%q client_ip=%s body=%q", c.Request.RequestURI, c.ClientIP(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 验证签名
	if !verifyCreemSignatureWithConfig(string(bodyBytes), signature, config) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 验签失败 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes)))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 验签成功 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 解析新格式的webhook数据
	var webhookEvent CreemWebhookEvent
	if err := c.ShouldBindJSON(&webhookEvent); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 解析失败 path=%q client_ip=%s error=%q body=%q", c.Request.RequestURI, c.ClientIP(), err.Error(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := validateCreemWebhookMode(&webhookEvent, config); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 环境不匹配 event_id=%s error=%q", webhookEvent.Id, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 解析成功 event_type=%s event_id=%s request_id=%s order_id=%s order_status=%s", webhookEvent.EventType, webhookEvent.Id, webhookEvent.Object.RequestId, webhookEvent.Object.Order.Id, webhookEvent.Object.Order.Status))

	// 根据事件类型处理不同的webhook
	switch webhookEvent.EventType {
	case "checkout.completed":
		handleCheckoutCompleted(c, &webhookEvent)
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 忽略事件 event_type=%s event_id=%s", webhookEvent.EventType, webhookEvent.Id))
		c.Status(http.StatusOK)
	}
}

// 处理支付完成事件
func handleCheckoutCompleted(c *gin.Context, event *CreemWebhookEvent) {
	// 验证订单状态
	if event.Object.Order.Status != "paid" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 订单状态未支付，忽略处理 request_id=%s order_id=%s order_status=%s", event.Object.RequestId, event.Object.Order.Id, event.Object.Order.Status))
		c.Status(http.StatusOK)
		return
	}

	// 获取引用ID（这是我们创建订单时传递的request_id）
	referenceId := event.Object.RequestId
	if referenceId == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少 request_id event_id=%s order_id=%s", event.Id, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// Try complete subscription order first
	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if err := model.CompleteSubscriptionOrder(referenceId, common.GetJsonString(event), model.PaymentProviderCreem, ""); err == nil {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理成功 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
		c.Status(http.StatusOK)
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		if errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) {
			c.Status(http.StatusOK)
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理失败 trade_no=%s creem_order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
		if model.IsPermanentSubscriptionOrderError(err) {
			// The callback was authenticated and the order was found, but its
			// immutable snapshot is permanently incompatible (amount, currency or
			// product). Acknowledge it without creating a subscription so Creem
			// does not retry the same invalid payment forever.
			c.Status(http.StatusOK)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// 验证订单类型，目前只处理一次性付款（充值）
	if event.Object.Order.Type != "onetime" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 暂不支持该订单类型，忽略处理 request_id=%s creem_order_id=%s order_type=%s", referenceId, event.Object.Order.Id, event.Object.Order.Type))
		c.Status(http.StatusOK)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 支付完成回调 trade_no=%s creem_order_id=%s amount_paid=%d currency=%s product_name=%q customer_email=%q customer_name=%q", referenceId, event.Object.Order.Id, event.Object.Order.AmountPaid, event.Object.Order.Currency, event.Object.Product.Name, event.Object.Customer.Email, event.Object.Customer.Name))

	// 查询本地订单确认存在
	topUp, lookupErr := model.GetTopUpByTradeNoWithError(referenceId)
	if lookupErr != nil {
		if errors.Is(lookupErr, model.ErrTopUpNotFound) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值订单不存在 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 查询充值订单失败 trade_no=%s creem_order_id=%s error=%q", referenceId, event.Object.Order.Id, lookupErr.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if topUp == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值订单不存在 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if topUp.PaymentProvider != model.PaymentProviderCreem {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值订单支付渠道不匹配 trade_no=%s expected=%s actual=%s", referenceId, model.PaymentProviderCreem, topUp.PaymentProvider))
		// The callback is authenticated, but this local order belongs to another
		// provider. It can never be settled here, so acknowledge without mutation.
		c.Status(http.StatusOK)
		return
	}

	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值订单状态非 pending，忽略处理 trade_no=%s status=%s creem_order_id=%s", referenceId, topUp.Status, event.Object.Order.Id))
		c.Status(http.StatusOK) // 已处理过的订单，返回成功避免重复处理
		return
	}
	if err := validateCreemPayment(topUp, event); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 回调金额/货币与订单快照不匹配 trade_no=%s creem_order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
		if service.IsPermanentPaymentSnapshotError(err) {
			c.Status(http.StatusOK)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	// 处理充值，传入客户邮箱和姓名信息
	customerEmail := event.Object.Customer.Email
	customerName := event.Object.Customer.Name

	// 防护性检查，确保邮箱和姓名不为空字符串
	if customerEmail == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 回调客户邮箱为空 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
	}
	if customerName == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 回调客户姓名为空 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
	}

	err := model.RechargeCreem(referenceId, customerEmail, customerName, c.ClientIP())
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 充值处理失败 trade_no=%s creem_order_id=%s client_ip=%s error=%q", referenceId, event.Object.Order.Id, c.ClientIP(), err.Error()))
		if model.IsPermanentTopUpError(err, topUp) {
			// A verified payment for a snapshot-backed order whose user was
			// hard-deleted cannot be credited. RechargeCreem closes that order
			// as failed; acknowledge the webhook so Creem does not retry it.
			c.Status(http.StatusOK)
			return
		}
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值成功 trade_no=%s creem_order_id=%s quota=%d money=%.2f client_ip=%s", referenceId, event.Object.Order.Id, topUp.Amount, topUp.Money, c.ClientIP()))
	c.Status(http.StatusOK)
}

func isPermanentCreemCreateError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "http status 408") || strings.Contains(message, "http status 429") {
		return false
	}
	for _, code := range []string{"http status 400", "http status 401", "http status 403", "http status 404", "http status 409"} {
		if strings.Contains(message, strings.ToLower(code)) {
			return true
		}
	}
	return false
}

func settleCreemTopUpCreateFailure(tradeNo string, err error) error {
	if !isPermanentCreemCreateError(err) {
		return nil
	}
	return model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderCreem, common.TopUpStatusFailed)
}

type CreemCheckoutRequest struct {
	ProductId string `json:"product_id"`
	RequestId string `json:"request_id"`
	Customer  struct {
		Email string `json:"email"`
	} `json:"customer"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CreemCheckoutResponse struct {
	CheckoutUrl string `json:"checkout_url"`
	Id          string `json:"id"`
}

func verifyCreemSignatureWithConfig(payload string, signature string, config setting.CreemConfig) bool {
	if strings.TrimSpace(config.WebhookSecret) == "" {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(generateCreemSignature(payload, config.WebhookSecret)))
}

func genCreemLink(ctx context.Context, config setting.CreemConfig, referenceId string, product *CreemProduct, email string, username string) (string, error) {
	if config.APIKey == "" {
		return "", fmt.Errorf("未配置Creem API密钥")
	}

	// 根据测试模式选择 API 端点
	apiUrl := "https://api.creem.io/v1/checkouts"
	if config.TestMode {
		apiUrl = "https://test-api.creem.io/v1/checkouts"
		logger.LogInfo(ctx, fmt.Sprintf("Creem 使用测试环境 api_url=%s", apiUrl))
	}

	// 构建请求数据，确保包含用户邮箱
	requestData := CreemCheckoutRequest{
		ProductId: product.ProductId,
		RequestId: referenceId, // 这个作为订单ID传递给Creem
		Customer: struct {
			Email string `json:"email"`
		}{
			Email: email, // 用户邮箱会在支付页面预填充
		},
		Metadata: map[string]string{
			"username":     username,
			"reference_id": referenceId,
			"product_name": product.Name,
			"quota":        fmt.Sprintf("%d", product.Quota),
		},
	}

	// 序列化请求数据
	jsonData, err := json.Marshal(requestData)
	if err != nil {
		return "", fmt.Errorf("序列化请求数据失败: %v", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", config.APIKey)

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付请求已发送 api_url=%s product_id=%s email=%q trade_no=%s", apiUrl, product.ProductId, email, referenceId))

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem API 响应已收到 trade_no=%s status_code=%d body=%q", referenceId, resp.StatusCode, string(body)))

	// 检查响应状态
	if resp.StatusCode/100 != 2 {
		return "", fmt.Errorf("Creem API http status %d ", resp.StatusCode)
	}
	// 解析响应
	var checkoutResp CreemCheckoutResponse
	err = json.Unmarshal(body, &checkoutResp)
	if err != nil {
		return "", fmt.Errorf("解析响应失败: %v", err)
	}

	if checkoutResp.CheckoutUrl == "" {
		return "", fmt.Errorf("Creem API resp no checkout url ")
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付链接创建成功 trade_no=%s response_id=%s checkout_url=%q", referenceId, checkoutResp.Id, checkoutResp.CheckoutUrl))
	return checkoutResp.CheckoutUrl, nil
}

func fetchCreemProduct(ctx context.Context, productID string) (*creemProviderProduct, error) {
	config := creemConfigFromContext(ctx)
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("Creem API key is not configured")
	}
	productID = strings.TrimSpace(productID)
	if productID == "" {
		return nil, fmt.Errorf("Creem product id is empty")
	}
	apiURL := "https://api.creem.io/v1/products/" + url.PathEscape(productID)
	if config.TestMode {
		apiURL = "https://test-api.creem.io/v1/products/" + url.PathEscape(productID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create Creem product request: %w", err)
	}
	req.Header.Set("x-api-key", config.APIKey)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch Creem product: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("Creem product API http status %d", resp.StatusCode)
	}
	var product creemProviderProduct
	if err := common.DecodeJson(resp.Body, &product); err != nil {
		return nil, fmt.Errorf("decode Creem product: %w", err)
	}
	return &product, nil
}

func validateCreemProductContract(local *CreemProduct, provider *creemProviderProduct) error {
	if local == nil || provider == nil {
		return fmt.Errorf("Creem product contract is missing")
	}
	if strings.TrimSpace(local.ProductId) == "" || strings.TrimSpace(provider.Id) == "" ||
		!strings.EqualFold(strings.TrimSpace(local.ProductId), strings.TrimSpace(provider.Id)) {
		return fmt.Errorf("product id mismatch")
	}
	if local.Price <= 0 || provider.Price <= 0 || model.SubscriptionProviderMinorAmount(local.Price, local.Currency) != provider.Price {
		return fmt.Errorf("product price mismatch")
	}
	if strings.TrimSpace(local.Currency) == "" || strings.TrimSpace(provider.Currency) == "" ||
		!strings.EqualFold(strings.TrimSpace(local.Currency), strings.TrimSpace(provider.Currency)) {
		return fmt.Errorf("product currency mismatch")
	}
	if !model.SubscriptionProviderAmountRepresentable(local.Price, local.Currency) {
		return fmt.Errorf("product price is not representable in currency minor units")
	}
	return nil
}

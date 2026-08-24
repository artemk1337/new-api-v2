package controller

import (
	"context"
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
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

var stripeAdaptor = &StripeAdaptor{}

// stripePermanentWebhookError marks a callback that cannot become valid by
// retrying (missing/mismatched order or immutable payment snapshot mismatch).
// Provider callbacks should receive 4xx for these cases and 5xx for local,
// retryable settlement failures.
var stripePermanentWebhookError = errors.New("permanent Stripe webhook error")

func markStripeWebhookPermanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", stripePermanentWebhookError, err)
}

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	// Amount is the quantity of units to purchase.
	Amount float64 `json:"amount"`
	// PaymentMethod specifies the payment method (e.g., "stripe").
	PaymentMethod string `json:"payment_method"`
	// SuccessURL is the optional custom URL to redirect after successful payment.
	// If empty, defaults to the server's console log page.
	SuccessURL string `json:"success_url,omitempty"`
	// CancelURL is the optional custom URL to redirect when payment is canceled.
	// If empty, defaults to the server's console topup page.
	CancelURL string `json:"cancel_url,omitempty"`
}

type StripeAdaptor struct {
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodStripe, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	payMoney := quote.ChargedAmount
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(payMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment amount must be exact to cents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if !isStripeTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Stripe payments are not enabled"})
		return
	}
	if req.PaymentMethod != model.PaymentMethodStripe {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}
	if req.Amount > 10000 {
		c.JSON(http.StatusOK, gin.H{"message": "充值数量不能大于 10000", "data": 10})
		return
	}

	if req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付成功重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	if req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "支付取消重定向URL不在可信任域名列表中", "data": ""})
		return
	}

	id := c.GetInt("id")
	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Stripe пользователь не найден user_id=%d error=%v", id, err))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}
	quote, quoteErr := service.BuildPaymentQuote(req.Amount, model.PaymentMethodStripe, user.Group)
	if quoteErr != nil {
		common.ApiErrorMsg(c, quoteErr.Error())
		return
	}
	chargedMoney := quote.ChargedAmount
	providerChargedMoney := decimal.NewFromFloat(chargedMoney).Round(2).InexactFloat64()
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(chargedMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment amount must be exact to cents"})
		return
	}

	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))
	quotaToAdd := getTopUpQuotaToAdd(req.Amount)

	topUp := &model.TopUp{
		UserId:            id,
		Amount:            int64(req.Amount),
		RequestedAmount:   req.Amount,
		Money:             providerChargedMoney,
		TradeNo:           referenceId,
		PaymentMethod:     model.PaymentMethodStripe,
		PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodStripe),
		PaymentProvider:   model.PaymentProviderStripe,
		QuotaToAdd:        quotaToAdd,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	service.ApplyPaymentQuote(topUp, quote)
	topUp.PaymentChargedAmount = providerChargedMoney
	topUp.Money = providerChargedMoney
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建充值订单失败 user_id=%d trade_no=%s amount=%g error=%q", id, referenceId, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	payLink, err := genStripeLink(referenceId, user.StripeCustomer, user.Email, req.Amount, providerChargedMoney, req.SuccessURL, req.CancelURL)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 创建 Checkout Session 失败 user_id=%d trade_no=%s amount=%g error=%q", id, referenceId, req.Amount, err.Error()))
		// A transport error is ambiguous: Stripe may have created the session
		// before the client observed the failure. Keep the durable pending row so
		// a late webhook can still settle it.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Stripe 充值订单创建成功 user_id=%d trade_no=%s amount=%g money=%.2f", id, referenceId, req.Amount, chargedMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(payload)))
	event, err := webhook.ConstructEventWithOptions(payload, signature, setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 验签失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	callerIp := c.ClientIP()
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 验签成功 event_type=%s client_ip=%s path=%q", string(event.Type), callerIp, c.Request.RequestURI))
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		err = sessionCompleted(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionExpired:
		err = sessionExpired(ctx, event)
	case stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		err = sessionAsyncPaymentSucceeded(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		err = sessionAsyncPaymentFailed(ctx, event, callerIp)
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 忽略事件 event_type=%s client_ip=%s", string(event.Type), callerIp))
	}
	if err != nil {
		if errors.Is(err, stripePermanentWebhookError) {
			c.AbortWithStatus(http.StatusBadRequest)
		} else {
			c.AbortWithStatus(http.StatusInternalServerError)
		}
		return
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.completed 状态异常，忽略处理 trade_no=%s status=%s client_ip=%s", referenceId, status, callerIp))
		return nil
	}

	paymentStatus := event.GetObjectValue("payment_status")
	if paymentStatus != "paid" {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe Checkout 支付未完成，等待异步结果 trade_no=%s payment_status=%s client_ip=%s", referenceId, paymentStatus, callerIp))
		return nil
	}

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentSucceeded handles delayed payment methods (bank transfer, SEPA, etc.)
// that confirm payment after the checkout session completes.
func sessionAsyncPaymentSucceeded(ctx context.Context, event stripe.Event, callerIp string) error {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付成功 trade_no=%s client_ip=%s", referenceId, callerIp))

	return fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentFailed handles delayed payment failures with durable,
// database-level state transitions. Subscription orders are expired before
// falling back to top-up orders because both use Stripe client references.
func sessionAsyncPaymentFailed(ctx context.Context, event stripe.Event, callerIp string) error {
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogWarn(ctx, fmt.Sprintf("Stripe async payment failed trade_no=%s client_ip=%s", referenceId, callerIp))
	if referenceId == "" {
		return markStripeWebhookPermanent(fmt.Errorf("missing order reference"))
	}

	if err := model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe subscription order expired after async failure trade_no=%s", referenceId))
		return nil
	} else if !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		if errors.Is(err, model.ErrPaymentMethodMismatch) {
			return markStripeWebhookPermanent(err)
		}
		return err
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
	if err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe top-up marked failed trade_no=%s client_ip=%s", referenceId, callerIp))
		return nil
	}
	if errors.Is(err, model.ErrTopUpStatusInvalid) {
		// The failure was already applied or a concurrent success won the CAS.
		return nil
	}
	if errors.Is(err, model.ErrTopUpNotFound) {
		return markStripeWebhookPermanent(err)
	}
	if errors.Is(err, model.ErrPaymentMethodMismatch) {
		return markStripeWebhookPermanent(err)
	}
	return err
}

// fulfillOrder is the shared logic for crediting quota after payment is confirmed.
func fulfillOrder(ctx context.Context, event stripe.Event, referenceId string, customerId string, callerIp string) error {
	if len(referenceId) == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 完成订单时缺少订单号 client_ip=%s", callerIp))
		return markStripeWebhookPermanent(fmt.Errorf("missing order reference"))
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	payload := map[string]any{
		"customer":        customerId,
		"amount_total":    event.GetObjectValue("amount_total"),
		"currency":        strings.ToUpper(event.GetObjectValue("currency")),
		"stripe_price_id": event.GetObjectValue("metadata", "stripe_price_id"),
		"event_type":      string(event.Type),
	}
	if err := model.CompleteSubscriptionOrder(referenceId, common.GetJsonString(payload), model.PaymentProviderStripe, ""); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单处理成功 trade_no=%s event_type=%s client_ip=%s", referenceId, string(event.Type), callerIp))
		return nil
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		if errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) {
			// Expired/terminal subscription orders are intentionally final. Stripe
			// should not retry a legitimate late callback.
			return nil
		}
		logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单处理失败 trade_no=%s event_type=%s client_ip=%s error=%q", referenceId, string(event.Type), callerIp, err.Error()))
		if model.IsPermanentSubscriptionOrderError(err) {
			return markStripeWebhookPermanent(err)
		}
		return err
	}
	topUp, lookupErr := model.GetTopUpByTradeNoWithError(referenceId)
	if lookupErr != nil {
		if errors.Is(lookupErr, model.ErrTopUpNotFound) {
			logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在或支付网关不匹配 trade_no=%s", referenceId))
			return markStripeWebhookPermanent(lookupErr)
		}
		logger.LogError(ctx, fmt.Sprintf("Stripe 查询充值订单失败 trade_no=%s error=%q", referenceId, lookupErr.Error()))
		return lookupErr
	}
	if topUp == nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在或支付网关不匹配 trade_no=%s", referenceId))
		return markStripeWebhookPermanent(fmt.Errorf("topup not found or provider mismatch"))
	}
	if topUp.PaymentProvider != model.PaymentProviderStripe {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单支付网关不匹配 trade_no=%s expected=%s actual=%s", referenceId, model.PaymentProviderStripe, topUp.PaymentProvider))
		return nil
	}
	if topUp.Status != common.TopUpStatusPending {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe terminal top-up callback acknowledged trade_no=%s status=%s", referenceId, topUp.Status))
		return nil
	}
	if err := validateStripeTopUpPayment(topUp, event); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 回调金额/货币与订单快照不匹配 trade_no=%s error=%q", referenceId, err.Error()))
		if service.IsPermanentPaymentSnapshotError(err) {
			return nil
		}
		return err
	}

	err := model.Recharge(referenceId, customerId, callerIp)
	if err != nil {
		if errors.Is(err, model.ErrTopUpStatusInvalid) || errors.Is(err, model.ErrTopUpExpired) {
			return nil
		}
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值处理失败 trade_no=%s event_type=%s client_ip=%s error=%q", referenceId, string(event.Type), callerIp, err.Error()))
		if model.IsPermanentTopUpError(err, topUp) {
			return markStripeWebhookPermanent(err)
		}
		return err
	}

	total, _ := strconv.ParseFloat(event.GetObjectValue("amount_total"), 64)
	currency := strings.ToUpper(event.GetObjectValue("currency"))
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值成功 trade_no=%s amount_total=%.2f currency=%s event_type=%s client_ip=%s", referenceId, total/100, currency, string(event.Type), callerIp))
	return nil
}

func validateStripeTopUpPayment(topUp *model.TopUp, event stripe.Event) error {
	amountCents, err := decimal.NewFromString(strings.TrimSpace(event.GetObjectValue("amount_total")))
	if err != nil {
		return fmt.Errorf("%w: invalid Stripe payment amount: %v", service.ErrPaymentSnapshotValidation, err)
	}
	amountUSD := amountCents.Div(decimal.NewFromInt(100))
	currency := strings.ToUpper(strings.TrimSpace(event.GetObjectValue("currency")))
	return service.ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderStripe, currency, amountUSD.InexactFloat64())
}

func sessionExpired(ctx context.Context, event stripe.Event) error {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 状态异常，忽略处理 trade_no=%s status=%s", referenceId, status))
		return nil
	}

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, "Stripe checkout.expired 缺少订单号")
		return markStripeWebhookPermanent(fmt.Errorf("missing order reference"))
	}

	// Subscription order expiration
	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if err := model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单已过期 trade_no=%s", referenceId))
		return nil
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		if errors.Is(err, model.ErrPaymentMethodMismatch) {
			logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 订单支付网关不匹配 trade_no=%s", referenceId))
			return nil
		}
		logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单过期处理失败 trade_no=%s error=%q", referenceId, err.Error()))
		return err
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusExpired)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在，无法标记过期 trade_no=%s", referenceId))
		return markStripeWebhookPermanent(err)
	}
	if errors.Is(err, model.ErrTopUpStatusInvalid) {
		// A repeated expiry event must not retry forever after another terminal
		// callback has already settled or failed the top-up.
		return nil
	}
	if errors.Is(err, model.ErrPaymentMethodMismatch) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 充值订单支付网关不匹配 trade_no=%s", referenceId))
		return nil
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值订单过期处理失败 trade_no=%s error=%q", referenceId, err.Error()))
		return err
	}

	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已过期 trade_no=%s", referenceId))
	return nil
}

// genStripeLink generates a Stripe Checkout session URL for payment.
// It creates a new checkout session with the specified parameters and returns the payment URL.
//
// Parameters:
//   - referenceId: unique reference identifier for the transaction
//   - customerId: existing Stripe customer ID (empty string if new customer)
//   - email: customer email address for new customer creation
//   - amount: quantity of units to purchase
//   - successURL: custom URL to redirect after successful payment (empty for default)
//   - cancelURL: custom URL to redirect when payment is canceled (empty for default)
//
// Returns the checkout session URL or an error if the session creation fails.
func genStripeLink(referenceId string, customerId string, email string, amount float64, chargedMoney float64, successURL string, cancelURL string) (string, error) {
	if !isStripeAPISecretConfigured() {
		return "", fmt.Errorf("无效的Stripe API密钥")
	}

	stripe.Key = strings.TrimSpace(setting.StripeApiSecret)

	// Use custom URLs if provided, otherwise use defaults
	if successURL == "" {
		successURL = paymentReturnPath("/console/log")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/console/topup")
	}

	// Always use an inline PriceData item for top-ups. A configured Stripe
	// PriceId has an opaque amount and can diverge from the server quote.
	lineItem := newStripeTopUpLineItemExact(chargedMoney)

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID:   stripe.String(referenceId),
		SuccessURL:          stripe.String(successURL),
		CancelURL:           stripe.String(cancelURL),
		LineItems:           []*stripe.CheckoutSessionLineItemParams{lineItem},
		Mode:                stripe.String(string(stripe.CheckoutSessionModePayment)),
		AllowPromotionCodes: stripe.Bool(setting.StripePromotionCodesEnabled),
	}

	if "" == customerId {
		if "" != email {
			params.CustomerEmail = stripe.String(email)
		}

		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}

	result, err := session.New(params)
	if err != nil {
		return "", err
	}

	return result.URL, nil
}

func newStripeTopUpLineItem(amount float64, chargedMoney float64) *stripe.CheckoutSessionLineItemParams {
	lineItem := &stripe.CheckoutSessionLineItemParams{
		Price:    stripe.String(setting.StripePriceId),
		Quantity: stripe.Int64(int64(amount)),
	}
	if amount != math.Trunc(amount) {
		return newStripeTopUpLineItemExact(chargedMoney)
	}
	return lineItem
}

func newStripeTopUpLineItemExact(chargedMoney float64) *stripe.CheckoutSessionLineItemParams {
	lineItem := &stripe.CheckoutSessionLineItemParams{
		Quantity: stripe.Int64(1),
		PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
			Currency:   stripe.String("usd"),
			UnitAmount: stripe.Int64(int64(math.Round(chargedMoney * 100))),
			ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
				Name: stripe.String("Account top-up"),
			},
		},
	}
	return lineItem
}

func GetChargedAmount(count float64, user model.User) float64 {
	topUpGroupRatio := common.GetTopupGroupRatio(user.Group)
	if topUpGroupRatio <= 1 {
		topUpGroupRatio = 1
	}

	return decimal.NewFromFloat(count).Mul(decimal.NewFromFloat(topUpGroupRatio)).InexactFloat64()
}

func getStripePayMoney(amount float64, group string) float64 {
	paymentAmount := decimal.NewFromFloat(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		paymentAmount = paymentAmount.Div(decimal.NewFromFloat(common.GetQuotaPerUnit()))
	}
	topupGroupRatio := common.GetTopupGroupRatio(getPaymentTopupGroup(model.PaymentMethodStripe, group))
	if topupGroupRatio <= 1 {
		topupGroupRatio = 1
	}
	return paymentAmount.
		Mul(decimal.NewFromFloat(setting.StripeUnitPrice)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		InexactFloat64()
}

func getStripeMinTopup() float64 {
	minTopup := float64(setting.StripeMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minTopup *= common.GetQuotaPerUnit()
	}
	return minTopup
}

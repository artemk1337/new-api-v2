package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/price"
	"github.com/thanhpk/randstr"
)

type SubscriptionStripePayRequest struct {
	PlanId int `json:"plan_id"`
}

func SubscriptionRequestStripePay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionStripePayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.StripePriceId == "" {
		common.ApiErrorMsg(c, "该套餐未配置 StripePriceId")
		return
	}
	if !isStripeAPISecretConfigured() {
		common.ApiErrorMsg(c, "Stripe 未配置或密钥无效")
		return
	}
	if !isStripeWebhookConfigured() {
		common.ApiErrorMsg(c, "Stripe Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	if err := validateStripeSubscriptionPrice(plan.StripePriceId, plan.PriceAmount, plan.Currency); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	reference := fmt.Sprintf("sub-stripe-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "sub_ref_" + common.Sha1([]byte(reference))

	order := &model.SubscriptionOrder{
		UserId:            userId,
		PlanId:            plan.Id,
		Money:             plan.PriceAmount,
		TradeNo:           referenceId,
		PaymentMethod:     model.PaymentMethodStripe,
		PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodStripe),
		PaymentProvider:   model.PaymentProviderStripe,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	order.PlanSnapshot, err = model.NewSubscriptionOrderSnapshot(plan, model.PaymentProviderStripe)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payLink, err := createStripeSubscriptionCheckout(order, user, plan)
	if err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅支付链接创建失败 trade_no=%s plan_id=%d error=%q", referenceId, plan.Id, err.Error()))
		if isPermanentStripeCreateError(err) {
			if failErr := model.FailSubscriptionOrder(referenceId, model.PaymentProviderStripe); failErr != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("Stripe 订阅订单终止失败 trade_no=%s error=%q", referenceId, failErr.Error()))
			}
		}
		// Ambiguous transport errors keep the pending order: Stripe may have
		// created the Checkout Session before the error reached us.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func isPermanentStripeCreateError(err error) bool {
	var stripeErr *stripe.Error
	if !errors.As(err, &stripeErr) {
		return false
	}
	status := stripeErr.HTTPStatusCode
	return status >= http.StatusBadRequest && status < http.StatusInternalServerError &&
		status != http.StatusRequestTimeout && status != http.StatusTooManyRequests
}

type stripeSubscriptionLinkFunc func(referenceId string, customerId string, email string, priceId string) (string, error)

var stripeSubscriptionLinkCreator stripeSubscriptionLinkFunc = genStripeSubscriptionLink

var stripeSubscriptionPriceLookup = func(priceID string) (*stripe.Price, error) {
	stripe.Key = strings.TrimSpace(setting.StripeApiSecret)
	return price.Get(priceID, nil)
}

// SubscriptionPlan entitlements are granted once by our webhook. Use a
// one-time Checkout Session so Stripe does not create an unmanaged recurring
// subscription; recurring Prices are rejected before this point.
const stripeSubscriptionCheckoutMode = stripe.CheckoutSessionModePayment

func validateStripeSubscriptionPrice(priceID string, expectedAmount float64, expectedCurrency string) error {
	if strings.TrimSpace(priceID) == "" {
		return fmt.Errorf("Stripe Price не настроен")
	}
	configuredPrice, err := stripeSubscriptionPriceLookup(strings.TrimSpace(priceID))
	if err != nil {
		return fmt.Errorf("не удалось проверить Stripe Price: %w", err)
	}
	if configuredPrice == nil || configuredPrice.Recurring != nil || configuredPrice.Type == stripe.PriceTypeRecurring {
		return fmt.Errorf("Stripe Price должен быть одноразовым")
	}
	currency := strings.ToUpper(strings.TrimSpace(expectedCurrency))
	if currency == "" {
		currency = "USD"
	}
	if !model.SubscriptionProviderAmountRepresentable(expectedAmount, currency) {
		return fmt.Errorf("сумма тарифа должна быть указана с точностью до минимальной единицы валюты")
	}
	if strings.ToUpper(string(configuredPrice.Currency)) != currency {
		return fmt.Errorf("валюта Stripe Price не совпадает с тарифом")
	}
	if configuredPrice.UnitAmount != model.SubscriptionProviderMinorAmount(expectedAmount, currency) {
		return fmt.Errorf("сумма Stripe Price не совпадает с тарифом")
	}
	return nil
}

// createStripeSubscriptionCheckout persists the local pending order before
// contacting Stripe. The pending state is intentionally retained when Stripe
// returns an error because a transport failure can follow a successfully
// created Checkout Session; its webhook must still find the order.
func createStripeSubscriptionCheckout(order *model.SubscriptionOrder, user *model.User, plan *model.SubscriptionPlan) (string, error) {
	if order == nil || user == nil || plan == nil {
		return "", fmt.Errorf("invalid Stripe subscription checkout arguments")
	}
	if strings.TrimSpace(order.PlanSnapshot) == "" {
		snapshot, err := model.NewSubscriptionOrderSnapshot(plan, model.PaymentProviderStripe)
		if err != nil {
			return "", err
		}
		order.PlanSnapshot = snapshot
	}
	if err := model.CreatePendingSubscriptionOrder(order, plan.MaxPurchasePerUser); err != nil {
		return "", err
	}
	return stripeSubscriptionLinkCreator(order.TradeNo, user.StripeCustomer, user.Email, plan.StripePriceId)
}

func genStripeSubscriptionLink(referenceId string, customerId string, email string, priceId string) (string, error) {
	if !isStripeAPISecretConfigured() {
		return "", fmt.Errorf("无效的Stripe API密钥")
	}
	stripe.Key = strings.TrimSpace(setting.StripeApiSecret)

	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId),
		SuccessURL:        stripe.String(paymentReturnPath("/console/topup")),
		CancelURL:         stripe.String(paymentReturnPath("/console/topup")),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(priceId),
				Quantity: stripe.Int64(1),
			},
		},
		Mode: stripe.String(string(stripeSubscriptionCheckoutMode)),
	}
	params.AddMetadata("stripe_price_id", strings.TrimSpace(priceId))
	params.AddMetadata("subscription_reference", referenceId)

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

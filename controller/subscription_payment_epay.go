package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

type SubscriptionEpayPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
}

// EPay's Purchase implementation currently only builds a signed redirect
// URL. Keep the call injectable so the lifecycle policy can be tested without
// making a real provider request and, more importantly, so future client
// implementations cannot accidentally turn an ambiguous transport failure
// into a terminal local order state.
var errEpayPurchaseInvalidConfiguration = errors.New("epay purchase invalid configuration")

var epayPurchase = purchaseEpay

func purchaseEpay(client *epay.Client, args *epay.PurchaseArgs) (string, map[string]string, error) {
	uri, params, err := client.Purchase(args)
	if err == nil {
		return uri, params, nil
	}
	// The current library only parses the configured base URL. Preserve the
	// distinction if a future client adds network I/O: parse failures are
	// terminal configuration errors, while all other failures stay ambiguous.
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Op == "parse" {
		return uri, params, fmt.Errorf("%w: %v", errEpayPurchaseInvalidConfiguration, err)
	}
	return uri, params, err
}

func isPermanentEpayPurchaseError(err error) bool {
	if errors.Is(err, errEpayPurchaseInvalidConfiguration) {
		return true
	}
	var urlErr *url.Error
	return errors.As(err, &urlErr) && urlErr.Op == "parse"
}

func validateEpaySubscriptionPrice(amount float64) error {
	if !model.SubscriptionProviderAmountRepresentable(amount, operation_setting.QuotaDisplayTypeUSD) {
		return fmt.Errorf("сумма тарифа должна быть указана с точностью до минимальной единицы валюты")
	}
	return nil
}

func epaySubscriptionPaymentMethodMatches(order *model.SubscriptionOrder, callbackMethod string) bool {
	if order == nil {
		return false
	}
	expected := strings.TrimSpace(order.PaymentMethod)
	actual := strings.TrimSpace(callbackMethod)
	return expected != "" && actual != "" && strings.EqualFold(expected, actual)
}

func settleEpayPurchaseFailure(tradeNo string, err error) error {
	if !isPermanentEpayPurchaseError(err) {
		return nil
	}
	return model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderEpay)
}

func SubscriptionRequestEpay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}

	var req SubscriptionEpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	if !paymentMethodAllowedForUser(c, req.PaymentMethod) {
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
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	if err := validateEpaySubscriptionPrice(plan.PriceAmount); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	userId := c.GetInt("id")
	if user, userErr := model.GetUserById(userId, false); userErr != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/return")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/subscription/epay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("SUBUSR%dNO%s", userId, tradeNo)

	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}

	order := &model.SubscriptionOrder{
		UserId:            userId,
		PlanId:            plan.Id,
		Money:             plan.PriceAmount,
		TradeNo:           tradeNo,
		PaymentMethod:     req.PaymentMethod,
		PaymentMethodName: model.PaymentMethodDisplayName(req.PaymentMethod),
		PaymentProvider:   model.PaymentProviderEpay,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	order.PlanSnapshot, err = model.NewSubscriptionOrderSnapshotWithProvider(plan, model.PaymentProviderEpay, plan.PriceAmount, operation_setting.QuotaDisplayTypeUSD, req.PaymentMethod)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.CreatePendingSubscriptionOrder(order, plan.MaxPurchasePerUser); err != nil {
		if errors.Is(err, model.ErrSubscriptionPurchaseLimit) {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	uri, params, err := epayPurchase(client, &epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("SUB:%s", plan.Title),
		Money:          strconv.FormatFloat(plan.PriceAmount, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		// Only a proven local/configuration rejection may close the order. A
		// transport or provider error is ambiguous: the provider may already
		// have accepted the purchase, so keep the pending row for notify/retry
		// reconciliation.
		if expireErr := settleEpayPurchaseFailure(tradeNo, err); expireErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("EPay subscription order expiration failed trade_no=%s error=%q", tradeNo, expireErr.Error()))
		}
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func SubscriptionEpayNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if verifyInfo.TradeStatus != epay.StatusTradeSuccess {
		if isTerminalEpayTradeStatus(verifyInfo.TradeStatus) {
			LockOrder(verifyInfo.ServiceTradeNo)
			defer UnlockOrder(verifyInfo.ServiceTradeNo)
			order, lookupErr := model.GetSubscriptionOrderByTradeNoWithError(verifyInfo.ServiceTradeNo)
			if lookupErr != nil && !errors.Is(lookupErr, model.ErrSubscriptionOrderNotFound) {
				_, _ = c.Writer.Write([]byte("fail"))
				return
			}
			if order != nil && order.PaymentProvider != model.PaymentProviderEpay {
				_, _ = c.Writer.Write([]byte("success"))
				return
			}
			if order != nil && !epaySubscriptionPaymentMethodMatches(order, verifyInfo.Type) {
				// The callback is authenticated, but its payment method does not
				// match the immutable local order. Acknowledge it without closing
				// the order so a valid provider callback can still settle it.
				_, _ = c.Writer.Write([]byte("success"))
				return
			}
			if err := model.ExpireSubscriptionOrder(verifyInfo.ServiceTradeNo, model.PaymentProviderEpay); err != nil &&
				!errors.Is(err, model.ErrSubscriptionOrderNotFound) && !errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) {
				if errors.Is(err, model.ErrPaymentMethodMismatch) {
					_, _ = c.Writer.Write([]byte("success"))
					return
				}
				_, _ = c.Writer.Write([]byte("fail"))
				return
			}
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		// Intermediate/unknown statuses are acknowledgements only. Keep the
		// order pending so a later success or terminal callback can reconcile it.
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	LockOrder(verifyInfo.ServiceTradeNo)
	defer UnlockOrder(verifyInfo.ServiceTradeNo)
	order, lookupErr := model.GetSubscriptionOrderByTradeNoWithError(verifyInfo.ServiceTradeNo)
	if lookupErr != nil && !errors.Is(lookupErr, model.ErrSubscriptionOrderNotFound) {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	if order != nil && order.PaymentProvider == model.PaymentProviderEpay && !epaySubscriptionPaymentMethodMatches(order, verifyInfo.Type) {
		// A valid EPay callback for another method must not settle or rename
		// this immutable subscription order.
		_, _ = c.Writer.Write([]byte("success"))
		return
	}

	if err := model.CompleteSubscriptionOrder(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), model.PaymentProviderEpay, verifyInfo.Type); err != nil {
		if model.IsPermanentSubscriptionOrderError(err) {
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

// SubscriptionEpayReturn handles browser return after payment.
// It verifies the payload and completes the order, then redirects to console.
func SubscriptionEpayReturn(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}

	if len(params) == 0 {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}

	client := GetEpayClient()
	if client == nil {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		order, lookupErr := model.GetSubscriptionOrderByTradeNoWithError(verifyInfo.ServiceTradeNo)
		if lookupErr != nil && !errors.Is(lookupErr, model.ErrSubscriptionOrderNotFound) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		if order != nil && order.PaymentProvider == model.PaymentProviderEpay && !epaySubscriptionPaymentMethodMatches(order, verifyInfo.Type) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		if err := model.CompleteSubscriptionOrder(verifyInfo.ServiceTradeNo, common.GetJsonString(verifyInfo), model.PaymentProviderEpay, verifyInfo.Type); err != nil {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=success"))
		return
	}
	if isTerminalEpayTradeStatus(verifyInfo.TradeStatus) {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		order, lookupErr := model.GetSubscriptionOrderByTradeNoWithError(verifyInfo.ServiceTradeNo)
		if lookupErr != nil && !errors.Is(lookupErr, model.ErrSubscriptionOrderNotFound) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=pending"))
			return
		}
		if order != nil && order.PaymentProvider == model.PaymentProviderEpay && !epaySubscriptionPaymentMethodMatches(order, verifyInfo.Type) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
			return
		}
		if err := model.ExpireSubscriptionOrder(verifyInfo.ServiceTradeNo, model.PaymentProviderEpay); err != nil &&
			!errors.Is(err, model.ErrSubscriptionOrderNotFound) && !errors.Is(err, model.ErrSubscriptionOrderStatusInvalid) {
			c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=pending"))
			return
		}
		c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=fail"))
		return
	}
	c.Redirect(http.StatusFound, paymentReturnPath("/console/topup?pay=pending"))
}

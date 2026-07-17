package controller

import (
	"crypto/hmac"
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const nowPaymentsSignatureHeader = "x-nowpayments-sig"

type NOWPaymentsPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

func RequestNOWPaymentsAmount(c *gin.Context) {
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("Top-up amount cannot be less than %d", getMinTopup()))
		return
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to get user group")
		return
	}
	paymentAmount := getPayMoney(req.Amount, group)
	if paymentAmount <= 0.01 {
		common.ApiErrorMsg(c, "Top-up amount is too low")
		return
	}
	common.ApiSuccess(c, decimal.NewFromFloat(paymentAmount).Round(2).StringFixed(2))
}

func RequestNOWPaymentsPay(c *gin.Context) {
	if !isNOWPaymentsTopUpEnabled() {
		common.ApiErrorMsg(c, "NOWPayments are not enabled")
		return
	}
	var req NOWPaymentsPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PaymentMethod != model.PaymentMethodNOWPayments {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("Top-up amount cannot be less than %d", getMinTopup()))
		return
	}
	userID := c.GetInt("id")
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to get user group")
		return
	}
	paymentAmount := getPayMoney(req.Amount, group)
	if paymentAmount <= 0.01 {
		common.ApiErrorMsg(c, "Top-up amount is too low")
		return
	}
	tradeNo := fmt.Sprintf("NOW%d%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	quotaToAdd := getYooKassaQuotaToAdd(req.Amount)
	topUp := &model.TopUp{UserId: userID, Amount: req.Amount, Money: paymentAmount, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodNOWPayments, PaymentProvider: model.PaymentProviderNOWPayments, QuotaToAdd: quotaToAdd, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending}
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to create top-up user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "Failed to create order")
		return
	}
	returnURL := paymentReturnPath("/console/topup?show_history=true")
	invoice, err := service.NewNOWPaymentsClient(nil).CreateInvoice(c.Request.Context(), service.NOWPaymentsInvoiceRequest{
		PriceAmount:   decimal.NewFromFloat(paymentAmount).Round(2).StringFixed(2),
		PriceCurrency: strings.ToLower(setting.NOWPaymentsPriceCurrency),
		PayCurrency:   strings.ToLower(strings.TrimSpace(setting.NOWPaymentsPayCurrency)),
		OrderID:       tradeNo, OrderDescription: "Top up " + tradeNo,
		IPNCallbackURL: setting.NOWPaymentsIPNCallbackURL,
		SuccessURL:     returnURL, CancelURL: returnURL,
	})
	if err != nil || strings.TrimSpace(invoice.InvoiceURL) == "" {
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to create invoice trade_no=%s error=%q", tradeNo, err.Error()))
		}
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNOWPayments, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "Failed to start payment")
		return
	}
	metadata, _ := common.Marshal(map[string]string{"invoice_id": invoice.ID})
	if err := (&model.PaymentMetadata{TradeNo: tradeNo, PaymentProvider: model.PaymentProviderNOWPayments, ExternalPaymentID: invoice.ID, Metadata: string(metadata), CreateTime: time.Now().Unix(), UpdateTime: time.Now().Unix()}).Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to save payment metadata trade_no=%s invoice_id=%s error=%q", tradeNo, invoice.ID, err.Error()))
	}
	common.ApiSuccess(c, gin.H{"payment_url": invoice.InvoiceURL, "trade_no": tradeNo})
}

func NOWPaymentsWebhook(c *gin.Context) {
	if !isNOWPaymentsWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !verifyNOWPaymentsSignature(body, c.GetHeader(nowPaymentsSignatureHeader), setting.NOWPaymentsIPNSecret) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	var payload service.NOWPaymentsPayment
	if err := common.Unmarshal(body, &payload); err != nil || strings.TrimSpace(payload.PaymentID) == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if payload.PaymentStatus != "finished" && payload.PaymentStatus != "confirmed" {
		c.Status(http.StatusOK)
		return
	}
	ctx, cancel := service.NOWPaymentsRequestTimeoutContext(c.Request.Context())
	defer cancel()
	payment, err := service.NewNOWPaymentsClient(nil).GetPayment(ctx, payload.PaymentID)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to verify payment_id=%s error=%q", payload.PaymentID, err.Error()))
		c.AbortWithStatus(http.StatusBadGateway)
		return
	}
	if err := completeNOWPaymentsPayment(payment, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments payment completion failed payment_id=%s error=%q", payment.PaymentID, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.Status(http.StatusOK)
}

func verifyNOWPaymentsSignature(body []byte, signature, secret string) bool {
	if strings.TrimSpace(signature) == "" || strings.TrimSpace(secret) == "" {
		return false
	}
	var payload map[string]any
	if err := common.Unmarshal(body, &payload); err != nil {
		return false
	}
	canonical, err := common.Marshal(payload)
	if err != nil {
		return false
	}
	mac := hmac.New(sha512.New, []byte(secret))
	_, _ = mac.Write(canonical)
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(signature))), []byte(fmt.Sprintf("%x", mac.Sum(nil))))
}

func completeNOWPaymentsPayment(payment *service.NOWPaymentsPayment, callerIP string) error {
	if payment.PaymentStatus != "finished" && payment.PaymentStatus != "confirmed" {
		return fmt.Errorf("unexpected payment status %s", payment.PaymentStatus)
	}
	tradeNo := strings.TrimSpace(payment.OrderID)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderNOWPayments {
		return fmt.Errorf("topup not found or provider mismatch")
	}
	if !strings.EqualFold(payment.PriceCurrency, setting.NOWPaymentsPriceCurrency) {
		return fmt.Errorf("unexpected price currency %s", payment.PriceCurrency)
	}
	actual, err := decimal.NewFromString(payment.PriceAmount)
	if err != nil {
		return err
	}
	if !actual.Equal(decimal.NewFromFloat(topUp.Money).Round(2)) {
		return fmt.Errorf("amount mismatch")
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	return model.RechargeNOWPayments(tradeNo, callerIP)
}

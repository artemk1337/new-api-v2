package controller

import (
	"crypto/hmac"
	"crypto/sha512"
	"errors"
	"fmt"
	"io"
	"math"
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
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
}

func nowPaymentsInvoicePriceCurrency(_ *model.TopUp) string {
	return "usdt"
}

func RequestNOWPaymentsAmount(c *gin.Context) {
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("Top-up amount cannot be less than %g", getMinTopup()))
		return
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to get user group")
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodNOWPayments, group)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	paymentAmount := quote.ChargedAmount
	if paymentAmount <= 0.01 {
		common.ApiErrorMsg(c, "Top-up amount is too low")
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(paymentAmount, 2) {
		common.ApiErrorMsg(c, "Payment amount must be exact to cents")
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
		common.ApiErrorMsg(c, fmt.Sprintf("Top-up amount cannot be less than %g", getMinTopup()))
		return
	}
	userID := c.GetInt("id")
	if user, userErr := model.GetUserById(userID, false); userErr != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments user does not exist user_id=%d error=%v", userID, userErr))
		common.ApiErrorMsg(c, "User does not exist")
		return
	}
	group, err := model.GetUserGroup(userID, true)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to get user group")
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodNOWPayments, group)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	paymentAmount := quote.ChargedAmount
	if paymentAmount <= 0.01 {
		common.ApiErrorMsg(c, "Top-up amount is too low")
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(paymentAmount, 2) {
		common.ApiErrorMsg(c, "Payment amount must be exact to cents")
		return
	}
	tradeNo := fmt.Sprintf("NOW%d%s%d", userID, common.GetRandomString(6), time.Now().Unix())
	quotaToAdd := getTopUpQuotaToAdd(req.Amount)
	topUp := &model.TopUp{UserId: userID, Amount: int64(req.Amount), RequestedAmount: req.Amount, Money: paymentAmount, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodNOWPayments, PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodNOWPayments), PaymentProvider: model.PaymentProviderNOWPayments, QuotaToAdd: quotaToAdd, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending}
	service.ApplyPaymentQuote(topUp, quote)
	// NOWPayments receives a two-decimal price. Persist exactly that amount in
	// the immutable snapshot so the callback compares the provider charge, not
	// an unrounded preview value.
	providerPaymentAmount := decimal.NewFromFloat(paymentAmount).Round(2).InexactFloat64()
	topUp.PaymentChargedAmount = providerPaymentAmount
	topUp.Money = providerPaymentAmount
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to create top-up user_id=%d trade_no=%s error=%q", userID, tradeNo, err.Error()))
		common.ApiErrorMsg(c, "Failed to create order")
		return
	}
	returnURL := paymentReturnPath("/console/topup?show_history=true")
	invoice, err := service.NewNOWPaymentsClient(nil).CreateInvoice(c.Request.Context(), service.NOWPaymentsInvoiceRequest{
		PriceAmount:   decimal.NewFromFloat(providerPaymentAmount).StringFixed(2),
		PriceCurrency: nowPaymentsInvoicePriceCurrency(topUp),
		PayCurrency:   "usdt",
		OrderID:       tradeNo, OrderDescription: "Top up " + tradeNo,
		IPNCallbackURL: setting.NOWPaymentsIPNCallbackURL,
		SuccessURL:     returnURL, CancelURL: returnURL,
	})
	// A transport timeout or an incomplete response is ambiguous: NOWPayments
	// may have accepted the idempotent request even though the client did not
	// receive the invoice. Keep the local order pending so a late IPN can still
	// settle it; only an explicit, terminal provider rejection should fail an
	// order (the SDK currently exposes no reliable terminal error type).
	if err != nil || invoice == nil || strings.TrimSpace(invoice.InvoiceURL) == "" {
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to create invoice trade_no=%s error=%q", tradeNo, err.Error()))
		} else {
			logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments returned incomplete invoice trade_no=%s", tradeNo))
		}
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
	statusCode, err := completeNOWPaymentsPayment(payment, c.ClientIP())
	if err != nil {
		paymentID := ""
		if payment != nil {
			paymentID = payment.PaymentID
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments payment completion failed payment_id=%s error=%q", paymentID, err.Error()))
		c.AbortWithStatus(statusCode)
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

func completeNOWPaymentsPayment(payment *service.NOWPaymentsPayment, callerIP string) (int, error) {
	if payment == nil {
		return http.StatusBadRequest, fmt.Errorf("missing NOWPayments payment")
	}
	if payment.PaymentStatus != "finished" && payment.PaymentStatus != "confirmed" {
		return http.StatusBadRequest, fmt.Errorf("unexpected payment status %s", payment.PaymentStatus)
	}
	tradeNo := strings.TrimSpace(payment.OrderID)
	topUp, lookupErr := model.GetTopUpByTradeNoWithError(tradeNo)
	if lookupErr != nil {
		if errors.Is(lookupErr, model.ErrTopUpNotFound) {
			return http.StatusBadRequest, fmt.Errorf("topup not found or provider mismatch")
		}
		return http.StatusInternalServerError, fmt.Errorf("lookup topup: %w", lookupErr)
	}
	if topUp == nil {
		return http.StatusBadRequest, fmt.Errorf("topup not found or provider mismatch")
	}
	if topUp.PaymentProvider != model.PaymentProviderNOWPayments {
		return http.StatusOK, nil
	}
	if topUp.Status != common.TopUpStatusPending {
		// The payment has already reached a terminal local state (including an
		// expired checkout). Acknowledge a genuine provider retry; it can no
		// longer result in a credit.
		return http.StatusOK, nil
	}
	actual, err := decimal.NewFromString(payment.PriceAmount)
	if err != nil {
		return http.StatusBadRequest, err
	}
	if err := service.ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderNOWPayments, payment.PriceCurrency, actual.InexactFloat64()); err != nil {
		if service.IsPermanentPaymentSnapshotError(err) {
			return http.StatusOK, nil
		}
		return http.StatusInternalServerError, err
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if err := model.RechargeNOWPayments(tradeNo, callerIP); err != nil {
		if errors.Is(err, model.ErrTopUpStatusInvalid) || errors.Is(err, model.ErrTopUpExpired) {
			return http.StatusOK, nil
		}
		if model.IsPermanentTopUpError(err, topUp) {
			return http.StatusBadRequest, err
		}
		return http.StatusInternalServerError, err
	}
	return http.StatusOK, nil
}

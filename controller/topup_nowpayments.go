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

type NOWPaymentsSyncRequest struct {
	TradeNo string `json:"trade_no"`
}

func RequestNOWPaymentsAmount(c *gin.Context) {
	if !paymentMethodAllowedForUser(c, model.PaymentMethodNOWPayments) {
		return
	}
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to get user group")
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodNOWPayments, group, c.GetInt("id"))
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
	if !paymentMethodAllowedForUser(c, model.PaymentMethodNOWPayments) {
		return
	}
	if !isNOWPaymentsTopUpEnabled() {
		common.ApiErrorMsg(c, "NOWPayments are not enabled")
		return
	}
	var req NOWPaymentsPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PaymentMethod != model.PaymentMethodNOWPayments {
		common.ApiErrorMsg(c, "Invalid parameters")
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
	quote, err := service.BuildPaymentQuote(req.Amount, model.PaymentMethodNOWPayments, group, userID)
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
	if isNOWPaymentsTerminalFailure(payload.PaymentStatus) {
		tradeNo := strings.TrimSpace(payload.OrderID)
		if tradeNo == "" {
			if metadata := model.GetPaymentMetadataByExternalPaymentID(model.PaymentProviderNOWPayments, payload.PaymentID); metadata != nil {
				tradeNo = metadata.TradeNo
			}
		}
		if err := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNOWPayments, common.TopUpStatusExpired); err != nil && !errors.Is(err, model.ErrTopUpStatusInvalid) {
			if errors.Is(err, model.ErrTopUpNotFound) || tradeNo == "" {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("NOWPayments terminal webhook has no local order trade_no=%s payment_id=%s", tradeNo, payload.PaymentID))
				c.Status(http.StatusOK)
				return
			}
			logger.LogError(c.Request.Context(), fmt.Sprintf("NOWPayments failed to expire payment order trade_no=%s payment_id=%s error=%q", tradeNo, payload.PaymentID, err.Error()))
			c.AbortWithStatus(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
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

func SyncNOWPaymentsTopUp(c *gin.Context) {
	var req NOWPaymentsSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TradeNo) == "" {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	tradeNo := strings.TrimSpace(req.TradeNo)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderNOWPayments {
		common.ApiErrorMsg(c, "Order does not exist")
		return
	}
	if topUp.UserId != c.GetInt("id") && c.GetInt("role") < common.RoleAdminUser {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		common.ApiSuccess(c, gin.H{"status": topUp.Status})
		return
	}
	metadata := model.GetPaymentMetadataByTradeNo(tradeNo)
	if metadata == nil || strings.TrimSpace(metadata.ExternalPaymentID) == "" {
		common.ApiErrorMsg(c, "Payment information does not exist")
		return
	}
	ctx, cancel := service.NOWPaymentsRequestTimeoutContext(c.Request.Context())
	defer cancel()
	invoice, err := service.NewNOWPaymentsClient(nil).GetInvoice(ctx, metadata.ExternalPaymentID)
	if err != nil {
		common.ApiErrorMsg(c, "Failed to synchronize payment status")
		return
	}
	invoiceStatus := invoice.PaymentStatus
	if invoiceStatus == "" {
		invoiceStatus = invoice.Status
	}
	if isNOWPaymentsTerminalFailure(invoiceStatus) {
		if err := model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderNOWPayments, common.TopUpStatusExpired); err != nil {
			common.ApiErrorMsg(c, "Failed to update payment status")
			return
		}
		common.ApiSuccess(c, gin.H{"status": common.TopUpStatusExpired})
		return
	}
	if invoiceStatus != "finished" && invoiceStatus != "confirmed" {
		common.ApiSuccess(c, gin.H{"status": common.TopUpStatusPending})
		return
	}
	payment := &service.NOWPaymentsPayment{PaymentID: invoice.PaymentID, PaymentStatus: invoiceStatus, OrderID: invoice.OrderID, PriceAmount: string(invoice.PriceAmount), PriceCurrency: invoice.PriceCurrency}
	if invoice.PaymentID != "" {
		payment, err = service.NewNOWPaymentsClient(nil).GetPayment(ctx, invoice.PaymentID)
		if err != nil {
			common.ApiErrorMsg(c, "Failed to synchronize payment status")
			return
		}
	}
	if _, err := completeNOWPaymentsPayment(payment, c.ClientIP()); err != nil {
		common.ApiErrorMsg(c, "Failed to synchronize payment status")
		return
	}
	common.ApiSuccess(c, gin.H{"status": common.TopUpStatusSuccess})
}

func isNOWPaymentsTerminalFailure(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "expired", "failed", "refunded", "canceled", "cancelled":
		return true
	default:
		return false
	}
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

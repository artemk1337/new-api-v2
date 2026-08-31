package controller

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
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
	"gorm.io/gorm"
)

type YooKassaPayRequest struct {
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
}

type yooKassaWebhookPayload struct {
	Type   string `json:"type"`
	Event  string `json:"event"`
	Object struct {
		ID string `json:"id"`
	} `json:"object"`
}

func formatYooKassaAmount(amount float64) string {
	return decimal.NewFromFloat(amount).Round(2).StringFixed(2)
}

func getTopUpQuotaToAdd(amount float64) int {
	quota := decimal.NewFromFloat(amount)
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		quota = quota.Mul(decimal.NewFromFloat(common.GetQuotaPerUnit()))
	}
	cashbackPercent := decimal.NewFromFloat(operation_setting.GetPaymentSetting().AmountCashback.CashbackPercentForAmount(amount))
	return int(quota.Mul(decimal.NewFromInt(100).Add(cashbackPercent)).Div(decimal.NewFromInt(100)).IntPart())
}

func getYooKassaReturnURLForConfig(config setting.YooKassaConfig, tradeNo string) string {
	rawURL := paymentReturnPath("/console/topup?show_history=true")
	if strings.TrimSpace(config.ReturnURL) != "" {
		rawURL = config.ReturnURL
	}
	if tradeNo == "" {
		return rawURL
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsedURL.Query()
	query.Set("show_history", "true")
	query.Set("payment_provider", model.PaymentProviderYooKassa)
	query.Set("trade_no", tradeNo)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String()
}

func isYooKassaPaymentMethodEnabled(paymentMethod string) bool {
	return isYooKassaPaymentMethodEnabledForConfig(setting.GetYooKassaConfig(), paymentMethod)
}

func isCurrentYooKassaSBPEnabled() bool {
	payMethods, err := topUpPayMethods()
	return err == nil && paymentMethodsContainType(payMethods, model.PaymentMethodYooKassaSBP)
}

func isYooKassaSBPAvailable() bool {
	config := setting.GetYooKassaConfig()
	payMethods, err := topUpPayMethods()
	return err == nil && isYooKassaSBPAvailableForMethods(config, payMethods)
}

func isYooKassaSBPAvailableForMethods(config setting.YooKassaConfig, payMethods []map[string]string) bool {
	return isYooKassaTopUpEnabledForConfig(config) &&
		isYooKassaPaymentMethodEnabledForConfig(config, model.PaymentMethodYooKassaSBP) &&
		paymentMethodsContainType(payMethods, model.PaymentMethodYooKassaSBP)
}

func isYooKassaPaymentMethodEnabledForConfig(config setting.YooKassaConfig, paymentMethod string) bool {
	method := strings.TrimSpace(paymentMethod)
	if method == "" {
		return false
	}
	if method == model.PaymentMethodYooKassaSBP {
		method = "sbp"
	}
	if !strings.EqualFold(method, "sbp") {
		return false
	}
	for _, configured := range strings.Split(config.PaymentMethods, ",") {
		if strings.EqualFold(strings.TrimSpace(configured), method) {
			return true
		}
	}
	return false
}

func RequestYooKassaAmount(c *gin.Context) {
	if !paymentMethodAllowedForUser(c, model.PaymentMethodYooKassaSBP) {
		return
	}
	payMethods, payMethodsErr := topUpPayMethods()
	if payMethodsErr != nil {
		common.ApiError(c, payMethodsErr)
		return
	}
	if !isYooKassaSBPAvailableForMethods(setting.GetYooKassaConfig(), payMethods) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment method does not exist"})
		return
	}
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Invalid parameters"})
		return
	}
	id := c.GetInt("id")
	if user, userErr := model.GetUserById(id, false); userErr != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("YooKassa user does not exist user_id=%d error=%v", id, userErr))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "User does not exist"})
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to get user group"})
		return
	}
	quote, err := service.BuildPaymentQuoteWithPayMethods(req.Amount, model.PaymentMethodYooKassaSBP, group, payMethods)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	payMoney := quote.ChargedAmount
	providerPayMoney := decimal.NewFromFloat(payMoney).Round(2).InexactFloat64()
	if providerPayMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Top-up amount is too low"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(providerPayMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment amount must be exact to cents"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": formatYooKassaAmount(providerPayMoney)})
}

func RequestYooKassaPay(c *gin.Context) {
	if !paymentMethodAllowedForUser(c, model.PaymentMethodYooKassaSBP) {
		return
	}
	yooKassaConfig := setting.GetYooKassaConfig()
	if !isYooKassaTopUpEnabledForConfig(yooKassaConfig) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "YooKassa payments are not enabled"})
		return
	}

	var req YooKassaPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Invalid parameters"})
		return
	}
	payMethods, payMethodsErr := topUpPayMethods()
	if payMethodsErr != nil {
		common.ApiError(c, payMethodsErr)
		return
	}
	if !isYooKassaPaymentMethodEnabledForConfig(yooKassaConfig, req.PaymentMethod) || !paymentMethodsContainType(payMethods, model.PaymentMethodYooKassaSBP) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment method does not exist"})
		return
	}

	id := c.GetInt("id")
	if user, userErr := model.GetUserById(id, false); userErr != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("YooKassa user does not exist user_id=%d error=%v", id, userErr))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "User does not exist"})
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to get user group"})
		return
	}
	quote, err := service.BuildPaymentQuoteWithPayMethods(req.Amount, model.PaymentMethodYooKassaSBP, group, payMethods)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	payMoney := quote.ChargedAmount
	providerPayMoney := decimal.NewFromFloat(payMoney).Round(2).InexactFloat64()
	if providerPayMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Top-up amount is too low"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(providerPayMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Payment amount must be exact to cents"})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", id, common.GetRandomString(6), time.Now().Unix())
	quotaToAdd := getTopUpQuotaToAdd(req.Amount)
	topUp := &model.TopUp{
		UserId:            id,
		Amount:            int64(req.Amount),
		RequestedAmount:   req.Amount,
		Money:             providerPayMoney,
		TradeNo:           tradeNo,
		PaymentMethod:     model.PaymentMethodYooKassaSBP,
		PaymentMethodName: model.PaymentMethodDisplayName(model.PaymentMethodYooKassaSBP),
		PaymentProvider:   model.PaymentProviderYooKassa,
		QuotaToAdd:        quotaToAdd,
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	service.ApplyPaymentQuote(topUp, quote)
	topUp.PaymentChargedAmount = providerPayMoney
	topUp.Money = providerPayMoney
	// Persist the same server-side settlement quota in webhook metadata. This
	// metadata is only a legacy fallback; the immutable local snapshot remains
	// authoritative for the normal callback path.
	quotaToAdd = topUp.QuotaToAdd
	if err := topUp.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to create top-up order user_id=%d trade_no=%s amount=%g error=%q", id, tradeNo, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to create order"})
		return
	}

	ctx, cancel := service.YooKassaRequestTimeoutContext(c.Request.Context())
	defer cancel()
	request := service.NewYooKassaPaymentRequest(tradeNo, id, topUp.Id, formatYooKassaAmount(providerPayMoney), getYooKassaReturnURLForConfig(yooKassaConfig, tradeNo), "sbp")
	payment, err := service.NewYooKassaClientWithConfig(nil, yooKassaConfig).CreatePayment(ctx, tradeNo, request)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to create payment user_id=%d trade_no=%s amount=%g error=%q", id, tradeNo, req.Amount, err.Error()))
		// The provider may have accepted the idempotent request before the
		// transport failed. Keep the durable order pending so a webhook or
		// manual synchronization can still settle it.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to start payment"})
		return
	}
	if payment == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa returned empty payment response user_id=%d trade_no=%s", id, tradeNo))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to start payment"})
		return
	}

	metadataBytes, _ := common.Marshal(map[string]string{
		"trade_no":     tradeNo,
		"user_id":      fmt.Sprintf("%d", id),
		"topup_id":     fmt.Sprintf("%d", topUp.Id),
		"quota_to_add": fmt.Sprintf("%d", quotaToAdd),
	})
	paymentMetadata := &model.PaymentMetadata{
		TradeNo:           tradeNo,
		PaymentProvider:   model.PaymentProviderYooKassa,
		ExternalPaymentID: payment.ID,
		Metadata:          string(metadataBytes),
		CreateTime:        time.Now().Unix(),
		UpdateTime:        time.Now().Unix(),
	}
	if err := paymentMetadata.Insert(); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to save payment metadata trade_no=%s payment_id=%s error=%q", tradeNo, payment.ID, err.Error()))
	}

	confirmationURL := strings.TrimSpace(payment.Confirmation.ConfirmationURL)
	if confirmationURL == "" {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa response is missing confirmation_url user_id=%d trade_no=%s payment_id=%s", id, tradeNo, payment.ID))
		// Missing confirmation data is an incomplete provider response, not a
		// proven terminal rejection. Preserve pending for webhook/reconciliation.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Failed to start payment"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("YooKassa top-up order created successfully user_id=%d trade_no=%s payment_id=%s amount=%g money=%.2f", id, tradeNo, payment.ID, req.Amount, payMoney))
	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"confirmation_url": confirmationURL,
			"payment_id":       payment.ID,
			"trade_no":         tradeNo,
		},
	})
}

type YooKassaSyncRequest struct {
	TradeNo string `json:"trade_no"`
}

func SyncYooKassaTopUp(c *gin.Context) {
	yooKassaConfig := setting.GetYooKassaConfig()
	var req YooKassaSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TradeNo) == "" {
		common.ApiErrorMsg(c, "Invalid parameters")
		return
	}
	tradeNo := strings.TrimSpace(req.TradeNo)
	topUp, lookupErr := model.GetTopUpByTradeNoWithError(tradeNo)
	if lookupErr != nil {
		if errors.Is(lookupErr, model.ErrTopUpNotFound) {
			common.ApiErrorMsg(c, "Order does not exist")
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to lookup top-up trade_no=%s error=%q", tradeNo, lookupErr.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderYooKassa {
		common.ApiErrorMsg(c, "Order does not exist")
		return
	}
	isAdmin := c.GetInt("role") >= common.RoleAdminUser
	if topUp.UserId != c.GetInt("id") && !isAdmin {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if topUp.Status == common.TopUpStatusExpired && !isAdmin {
		common.ApiErrorMsg(c, "Order has expired")
		return
	}
	metadata, metadataErr := model.GetPaymentMetadataByTradeNoWithError(tradeNo)
	if metadataErr != nil {
		if errors.Is(metadataErr, gorm.ErrRecordNotFound) {
			common.ApiErrorMsg(c, "Payment information does not exist")
			return
		}
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to lookup payment metadata trade_no=%s error=%q", tradeNo, metadataErr.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if metadata == nil || strings.TrimSpace(metadata.ExternalPaymentID) == "" {
		common.ApiErrorMsg(c, "Payment information does not exist")
		return
	}

	ctx, cancel := service.YooKassaRequestTimeoutContext(c.Request.Context())
	defer cancel()
	if _, err := completeYooKassaPaymentForConfig(ctx, yooKassaConfig, metadata.ExternalPaymentID, tradeNo, c.ClientIP(), false, isAdmin); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa failed to synchronize top-up trade_no=%s payment_id=%s user_id=%d error=%q", tradeNo, metadata.ExternalPaymentID, c.GetInt("id"), err.Error()))
		common.ApiErrorMsg(c, "Failed to synchronize payment status")
		return
	}
	common.ApiSuccess(c, nil)
}

func YooKassaNotify(c *gin.Context) {
	yooKassaConfig := setting.GetYooKassaConfig()
	if !isYooKassaWebhookConfiguredForConfig(yooKassaConfig) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("YooKassa webhook rejected reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	var payload yooKassaWebhookPayload
	if err := common.DecodeJson(c.Request.Body, &payload); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("YooKassa webhook invalid parameters path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if payload.Event != "payment.succeeded" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("YooKassa webhook ignored event notification_type=%s event=%s payment_id=%s client_ip=%s", payload.Type, payload.Event, payload.Object.ID, c.ClientIP()))
		c.Status(http.StatusOK)
		return
	}

	ctx, cancel := service.YooKassaRequestTimeoutContext(c.Request.Context())
	defer cancel()
	statusCode, err := completeYooKassaPaymentForConfig(ctx, yooKassaConfig, payload.Object.ID, "", c.ClientIP(), true, false)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("YooKassa top-up failed payment_id=%s client_ip=%s error=%q", payload.Object.ID, c.ClientIP(), err.Error()))
		c.AbortWithStatus(statusCode)
		return
	}
	c.Status(http.StatusOK)
}

func completeYooKassaPayment(ctx context.Context, paymentID string, expectedTradeNo string, callerIP string, acknowledgePermanent bool) (int, error) {
	return completeYooKassaPaymentForConfig(ctx, setting.GetYooKassaConfig(), paymentID, expectedTradeNo, callerIP, acknowledgePermanent, false)
}

func completeYooKassaPaymentForConfig(ctx context.Context, config setting.YooKassaConfig, paymentID string, expectedTradeNo string, callerIP string, acknowledgePermanent bool, allowExpiredReconciliation bool) (int, error) {
	payment, err := service.NewYooKassaClientWithConfig(nil, config).GetPayment(ctx, paymentID)
	if err != nil {
		return http.StatusBadGateway, err
	}
	tradeNo := payment.Metadata["trade_no"]
	if tradeNo == "" {
		metadata, metadataErr := model.GetPaymentMetadataByExternalPaymentIDWithError(model.PaymentProviderYooKassa, payment.ID)
		if metadataErr != nil {
			if !errors.Is(metadataErr, gorm.ErrRecordNotFound) {
				return http.StatusInternalServerError, fmt.Errorf("lookup payment metadata: %w", metadataErr)
			}
		} else if metadata != nil {
			tradeNo = metadata.TradeNo
		}
	}
	if tradeNo == "" {
		return http.StatusBadRequest, fmt.Errorf("missing trade_no for payment %s", payment.ID)
	}
	if expectedTradeNo != "" && tradeNo != expectedTradeNo {
		return http.StatusBadRequest, fmt.Errorf("trade_no mismatch")
	}
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
	if topUp.PaymentProvider != model.PaymentProviderYooKassa {
		if acknowledgePermanent {
			return http.StatusOK, nil
		}
		return http.StatusBadRequest, fmt.Errorf("topup not found or provider mismatch")
	}
	allowExpiredReconciliation = allowExpiredReconciliation && (topUp.Status == common.TopUpStatusPending || topUp.Status == common.TopUpStatusExpired)
	if topUp.Status != common.TopUpStatusPending && !allowExpiredReconciliation {
		// A verified repeat callback for an already terminal order must not make
		// the provider retry forever. RechargeYooKassa still performs its own
		// atomic status check before any credit.
		if acknowledgePermanent || topUp.Status == common.TopUpStatusSuccess {
			return http.StatusOK, nil
		}
		return http.StatusBadRequest, fmt.Errorf("topup is not pending")
	}
	if err := validateYooKassaPayment(payment, topUp); err != nil {
		if errors.Is(err, service.ErrPaymentSnapshotValidation) {
			if acknowledgePermanent {
				return http.StatusOK, nil
			}
			return http.StatusBadRequest, err
		}
		return http.StatusBadRequest, err
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	var rechargeErr error
	if allowExpiredReconciliation {
		rechargeErr = model.ReconcileYooKassaTopUp(tradeNo, callerIP)
	} else {
		rechargeErr = model.RechargeYooKassa(tradeNo, callerIP)
	}
	if rechargeErr != nil {
		if errors.Is(rechargeErr, model.ErrTopUpStatusInvalid) || errors.Is(rechargeErr, model.ErrTopUpExpired) {
			return http.StatusOK, nil
		}
		if model.IsPermanentTopUpError(rechargeErr, topUp) {
			return http.StatusBadRequest, rechargeErr
		}
		return http.StatusInternalServerError, rechargeErr
	}
	return http.StatusOK, nil
}

func validateYooKassaPayment(payment *service.YooKassaPayment, topUp *model.TopUp) error {
	if payment.Status != "succeeded" {
		return fmt.Errorf("unexpected status %s", payment.Status)
	}
	if !payment.Paid {
		return fmt.Errorf("payment is not paid")
	}
	if payment.Amount.Currency != service.YooKassaCurrencyRUB {
		return fmt.Errorf("%w: unexpected currency %s", service.ErrPaymentSnapshotValidation, payment.Amount.Currency)
	}
	expectedAmount := decimal.NewFromFloat(topUp.Money).Round(2)
	actualAmount, err := decimal.NewFromString(payment.Amount.Value)
	if err != nil {
		return err
	}
	if !actualAmount.Equal(expectedAmount) {
		return fmt.Errorf("%w: amount mismatch expected %s actual %s", service.ErrPaymentSnapshotValidation, expectedAmount.StringFixed(2), actualAmount.StringFixed(2))
	}
	if metadataTradeNo := payment.Metadata["trade_no"]; metadataTradeNo != "" && metadataTradeNo != topUp.TradeNo {
		return fmt.Errorf("%w: trade_no mismatch", service.ErrPaymentSnapshotValidation)
	}
	return nil
}

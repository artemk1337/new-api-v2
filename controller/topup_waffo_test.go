package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/waffo-com/waffo-go/config"
	"github.com/waffo-com/waffo-go/core"
)

func TestWaffoIntermediateStatusesKeepTopUpPendingUntilSuccess(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-status-sequence"
	userID := 951
	require.NoError(t, model.DB.Create(&model.User{
		Id:       userID,
		Username: "waffo_status_user",
		Status:   common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               userID,
		Amount:               1,
		Money:                1,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodWaffo,
		PaymentProvider:      model.PaymentProviderWaffo,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    1,
		PaymentChargedAmount: 1,
		QuotaToAdd:           100,
		Status:               common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	intermediateStatuses := []string{
		core.OrderStatusPayInProgress,
		core.OrderStatusAuthorizationRequired,
		core.OrderStatusAuthedWaitingCapture,
	}
	for _, status := range intermediateStatuses {
		t.Run(status, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
			handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
				MerchantOrderID: tradeNo,
				OrderStatus:     status,
			})

			assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
		})
	}

	require.True(t, shouldMarkWaffoTopUpFailed(core.OrderStatusOrderClose))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo,
		OrderStatus:     core.OrderStatusPaySuccess,
		OrderCurrency:   "USD",
		OrderAmount:     "1.00",
	})
	assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(tradeNo).Status)

	var user model.User
	require.NoError(t, model.DB.First(&user, userID).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestWaffoSnapshotDeletedUserAcknowledgesPaidCallback(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-deleted-user-snapshot"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               9950,
		RequestedAmount:      10,
		Money:                10,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodWaffo,
		PaymentProvider:      model.PaymentProviderWaffo,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    10,
		PaymentChargedAmount: 10,
		PaymentCoefficient:   1,
		QuotaToAdd:           1000,
		Status:               common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	wantBody, _ := webhookHandler.BuildSuccessResponse()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo,
		OrderStatus:     core.OrderStatusPaySuccess,
		OrderCurrency:   "USD",
		OrderAmount:     "10.00",
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, wantBody, recorder.Body.String())
	require.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestWaffoTerminalTopUpAcknowledgesBeforeAmountValidation(t *testing.T) {
	for _, status := range []string{common.TopUpStatusFailed, common.TopUpStatusExpired} {
		t.Run(status, func(t *testing.T) {
			setupPaymentLifecycleDB(t)
			tradeNo := "waffo-terminal-" + status
			require.NoError(t, model.DB.Create(&model.TopUp{
				UserId: 9951, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
				PaymentMethod: model.PaymentMethodWaffo, PaymentProvider: model.PaymentProviderWaffo,
				PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
				PaymentCoefficient: 1, QuotaToAdd: 1000, Status: status,
			}).Error)

			webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
			wantBody, _ := webhookHandler.BuildSuccessResponse()
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
			handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
				MerchantOrderID: tradeNo, OrderStatus: core.OrderStatusPaySuccess,
				OrderCurrency: "USD", OrderAmount: "not-a-number",
			})

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, wantBody, recorder.Body.String())
			assert.Equal(t, status, model.GetTopUpByTradeNo(tradeNo).Status)
		})
	}
}

func TestWaffoTerminalCallbackAcknowledgesProviderMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-terminal-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 9954, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "stripe", PaymentProvider: model.PaymentProviderStripe,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, Status: common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	wantBody, _ := webhookHandler.BuildSuccessResponse()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo,
		OrderStatus:     core.OrderStatusOrderClose,
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, wantBody, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestWaffoSuccessCallbackAcknowledgesProviderMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-success-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 9955, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "stripe", PaymentProvider: model.PaymentProviderStripe,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, Status: common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	wantBody, _ := webhookHandler.BuildSuccessResponse()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo,
		OrderStatus:     core.OrderStatusPaySuccess,
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, wantBody, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestWaffoPendingTopUpAcknowledgesAmountMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-pending-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 9952, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: model.PaymentMethodWaffo, PaymentProvider: model.PaymentProviderWaffo,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, Status: common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo, OrderStatus: core.OrderStatusPaySuccess,
		OrderCurrency: "USD", OrderAmount: "9.00",
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	wantSuccess, _ := webhookHandler.BuildSuccessResponse()
	assert.Equal(t, wantSuccess, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestWaffoPendingTopUpRejectsMalformedAmount(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "waffo-pending-malformed-amount"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 9953, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: model.PaymentMethodWaffo, PaymentProvider: model.PaymentProviderWaffo,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, Status: common.TopUpStatusPending,
	}).Error)

	webhookHandler := core.NewWebhookHandler(&config.WaffoConfig{})
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/waffo/webhook", nil)
	handleWaffoPayment(ctx, webhookHandler, &core.PaymentNotificationResult{
		MerchantOrderID: tradeNo, OrderStatus: core.OrderStatusPaySuccess,
		OrderCurrency: "USD", OrderAmount: "not-a-number",
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	wantSuccess, _ := webhookHandler.BuildSuccessResponse()
	assert.NotEqual(t, wantSuccess, recorder.Body.String())
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

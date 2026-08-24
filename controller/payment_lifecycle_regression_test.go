package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
	"gorm.io/gorm"
)

func setupPaymentLifecycleDB(t *testing.T) {
	t.Helper()
	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.SubscriptionOrder{}, &model.Log{}))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.RedisEnabled = originalRedisEnabled
	})
}

func stripeAsyncFailureEvent(tradeNo string) stripe.Event {
	return stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"client_reference_id": tradeNo,
	}}}
}

func stripeExpiredEvent(tradeNo string) stripe.Event {
	return stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"client_reference_id": tradeNo,
		"status":              "expired",
	}}}
}

func TestStripeAsyncPaymentFailedExpiresSubscriptionOrderBeforeTopUpFallback(t *testing.T) {
	setupPaymentLifecycleDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.SubscriptionOrder{}))
	tradeNo := "sub-stripe-async-failed"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 501, PlanId: 601, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, sessionAsyncPaymentFailed(context.Background(), stripeAsyncFailureEvent(tradeNo), "127.0.0.1"))
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusExpired, order.Status)
}

func TestStripeAsyncPaymentFailedCannotOverwriteSettledTopUp(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-async-success-wins"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 502, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusSuccess,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, sessionAsyncPaymentFailed(context.Background(), stripeAsyncFailureEvent(tradeNo), "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestStripeAsyncPaymentFailedMarksPendingTopUpFailed(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-async-pending"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 503, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, sessionAsyncPaymentFailed(context.Background(), stripeAsyncFailureEvent(tradeNo), "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestStripeSessionExpiredAcknowledgesSettledTopUp(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-expired-after-success"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 504, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusSuccess,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, sessionExpired(context.Background(), stripeExpiredEvent(tradeNo)))
	assert.Equal(t, common.TopUpStatusSuccess, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestStripeSessionExpiredAcknowledgesProviderMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-expired-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 504, PlanId: 604, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, sessionExpired(context.Background(), stripeExpiredEvent(tradeNo)))
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestStripeFulfillAcknowledgesExpiredTopUp(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-expired-late-success"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 505, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderStripe, Status: common.TopUpStatusExpired,
		CreateTime: time.Now().Unix(),
	}).Error)
	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"amount_total": "1000", "currency": "usd", "metadata": map[string]interface{}{},
	}}}
	require.NoError(t, fulfillOrder(context.Background(), event, tradeNo, "", "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusExpired, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestStripeFulfillAcknowledgesImmutableSnapshotMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "stripe-snapshot-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               505,
		RequestedAmount:      10,
		Money:                10,
		TradeNo:              tradeNo,
		PaymentMethod:        "stripe",
		PaymentProvider:      model.PaymentProviderStripe,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    10,
		PaymentChargedAmount: 10,
		PaymentCoefficient:   1,
		QuotaToAdd:           1000,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)
	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"amount_total": "900",
		"currency":     "usd",
		"metadata":     map[string]interface{}{},
	}}}

	require.NoError(t, fulfillOrder(context.Background(), event, tradeNo, "", "127.0.0.1"))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestCreemCheckoutAcknowledgesExpiredSubscriptionOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "creem-expired-late-success"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 506, PlanId: 606, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusExpired,
		CreateTime: time.Now().Unix(),
	}).Error)
	event := &CreemWebhookEvent{}
	event.Object.RequestId = tradeNo
	event.Object.Order.Id = "creem-order"
	event.Object.Order.Status = "paid"
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	handleCheckoutCompleted(ctx, event)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, common.TopUpStatusExpired, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
}

func TestEpaySubscriptionPurchaseErrorPolicy(t *testing.T) {
	parseErr := &url.Error{Op: "parse", URL: "%gh", Err: errors.New("invalid URL escape")}
	require.True(t, isPermanentEpayPurchaseError(parseErr))
	require.True(t, isPermanentEpayPurchaseError(errEpayPurchaseInvalidConfiguration))
	require.False(t, isPermanentEpayPurchaseError(&url.Error{Op: "Get", URL: "https://epay.example.test", Err: errors.New("timeout")}))
	require.False(t, isPermanentEpayPurchaseError(errors.New("provider transport timeout")))
}

func TestEpaySubscriptionAmbiguousPurchaseKeepsPendingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "epay-subscription-ambiguous"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 505, PlanId: 606, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderEpay, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, settleEpayPurchaseFailure(tradeNo, errors.New("transport timeout")))
	assert.Equal(t, common.TopUpStatusPending, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
	require.NoError(t, settleEpayPurchaseFailure(tradeNo, errEpayPurchaseInvalidConfiguration))
	assert.Equal(t, common.TopUpStatusExpired, model.GetSubscriptionOrderByTradeNo(tradeNo).Status)
}

func TestValidateEpaySubscriptionPriceRejectsFractionalMinorUnit(t *testing.T) {
	require.Error(t, validateEpaySubscriptionPrice(10.005))
	require.NoError(t, validateEpaySubscriptionPrice(10.01))
}

func TestCreemTopUpCreateFailureOnlyClosesPermanentErrors(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "creem-topup-create-failure-policy"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 506, Money: 10, TradeNo: tradeNo,
		PaymentProvider: model.PaymentProviderCreem, Status: common.TopUpStatusPending,
		CreateTime: time.Now().Unix(),
	}).Error)

	require.NoError(t, settleCreemTopUpCreateFailure(tradeNo, errors.New("发送HTTP请求失败: timeout")))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
	require.NoError(t, settleCreemTopUpCreateFailure(tradeNo, errors.New("Creem API http status 400")))
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestCreemWebhookHardDeletedUserAcknowledgesSnapshotBackedPayment(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalConfig := setting.GetCreemConfig()
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey:        "creem-test-api-key",
		TestMode:      true,
		WebhookSecret: "creem-test-webhook-secret",
	})
	t.Cleanup(func() { setting.PublishCreemConfig(originalConfig) })

	tradeNo := "creem-hard-deleted-user"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               507,
		Amount:               5_000_000,
		RequestedAmount:      10,
		Money:                10,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodCreem,
		PaymentProvider:      model.PaymentProviderCreem,
		CreemProductID:       "prod_eur",
		PaymentCurrency:      "EUR",
		PaymentRateToUSD:     1.25,
		PaymentCoefficient:   1,
		PaymentBaseAmount:    8,
		PaymentChargedAmount: 10,
		QuotaToAdd:           5_000_000,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)

	body := fmt.Sprintf(`{"eventType":"checkout.completed","object":{"request_id":%q,"order":{"id":"creem-order-507","status":"paid","type":"onetime","amount_paid":1000,"currency":"EUR","product":"prod_eur"},"product":{"id":"prod_eur"},"customer":{"email":"deleted@example.com","name":"Deleted User"}}}`, tradeNo)
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(body))
	req.Header.Set(CreemSignatureHeader, generateCreemSignature(body, setting.GetCreemConfig().WebhookSecret))
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/creem/webhook", CreemWebhook)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestCreemWebhookAcknowledgesMismatchedTopUpProvider(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalConfig := setting.GetCreemConfig()
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey:        "creem-test-api-key",
		TestMode:      true,
		WebhookSecret: "creem-test-webhook-secret",
	})
	t.Cleanup(func() { setting.PublishCreemConfig(originalConfig) })

	tradeNo := "creem-mismatched-provider"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          508,
		Amount:          5_000_000,
		Money:           10,
		TradeNo:         tradeNo,
		PaymentMethod:   model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderStripe,
		QuotaToAdd:      5_000_000,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}).Error)

	body := fmt.Sprintf(`{"eventType":"checkout.completed","object":{"request_id":%q,"order":{"id":"creem-order-508","status":"paid","type":"onetime","amount_paid":1000,"currency":"USD"},"customer":{"email":"mismatch@example.com","name":"Mismatch User"}}}`, tradeNo)
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(body))
	req.Header.Set(CreemSignatureHeader, generateCreemSignature(body, setting.GetCreemConfig().WebhookSecret))
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/creem/webhook", CreemWebhook)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, model.PaymentProviderStripe, topUp.PaymentProvider)
	assert.Equal(t, int64(5_000_000), topUp.Amount)
}

func TestCreemWebhookAcknowledgesSnapshotMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalConfig := setting.GetCreemConfig()
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey:        "creem-test-api-key",
		TestMode:      true,
		WebhookSecret: "creem-test-webhook-secret",
	})
	t.Cleanup(func() { setting.PublishCreemConfig(originalConfig) })

	tradeNo := "creem-snapshot-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 509, Amount: 5_000_000, RequestedAmount: 10, Money: 10,
		TradeNo: tradeNo, PaymentMethod: model.PaymentMethodCreem,
		PaymentProvider: model.PaymentProviderCreem, CreemProductID: "prod_expected",
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		QuotaToAdd: 5_000_000, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}).Error)

	body := fmt.Sprintf(`{"eventType":"checkout.completed","object":{"request_id":%q,"order":{"id":"creem-order-509","status":"paid","type":"onetime","amount_paid":900,"currency":"USD","product":"prod_expected"},"product":{"id":"prod_expected"},"customer":{"email":"mismatch@example.com","name":"Mismatch User"}}}`, tradeNo)
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(body))
	req.Header.Set(CreemSignatureHeader, generateCreemSignature(body, setting.GetCreemConfig().WebhookSecret))
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/creem/webhook", CreemWebhook)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestCreemWebhookRejectsMismatchedEnvironment(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalConfig := setting.GetCreemConfig()
	setting.PublishCreemConfig(setting.CreemConfig{APIKey: "creem-test-api-key", TestMode: true, WebhookSecret: "creem-test-webhook-secret"})
	t.Cleanup(func() { setting.PublishCreemConfig(originalConfig) })

	body := `{"eventType":"checkout.completed","object":{"mode":"live","order":{"status":"paid","type":"onetime","amount_paid":1000,"currency":"USD"}}}`
	req := httptest.NewRequest(http.MethodPost, "/api/creem/webhook", strings.NewReader(body))
	req.Header.Set(CreemSignatureHeader, generateCreemSignature(body, setting.GetCreemConfig().WebhookSecret))
	recorder := httptest.NewRecorder()
	router := gin.New()
	router.POST("/api/creem/webhook", CreemWebhook)
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestCompleteNOWPaymentsPaymentAcknowledgesDeletedSnapshotUser(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "now-retryable"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               404,
		RequestedAmount:      1,
		Money:                1,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodNOWPayments,
		PaymentProvider:      model.PaymentProviderNOWPayments,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    1,
		PaymentChargedAmount: 1,
		PaymentCoefficient:   1,
		QuotaToAdd:           100,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)

	statusCode, err := completeNOWPaymentsPayment(&service.NOWPaymentsPayment{
		PaymentID:     "payment-1",
		PaymentStatus: "finished",
		OrderID:       tradeNo,
		PriceAmount:   decimal.NewFromInt(1).String(),
		PriceCurrency: "USD",
	}, "127.0.0.1")

	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, statusCode)
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestCompleteNOWPaymentsPaymentAcknowledgesImmutableSnapshotMismatch(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "now-invalid-snapshot"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               405,
		RequestedAmount:      1,
		Money:                1,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodNOWPayments,
		PaymentProvider:      model.PaymentProviderNOWPayments,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    1,
		PaymentChargedAmount: 1,
		PaymentCoefficient:   1,
		QuotaToAdd:           100,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)

	statusCode, err := completeNOWPaymentsPayment(&service.NOWPaymentsPayment{
		PaymentID:     "payment-2",
		PaymentStatus: "finished",
		OrderID:       tradeNo,
		PriceAmount:   "9.00",
		PriceCurrency: "USD",
	}, "127.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestCompleteNOWPaymentsPaymentAcknowledgesTerminalTopUpState(t *testing.T) {
	setupPaymentLifecycleDB(t)
	tradeNo := "now-expired"
	require.NoError(t, model.DB.Create(&model.User{
		Id: 407, Username: "now-expired-user", Status: common.UserStatusEnabled,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               407,
		RequestedAmount:      1,
		Money:                1,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodNOWPayments,
		PaymentProvider:      model.PaymentProviderNOWPayments,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    1,
		PaymentChargedAmount: 1,
		PaymentCoefficient:   1,
		QuotaToAdd:           100,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusFailed,
	}).Error)

	statusCode, err := completeNOWPaymentsPayment(&service.NOWPaymentsPayment{
		PaymentID:     "payment-3",
		PaymentStatus: "finished",
		OrderID:       tradeNo,
		PriceAmount:   "1.00",
		PriceCurrency: "USD",
	}, "127.0.0.1")

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, statusCode)
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 407).Error)
	assert.Zero(t, user.Quota)
}

func TestStripeWebhookPermanentErrorClassification(t *testing.T) {
	err := markStripeWebhookPermanent(errors.New("snapshot mismatch"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, stripePermanentWebhookError))
	assert.False(t, errors.Is(errors.New("database unavailable"), stripePermanentWebhookError))
}

func TestNOWPaymentsCreateRejectsMissingUserBeforeProviderCall(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalEnabled := setting.NOWPaymentsEnabled
	originalAPIKey := setting.NOWPaymentsAPIKey
	originalIPNSecret := setting.NOWPaymentsIPNSecret
	originalCallbackURL := setting.NOWPaymentsIPNCallbackURL
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		setting.NOWPaymentsEnabled = originalEnabled
		setting.NOWPaymentsAPIKey = originalAPIKey
		setting.NOWPaymentsIPNSecret = originalIPNSecret
		setting.NOWPaymentsIPNCallbackURL = originalCallbackURL
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
	})
	setting.NOWPaymentsEnabled = true
	setting.NOWPaymentsAPIKey = "api-key"
	setting.NOWPaymentsIPNSecret = "ipn-secret"
	setting.NOWPaymentsIPNCallbackURL = "https://example.test/nowpayments"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/nowpayments", func(c *gin.Context) {
		c.Set("id", 999)
		RequestNOWPaymentsPay(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/nowpayments", strings.NewReader(`{"amount":1,"payment_method":"nowpayments"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "User does not exist")
	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	assert.Zero(t, count)
}

func TestEpayNotifyAcknowledgesOnlyAfterLocalRecharge(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
		operation_setting.PayMethods = originalMethods
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	tradeNo := "epay-retryable"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               406,
		RequestedAmount:      10,
		Money:                10,
		TradeNo:              tradeNo,
		PaymentMethod:        "alipay",
		PaymentProvider:      model.PaymentProviderEpay,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    10,
		PaymentChargedAmount: 10,
		PaymentCoefficient:   1,
		QuotaToAdd:           1000,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"trade_no":     "provider-trade",
		"out_trade_no": tradeNo,
		"type":         "alipay",
		"name":         "top-up",
		"money":        "10.00",
		"trade_status": epay.StatusTradeSuccess,
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/epay", EpayNotify)
	req := httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestEpayNotifyAcknowledgesVerifiedSnapshotMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
		operation_setting.PayMethods = originalMethods
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	tradeNo := "epay-snapshot-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:               409,
		RequestedAmount:      10,
		Money:                10,
		TradeNo:              tradeNo,
		PaymentMethod:        "alipay",
		PaymentProvider:      model.PaymentProviderEpay,
		PaymentCurrency:      "USD",
		PaymentBaseAmount:    10,
		PaymentChargedAmount: 10,
		PaymentCoefficient:   1,
		QuotaToAdd:           1000,
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid":          operation_setting.EpayId,
		"trade_no":     "provider-trade",
		"out_trade_no": tradeNo,
		"type":         "alipay",
		"name":         "top-up",
		"money":        "9.00",
		"trade_status": epay.StatusTradeSuccess,
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestEpayNotifyAcknowledgesPaymentMethodMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
		operation_setting.PayMethods = originalMethods
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}, {"type": "wxpay"}}
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	tradeNo := "epay-method-mismatch"
	require.NoError(t, model.DB.Create(&model.User{Id: 410, Username: "epay-method-user", Status: common.UserStatusEnabled, Quota: 100}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 410, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
		Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "wxpay", "name": "top-up",
		"money": "10.00", "trade_status": epay.StatusTradeSuccess,
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	topUp := model.GetTopUpByTradeNo(tradeNo)
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	assert.Equal(t, "alipay", topUp.PaymentMethod)
	var user model.User
	require.NoError(t, model.DB.First(&user, 410).Error)
	assert.Equal(t, 100, user.Quota)
}

func TestEpayNotifyAcknowledgesProviderMismatchWithoutCredit(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"

	tradeNo := "epay-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 416, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "stripe", PaymentProvider: model.PaymentProviderStripe,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
		Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "alipay", "name": "top-up",
		"money": "10.00", "trade_status": epay.StatusTradeSuccess,
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestEpayNotifyClosesExplicitTerminalTopUpStatus(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
		operation_setting.PayMethods = originalMethods
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	tradeNo := "epay-terminal"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 407, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
		Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "alipay", "name": "top-up",
		"money": "10.00", "trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusFailed, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestEpayNotifyAcknowledgesTerminalPaymentMethodMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	originalMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
		operation_setting.PayMethods = originalMethods
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}, {"type": "wxpay"}}

	tradeNo := "epay-terminal-method-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 411, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
		Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "wxpay", "name": "top-up",
		"money": "10.00", "trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestEpayNotifyAcknowledgesTerminalProviderMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"

	tradeNo := "epay-terminal-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 414, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "stripe", PaymentProvider: model.PaymentProviderStripe,
		PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
		PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(),
		Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "alipay", "name": "top-up",
		"money": "10.00", "trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	router := gin.New()
	router.GET("/epay", EpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	assert.Equal(t, common.TopUpStatusPending, model.GetTopUpByTradeNo(tradeNo).Status)
}

func TestSubscriptionEpayNotifyAcknowledgesTerminalPaymentMethodMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"

	tradeNo := "epay-sub-terminal-method-mismatch"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 412, PlanId: 1, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "wxpay", "name": "subscription",
		"money": "10.00", "trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	router := gin.New()
	router.GET("/subscription/epay", SubscriptionEpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/subscription/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestSubscriptionEpayNotifyAcknowledgesTerminalProviderMismatchWithoutClosingOrder(t *testing.T) {
	setupPaymentLifecycleDB(t)
	originalAddress := operation_setting.PayAddress
	originalID := operation_setting.EpayId
	originalKey := operation_setting.EpayKey
	t.Cleanup(func() {
		operation_setting.PayAddress = originalAddress
		operation_setting.EpayId = originalID
		operation_setting.EpayKey = originalKey
	})
	operation_setting.PayAddress = "https://epay.example.test"
	operation_setting.EpayId = "merchant"
	operation_setting.EpayKey = "secret"

	tradeNo := "epay-sub-terminal-provider-mismatch"
	require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
		UserId: 415, PlanId: 1, Money: 10, TradeNo: tradeNo,
		PaymentMethod: "stripe", PaymentProvider: model.PaymentProviderStripe,
		CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
	}).Error)

	params := map[string]string{
		"pid": operation_setting.EpayId, "trade_no": "provider-trade",
		"out_trade_no": tradeNo, "type": "alipay", "name": "subscription",
		"money": "10.00", "trade_status": "TRADE_CLOSED",
	}
	signed := epay.GenerateParams(params, operation_setting.EpayKey)
	query := url.Values{}
	for key, value := range signed {
		query.Set(key, value)
	}

	router := gin.New()
	router.GET("/subscription/epay", SubscriptionEpayNotify)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/subscription/epay?"+query.Encode(), nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
	order := model.GetSubscriptionOrderByTradeNo(tradeNo)
	require.NotNil(t, order)
	assert.Equal(t, common.TopUpStatusPending, order.Status)
}

func TestSubscriptionEpayCallbacksKeepPendingOrderOnPaymentMethodMismatch(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		handler     gin.HandlerFunc
		tradeStatus string
		statusCode  int
	}{
		{
			name:        "notify success",
			path:        "/subscription/epay/notify",
			handler:     SubscriptionEpayNotify,
			tradeStatus: epay.StatusTradeSuccess,
			statusCode:  http.StatusOK,
		},
		{
			name:        "return success",
			path:        "/subscription/epay/return",
			handler:     SubscriptionEpayReturn,
			tradeStatus: epay.StatusTradeSuccess,
			statusCode:  http.StatusFound,
		},
		{
			name:        "return terminal",
			path:        "/subscription/epay/return",
			handler:     SubscriptionEpayReturn,
			tradeStatus: "TRADE_CLOSED",
			statusCode:  http.StatusFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupPaymentLifecycleDB(t)
			originalAddress := operation_setting.PayAddress
			originalID := operation_setting.EpayId
			originalKey := operation_setting.EpayKey
			t.Cleanup(func() {
				operation_setting.PayAddress = originalAddress
				operation_setting.EpayId = originalID
				operation_setting.EpayKey = originalKey
			})
			operation_setting.PayAddress = "https://epay.example.test"
			operation_setting.EpayId = "merchant"
			operation_setting.EpayKey = "secret"

			tradeNo := "epay-sub-" + strings.ReplaceAll(tt.name, " ", "-")
			require.NoError(t, model.DB.Create(&model.SubscriptionOrder{
				UserId: 413, PlanId: 1, Money: 10, TradeNo: tradeNo,
				PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
				CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending,
			}).Error)

			params := map[string]string{
				"pid": operation_setting.EpayId, "trade_no": "provider-trade",
				"out_trade_no": tradeNo, "type": "wxpay", "name": "subscription",
				"money": "10.00", "trade_status": tt.tradeStatus,
			}
			signed := epay.GenerateParams(params, operation_setting.EpayKey)
			query := url.Values{}
			for key, value := range signed {
				query.Set(key, value)
			}

			router := gin.New()
			router.GET(tt.path, tt.handler)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, tt.path+"?"+query.Encode(), nil))

			require.Equal(t, tt.statusCode, recorder.Code)
			if tt.statusCode == http.StatusOK {
				assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
			} else {
				assert.Contains(t, recorder.Header().Get("Location"), "pay=fail")
			}
			order := model.GetSubscriptionOrderByTradeNo(tradeNo)
			require.NotNil(t, order)
			assert.Equal(t, common.TopUpStatusPending, order.Status)
		})
	}
}

func TestEpayNotifyAcknowledgesTerminalTopUpBeforeAmountValidation(t *testing.T) {
	for _, status := range []string{common.TopUpStatusFailed, common.TopUpStatusExpired} {
		t.Run(status, func(t *testing.T) {
			setupPaymentLifecycleDB(t)
			originalAddress := operation_setting.PayAddress
			originalID := operation_setting.EpayId
			originalKey := operation_setting.EpayKey
			originalMethods := operation_setting.PayMethods
			originalPaymentSetting := *operation_setting.GetPaymentSetting()
			t.Cleanup(func() {
				operation_setting.PayAddress = originalAddress
				operation_setting.EpayId = originalID
				operation_setting.EpayKey = originalKey
				operation_setting.PayMethods = originalMethods
				*operation_setting.GetPaymentSetting() = originalPaymentSetting
			})
			operation_setting.PayAddress = "https://epay.example.test"
			operation_setting.EpayId = "merchant"
			operation_setting.EpayKey = "secret"
			operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
			operation_setting.GetPaymentSetting().ComplianceConfirmed = true
			operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

			tradeNo := "epay-terminal-" + status
			require.NoError(t, model.DB.Create(&model.TopUp{
				UserId: 408, RequestedAmount: 10, Money: 10, TradeNo: tradeNo,
				PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
				PaymentCurrency: "USD", PaymentBaseAmount: 10, PaymentChargedAmount: 10,
				PaymentCoefficient: 1, QuotaToAdd: 1000, CreateTime: time.Now().Unix(), Status: status,
			}).Error)

			params := map[string]string{
				"pid": operation_setting.EpayId, "trade_no": "provider-trade",
				"out_trade_no": tradeNo, "type": "alipay", "name": "top-up",
				"money": "not-a-number", "trade_status": epay.StatusTradeSuccess,
			}
			signed := epay.GenerateParams(params, operation_setting.EpayKey)
			query := url.Values{}
			for key, value := range signed {
				query.Set(key, value)
			}
			recorder := httptest.NewRecorder()
			router := gin.New()
			router.GET("/epay", EpayNotify)
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/epay?"+query.Encode(), nil))
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Equal(t, "success", strings.TrimSpace(recorder.Body.String()))
			assert.Equal(t, status, model.GetTopUpByTradeNo(tradeNo).Status)
		})
	}
}

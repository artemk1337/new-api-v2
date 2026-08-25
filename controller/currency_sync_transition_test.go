package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdminUpdatePlatformCurrencyAllowsActiveYooKassaCurrencyChanges(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}, &model.User{}, &model.TopUp{}, &model.Log{}))
	require.NoError(t, db.Create([]model.PlatformCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1},
		{Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true, ManualRateToUSD: 90, RateToUSD: 90},
	}).Error)
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"yookassa_sbp","name":"SBP","currency":"RUB","topup_group":"default"}]`}).Error)

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalYooKassaConfig := setting.GetYooKassaConfig()
	originalPayMethods := operation_setting.PayMethods
	originalRedisEnabled := common.RedisEnabled
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodYooKassaSBP, "currency": "RUB", "topup_group": "default"}}
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		setting.PublishYooKassaConfig(originalYooKassaConfig)
		operation_setting.PayMethods = originalPayMethods
		common.RedisEnabled = originalRedisEnabled
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	update := func(request platformCurrencyRequest) model.PlatformCurrency {
		t.Helper()
		body, marshalErr := common.Marshal(request)
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "code", Value: "RUB"}}
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/currencies/admin/RUB", strings.NewReader(string(body)))
		AdminUpdatePlatformCurrency(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		var response struct {
			Success bool                   `json:"success"`
			Data    model.PlatformCurrency `json:"data"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.True(t, response.Success, recorder.Body.String())
		return response.Data
	}

	quote, err := service.BuildPaymentQuote(10, model.PaymentMethodYooKassaSBP, "default")
	require.NoError(t, err)
	assert.Equal(t, 90.0, quote.RateToUSD)

	updated := update(platformCurrencyRequest{ManualRateToUSD: 95})
	assert.False(t, updated.SyncEnabled)
	rate, err := service.GetPlatformCurrencyRate("RUB")
	require.NoError(t, err)
	assert.Equal(t, 95.0, rate)

	updated = update(platformCurrencyRequest{SyncEnabled: common.GetPointer(true), SyncProvider: "cbr"})
	assert.True(t, updated.SyncEnabled)
	assert.Zero(t, updated.RateToUSD)
	_, err = service.BuildPaymentQuote(10, model.PaymentMethodYooKassaSBP, "default")
	require.Error(t, err, "new orders must not use the invalidated synchronized rate")

	updated = update(platformCurrencyRequest{SyncEnabled: common.GetPointer(false), ManualRateToUSD: 100})
	assert.False(t, updated.SyncEnabled)
	rate, err = service.GetPlatformCurrencyRate("RUB")
	require.NoError(t, err)
	assert.Equal(t, 100.0, rate)

	require.NoError(t, db.Create(&model.User{Id: 701, Username: "snapshot-user", Status: common.UserStatusEnabled}).Error)
	tradeNo := "yookassa-rub-snapshot"
	require.NoError(t, db.Create(&model.TopUp{
		UserId:               701,
		Amount:               10,
		RequestedAmount:      10,
		Money:                900,
		TradeNo:              tradeNo,
		PaymentMethod:        model.PaymentMethodYooKassaSBP,
		PaymentProvider:      model.PaymentProviderYooKassa,
		QuotaToAdd:           12345,
		PaymentCurrency:      "RUB",
		PaymentRateToUSD:     90,
		PaymentCoefficient:   1,
		PaymentBaseAmount:    10,
		PaymentChargedAmount: 900,
		Status:               common.TopUpStatusPending,
		CreateTime:           time.Now().Unix(),
	}).Error)

	updated = update(platformCurrencyRequest{Enabled: common.GetPointer(false)})
	assert.False(t, updated.Enabled)
	_, err = service.BuildPaymentQuote(10, model.PaymentMethodYooKassaSBP, "default")
	require.Error(t, err, "disabled currency must make future quotes unavailable")

	require.NoError(t, model.RechargeYooKassa(tradeNo, "127.0.0.1"))
	var user model.User
	require.NoError(t, db.Select("quota").First(&user, 701).Error)
	assert.Equal(t, 12345, user.Quota, "pending orders must settle from their immutable snapshot")
}

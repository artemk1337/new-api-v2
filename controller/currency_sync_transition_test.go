package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdminUpdatePlatformCurrencyRejectsActivePaymentSyncTransition(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PlatformCurrency{}))
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true,
		ManualRateToUSD: 1, RateToUSD: 1,
	}).Error)
	require.NoError(t, db.Create(&model.PlatformCurrency{
		Code: "EUR", Name: "Euro", Symbol: "€", Enabled: true,
		ManualRateToUSD: 0.92, RateToUSD: 0.92,
	}).Error)

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalWaffoEnabled := setting.WaffoEnabled
	originalWaffoSandbox := setting.WaffoSandbox
	originalWaffoAPIKey := setting.WaffoApiKey
	originalWaffoPrivateKey := setting.WaffoPrivateKey
	originalWaffoPublicCert := setting.WaffoPublicCert
	originalWaffoCurrency := setting.WaffoCurrency
	originalCreemConfig := setting.GetCreemConfig()
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		setting.WaffoEnabled = originalWaffoEnabled
		setting.WaffoSandbox = originalWaffoSandbox
		setting.WaffoApiKey = originalWaffoAPIKey
		setting.WaffoPrivateKey = originalWaffoPrivateKey
		setting.WaffoPublicCert = originalWaffoPublicCert
		setting.WaffoCurrency = originalWaffoCurrency
		setting.PublishCreemConfig(originalCreemConfig)
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	update := func() bool {
		body, marshalErr := common.Marshal(platformCurrencyRequest{SyncEnabled: common.GetPointer(true), SyncProvider: "cbr"})
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Params = gin.Params{{Key: "code", Value: "EUR"}}
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/currencies/admin/EUR", strings.NewReader(string(body)))
		AdminUpdatePlatformCurrency(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}
	assertQuoteAvailable := func() {
		rate, rateErr := service.GetPlatformCurrencyRate("EUR")
		require.NoError(t, rateErr)
		require.Equal(t, 0.92, rate)
		currency, currencyErr := model.GetPlatformCurrency("EUR")
		require.NoError(t, currencyErr)
		require.False(t, currency.SyncEnabled)
	}

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = "waffo_api"
	setting.WaffoPrivateKey = "waffo_private"
	setting.WaffoPublicCert = "waffo_public"
	setting.WaffoCurrency = "EUR"
	require.False(t, update(), "active Waffo must keep its quote during sync transition")
	assertQuoteAvailable()

	setting.WaffoEnabled = false
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey: "creem_api", Products: `[{"name":"Euro","productId":"prod_eur","price":10,"currency":"EUR"}]`,
		TestMode: true,
	})
	require.False(t, update(), "active Creem must keep its quote during sync transition")
	assertQuoteAvailable()
}

package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionValidatesPaymentCurrencyReadiness(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}, &model.User{}, &model.Log{}))
	for _, currency := range []model.PlatformCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1},
		{Code: "EUR", Name: "Euro", Symbol: "€", Enabled: true, ManualRateToUSD: 0.92, RateToUSD: 0.92},
	} {
		require.NoError(t, db.Create(&currency).Error)
	}

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	originalWaffoEnabled := setting.WaffoEnabled
	originalWaffoSandbox := setting.WaffoSandbox
	originalWaffoAPIKey := setting.WaffoApiKey
	originalWaffoPrivateKey := setting.WaffoPrivateKey
	originalWaffoPublicCert := setting.WaffoPublicCert
	originalWaffoCurrency := setting.WaffoCurrency
	originalCreemConfig := setting.GetCreemConfig()
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB, model.LOG_DB = db, db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.RedisEnabled = originalRedisEnabled
		setting.WaffoEnabled = originalWaffoEnabled
		setting.WaffoSandbox = originalWaffoSandbox
		setting.WaffoApiKey = originalWaffoAPIKey
		setting.WaffoPrivateKey = originalWaffoPrivateKey
		setting.WaffoPublicCert = originalWaffoPublicCert
		setting.WaffoCurrency = originalWaffoCurrency
		setting.PublishCreemConfig(originalCreemConfig)
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	update := func(key, value string) bool {
		body, marshalErr := common.Marshal(OptionUpdateRequest{Key: key, Value: value})
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
		UpdateOption(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}
	confirmCompliance := func() bool {
		body, marshalErr := common.Marshal(PaymentComplianceRequest{Confirmed: true})
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/payment/compliance", bytes.NewReader(body))
		ConfirmPaymentCompliance(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}

	require.False(t, update("WaffoCurrency", "GBP"), "missing Waffo currency must be rejected")
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "EUR").Update("enabled", false).Error)
	require.False(t, update("WaffoCurrency", "EUR"), "disabled Waffo currency must be rejected")
	require.True(t, update("WaffoCurrency", "USD"), "enabled Waffo currency remains valid")

	setting.WaffoEnabled = false
	setting.WaffoSandbox = false
	setting.WaffoApiKey = "waffo_api"
	setting.WaffoPrivateKey = "waffo_private"
	setting.WaffoPublicCert = "waffo_public"
	setting.WaffoCurrency = "EUR"
	require.True(t, update("WaffoEnabled", "true"), "enabling Waffo must use the latest persisted currency")
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "EUR").Update("enabled", true).Error)
	require.True(t, update("WaffoEnabled", "true"), "Waffo may be enabled with an enabled currency")
	require.True(t, update("WaffoEnabled", "false"))
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "WaffoCurrency").Update("value", "EUR").Error)
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "EUR").Updates(map[string]any{
		"manual_rate_to_usd": 0,
		"rate_to_usd":        0,
	}).Error)
	require.False(t, update("WaffoEnabled", "true"), "enabled currency without a quote must not enable Waffo")
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "EUR").Updates(map[string]any{
		"manual_rate_to_usd": 0.92,
		"rate_to_usd":        0.92,
	}).Error)
	require.True(t, update("WaffoEnabled", "true"))

	setting.PublishCreemConfig(setting.CreemConfig{Products: "[]"})
	require.False(t, update("CreemProducts", `[{"name":"Euro","productId":"prod_eur","price":10,"currency":"EUR","quota":1}]`))
	require.False(t, update("CreemApiKey", "creem_api"))
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "EUR").Update("enabled", false).Error)
	require.False(t, update("CreemWebhookSecret", "creem_webhook"))

	setting.WaffoEnabled = true
	setting.WaffoCurrency = "EUR"
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "WaffoCurrency").Update("value", "EUR").Error)
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "WaffoCurrency").Update("value", "USD").Error)
	require.Contains(t, activePaymentCurrencyDependenciesFromDB(db, "USD"), "Waffo", "dependency checks must use DB Waffo state, not stale local EUR")
	require.NoError(t, db.Model(&model.Option{}).Where("key = ?", "WaffoCurrency").Update("value", "EUR").Error)
	setting.PublishCreemConfig(setting.CreemConfig{Products: "[]"})
	require.False(t, confirmCompliance(), "compliance confirmation must not activate Waffo with an unavailable currency")

	setting.WaffoEnabled = false
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey: "creem_api", Products: `[{"name":"Euro","productId":"prod_eur","price":10,"currency":"EUR","quota":1}]`,
		WebhookSecret: "creem_webhook",
	})
	require.False(t, confirmCompliance(), "compliance confirmation must not activate Creem with unavailable catalog currencies")
}

func TestYooKassaReadinessUsesProspectivePayMethods(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	for _, option := range []model.Option{
		{Key: "payment_setting.compliance_confirmed", Value: "true"},
		{Key: "YooKassaEnabled", Value: "true"},
		{Key: "PayMethods", Value: `[{"type":"alipay"}]`},
	} {
		require.NoError(t, db.Create(&option).Error)
	}

	originalMethods := operation_setting.PayMethods
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	t.Cleanup(func() { operation_setting.PayMethods = originalMethods })

	// A later YooKassa option update must not require RUB when the persisted
	// checkout list no longer exposes SBP.
	returnURLUpdate := map[string]string{"YooKassaReturnURL": "https://example.test/return"}
	require.NoError(t, validatePaymentSettingsReadinessFromDB(db, returnURLUpdate, setting.CreemConfig{}))

	// Both single-option and bulk updates must evaluate the incoming
	// PayMethods payload, not a stale legacy YooKassaPaymentMethods value.
	activeMethods := `[{"type":"alipay"},{"type":"yookassa_sbp"}]`
	require.Error(t, validatePaymentSettingsReadinessFromDB(db, map[string]string{
		"PayMethods": activeMethods,
	}, setting.CreemConfig{}))
	require.Error(t, validatePaymentSettingsReadinessFromDB(db, map[string]string{
		"PayMethods":        activeMethods,
		"YooKassaReturnURL": "https://example.test/return",
	}, setting.CreemConfig{}))

	removedMethods := `[{"type":"alipay"}]`
	require.NoError(t, validatePaymentSettingsReadinessFromDB(db, map[string]string{
		"PayMethods":        removedMethods,
		"YooKassaReturnURL": "https://example.test/return",
	}, setting.CreemConfig{}))
}

func TestUpdateOptionRejectsCreemConfigKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	original := setting.GetCreemConfig()
	t.Cleanup(func() { setting.PublishCreemConfig(original) })

	for _, key := range []string{"CreemApiKey", "CreemProducts", "CreemTestMode", "CreemWebhookSecret"} {
		body, err := common.Marshal(OptionUpdateRequest{Key: key, Value: "changed"})
		require.NoError(t, err)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
		UpdateOption(ctx)

		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		require.False(t, response.Success, "generic update must reject %s", key)
	}
}

func TestSaveCreemConfigIsAtomic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}))
	require.NoError(t, db.Create(&model.PlatformCurrency{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1}).Error)

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalConfig := setting.GetCreemConfig()
	originalMinTopUp := operation_setting.MinTopUp
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		setting.PublishCreemConfig(originalConfig)
		operation_setting.MinTopUp = originalMinTopUp
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	setting.PublishCreemConfig(setting.CreemConfig{
		APIKey: "old-api", WebhookSecret: "old-webhook",
		Products: `[{"name":"Old","productId":"old","price":1,"currency":"USD","quota":1}]`,
	})

	save := func(payload saveCreemConfigRequest) bool {
		body, marshalErr := common.Marshal(payload)
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option/creem/save", bytes.NewReader(body))
		SaveCreemConfig(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}

	newAPIKey := "new-api"
	invalidProducts := `[{"name":"Invalid","productId":"invalid","price":1,"currency":"CNY"}]`
	require.False(t, save(saveCreemConfigRequest{APIKey: &newAPIKey, Products: &invalidProducts}))
	nullProducts := "null"
	require.False(t, save(saveCreemConfigRequest{APIKey: &newAPIKey, Products: &nullProducts}))
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Where("key IN ?", []string{"CreemApiKey", "CreemWebhookSecret", "CreemTestMode", "CreemProducts"}).Count(&count).Error)
	require.Zero(t, count, "invalid prospective config must not persist a partial update")

	products := `[{"name":"Basic","productId":"basic","price":10,"currency":"USD","quota":1}]`
	testMode := true
	require.True(t, save(saveCreemConfigRequest{APIKey: &newAPIKey, TestMode: &testMode, Products: &products}))
	var options []model.Option
	require.NoError(t, db.Where("key IN ?", []string{"CreemApiKey", "CreemWebhookSecret", "CreemTestMode", "CreemProducts"}).Find(&options).Error)
	require.Len(t, options, 4)
	assertOptionValue := func(key, want string) {
		for _, option := range options {
			if option.Key == key {
				require.Equal(t, want, option.Value)
				return
			}
		}
		t.Fatalf("option %s not found", key)
	}
	assertOptionValue("CreemApiKey", newAPIKey)
	assertOptionValue("CreemWebhookSecret", "old-webhook")
	assertOptionValue("CreemTestMode", "true")
	snapshot := setting.GetCreemConfig()
	require.Equal(t, newAPIKey, snapshot.APIKey)
	require.Equal(t, "old-webhook", snapshot.WebhookSecret)
	require.True(t, snapshot.TestMode)
	require.JSONEq(t, `[{"name":"Basic","productId":"basic","price":10,"currency":"USD","quota":1}]`, snapshot.Products)

	// A later partial update must merge with the committed snapshot rather than
	// restoring stale fields from the request handler's initial read.
	newWebhook := "new-webhook"
	require.True(t, save(saveCreemConfigRequest{WebhookSecret: &newWebhook}))
	snapshot = setting.GetCreemConfig()
	require.Equal(t, newAPIKey, snapshot.APIKey)
	require.Equal(t, newWebhook, snapshot.WebhookSecret)
	require.JSONEq(t, products, snapshot.Products)
}

func TestSavePaymentSettingsIsAtomicAcrossGenericAndCreem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}))
	require.NoError(t, db.Create(&model.PlatformCurrency{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1}).Error)

	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalConfig := setting.GetCreemConfig()
	originalMinTopUp := operation_setting.MinTopUp
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB, model.LOG_DB = db, db
	setting.PublishCreemConfig(setting.CreemConfig{APIKey: "old-api", WebhookSecret: "old-webhook", Products: "[]"})
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		setting.PublishCreemConfig(originalConfig)
		operation_setting.MinTopUp = originalMinTopUp
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	save := func(payload savePaymentSettingsRequest) bool {
		body, marshalErr := common.Marshal(payload)
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/option/payment-settings/save", bytes.NewReader(body))
		SavePaymentSettings(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}

	invalidProducts := `[{"productId":"invalid","price":1,"currency":"CNY","quota":1}]`
	require.False(t, save(savePaymentSettingsRequest{
		Options: []OptionUpdateRequest{{Key: "MinTopUp", Value: "12.5"}},
		Creem:   &saveCreemConfigRequest{APIKey: stringPtr("new-api"), Products: stringPtr(invalidProducts)},
	}))
	require.False(t, save(savePaymentSettingsRequest{
		Creem: &saveCreemConfigRequest{APIKey: stringPtr("new-api"), Products: stringPtr("null")},
	}))
	var count int64
	require.NoError(t, db.Model(&model.Option{}).Count(&count).Error)
	require.Zero(t, count, "invalid Creem config must roll back generic options")
	require.Equal(t, "old-api", setting.GetCreemConfig().APIKey)

	products := `[{"name":"","productId":"basic","price":10,"currency":"USD","quota":1}]`
	require.True(t, save(savePaymentSettingsRequest{
		Options: []OptionUpdateRequest{{Key: "MinTopUp", Value: "12.5"}},
		Creem:   &saveCreemConfigRequest{APIKey: stringPtr("new-api"), TestMode: boolPtr(true), Products: stringPtr(products)},
	}))
	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "MinTopUp").Error)
	require.Equal(t, "12.5", option.Value)
	option = model.Option{}
	require.NoError(t, db.First(&option, "key = ?", "CreemApiKey").Error)
	require.Equal(t, "new-api", option.Value)
	snapshot := setting.GetCreemConfig()
	require.Equal(t, "new-api", snapshot.APIKey)
	require.True(t, snapshot.TestMode)
	require.JSONEq(t, products, snapshot.Products)
}

func stringPtr(value string) *string { return &value }

func boolPtr(value bool) *bool { return &value }

func TestNormalizeCreemProductsRequiresContractFields(t *testing.T) {
	for name, raw := range map[string]string{
		"missing product id": `[{"price":1,"currency":"USD","quota":1}]`,
		"missing quota":      `[{"productId":"prod","price":1,"currency":"USD"}]`,
		"zero quota":         `[{"productId":"prod","price":1,"currency":"USD","quota":0}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := normalizeCreemProducts(raw)
			require.Error(t, err)
		})
	}
}

func TestFixedPaymentReadinessRequiresPlatformRate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}))
	for _, currency := range []model.PlatformCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1},
		{Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true, ManualRateToUSD: 90, RateToUSD: 90},
		{Code: "USDT", Name: "Tether", Symbol: "₮", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1},
	} {
		require.NoError(t, db.Create(&currency).Error)
	}
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"yookassa_sbp"}]`}).Error)
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalYooKassaConfig := setting.GetYooKassaConfig()
	originalNowEnabled, originalNowAPI, originalNowSecret, originalNowCallback := setting.NOWPaymentsEnabled, setting.NOWPaymentsAPIKey, setting.NOWPaymentsIPNSecret, setting.NOWPaymentsIPNCallbackURL
	model.DB, model.LOG_DB = db, db
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		setting.PublishYooKassaConfig(originalYooKassaConfig)
		setting.NOWPaymentsEnabled, setting.NOWPaymentsAPIKey, setting.NOWPaymentsIPNSecret, setting.NOWPaymentsIPNCallbackURL = originalNowEnabled, originalNowAPI, originalNowSecret, originalNowCallback
	})

	setting.PublishYooKassaConfig(setting.YooKassaConfig{ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	require.NoError(t, validatePaymentCurrencyReadiness("YooKassaEnabled", "true"))
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "RUB").Updates(map[string]any{"manual_rate_to_usd": 0, "rate_to_usd": 0}).Error)
	require.Error(t, validatePaymentCurrencyReadiness("YooKassaEnabled", "true"))

	setting.NOWPaymentsEnabled = false
	setting.NOWPaymentsAPIKey, setting.NOWPaymentsIPNSecret, setting.NOWPaymentsIPNCallbackURL = "api", "secret", "https://example.test/ipn"
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "USDT").Update("rate_to_usd", 1).Error)
	require.NoError(t, validatePaymentCurrencyReadiness("NOWPaymentsEnabled", "true"))
	require.NoError(t, db.Model(&model.PlatformCurrency{}).Where("code = ?", "USDT").Update("enabled", false).Error)
	require.Error(t, validatePaymentCurrencyReadiness("NOWPaymentsEnabled", "true"))
}

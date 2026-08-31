package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/currency_exchange_rate_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type platformCurrencyRequest struct {
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	Symbol          string  `json:"symbol"`
	Enabled         *bool   `json:"enabled"`
	SyncEnabled     *bool   `json:"sync_enabled"`
	SyncProvider    string  `json:"sync_provider"`
	ManualRateToUSD float64 `json:"manual_rate_to_usd"`
}

type currencySyncConfigRequest struct {
	UpdateInterval string `json:"update_interval"`
}

var fetchPlatformCurrencyRate = service.FetchPlatformCurrencyRate

func AdminGetCurrencySyncConfig(c *gin.Context) {
	config := currency_exchange_rate_setting.GetConfig()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{
		"update_interval":   config.UpdateInterval,
		"allowed_intervals": []string{currency_exchange_rate_setting.UpdateIntervalMinute, currency_exchange_rate_setting.UpdateIntervalHour, currency_exchange_rate_setting.UpdateIntervalDay},
	}})
}

func AdminUpdateCurrencySyncConfig(c *gin.Context) {
	var request currencySyncConfigRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid sync configuration payload")
		return
	}
	if err := currency_exchange_rate_setting.ValidateUpdateInterval(request.UpdateInterval); err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.UpdateOption("currency_exchange_rate.update_interval", request.UpdateInterval); err != nil {
		common.ApiError(c, err)
		return
	}
	AdminGetCurrencySyncConfig(c)
}

func AdminSyncAllPlatformCurrencies(c *gin.Context) {
	if err := service.UpdatePlatformCurrencies(c.Request.Context()); err != nil {
		common.ApiError(c, err)
		return
	}
	currencies, err := model.ListPlatformCurrencies(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, currencies)
}

var platformCurrencyCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{1,7}$`)

func validatePlatformCurrency(currency *model.PlatformCurrency) error {
	if !platformCurrencyCodePattern.MatchString(currency.Code) {
		return errors.New("currency code must contain 2-8 uppercase letters or digits")
	}
	if currency.Name == "" || currency.Symbol == "" {
		return errors.New("currency name and symbol are required")
	}
	if currency.Code == "USD" && !currency.Enabled {
		return errors.New("USD is the required base currency and cannot be disabled")
	}
	if currency.SyncEnabled {
		if currency.SyncProvider == "" {
			return errors.New("sync_provider is required when synchronization is enabled")
		}
		if err := currency_exchange_rate_setting.ValidateProvider(currency.SyncProvider); err != nil {
			return err
		}
		if !currency_exchange_rate_setting.IsProviderSupportedForCurrency(currency.SyncProvider, currency.Code) {
			return errors.New("selected synchronization source does not support USD/" + currency.Code)
		}
	}
	if !currency.SyncEnabled && currency.ManualRateToUSD <= 0 {
		return errors.New("manual_rate_to_usd must be greater than zero when synchronization is disabled")
	}
	return nil
}

func invalidatePlatformCurrencySyncState(currency *model.PlatformCurrency) {
	currency.LastSyncAt = nil
	currency.LastSyncError = ""
	if currency.SyncEnabled {
		currency.RateToUSD = 0
		return
	}
	currency.RateToUSD = currency.ManualRateToUSD
}

// preparePlatformCurrencySync obtains a quote before synchronization is made
// visible. This prevents an enabled payment currency from ever exposing an
// empty rate while its first background synchronization is pending.
func preparePlatformCurrencySync(ctx context.Context, currency *model.PlatformCurrency) (model.CurrencyExchangeRate, bool, error) {
	if !currency.SyncEnabled {
		return model.CurrencyExchangeRate{}, false, nil
	}
	rate, err := fetchPlatformCurrencyRate(ctx, currency.SyncProvider, currency.Code)
	if err != nil {
		return model.CurrencyExchangeRate{}, false, err
	}
	syncedAt := time.Now().UTC()
	currency.RateToUSD = rate
	currency.LastSyncAt = &syncedAt
	currency.LastSyncError = ""
	return model.CurrencyExchangeRate{
		BaseCurrency: "USD", QuoteCurrency: currency.Code, Provider: currency.SyncProvider,
		Rate: rate, RecordedAt: syncedAt,
	}, true, nil
}

// activePaymentCurrencyDependencies returns enabled checkout integrations that
// require a registry currency. Deleting such a row remains prohibited: unlike
// disabling it, deletion prevents an administrator from restoring the same
// configuration without recreating the currency.
func activePaymentCurrencyDependencies(code string) []string {
	return activePaymentCurrencyDependenciesFromDB(model.DB, code)
}

func activePaymentCurrencyDependenciesFromDB(db *gorm.DB, code string) []string {
	code = strings.ToUpper(strings.TrimSpace(code))
	dependencies := make([]string, 0, 3)
	complianceConfirmed := latestPaymentOptionBoolFromDB(db, "payment_setting.compliance_confirmed", operation_setting.IsPaymentComplianceConfirmed())
	yooKassaConfig := setting.GetYooKassaConfig()
	yooEnabled := latestPaymentOptionBoolFromDB(db, "YooKassaEnabled", yooKassaConfig.Enabled)
	yooShopID := latestPaymentOptionFromDB(db, "YooKassaShopID", yooKassaConfig.ShopID)
	yooSecret := latestPaymentOptionFromDB(db, "YooKassaSecretKey", yooKassaConfig.SecretKey)
	payMethods := latestPaymentOptionFromDB(db, "PayMethods", operation_setting.PayMethods2JsonString())
	if code == "RUB" && complianceConfirmed && yooEnabled && strings.TrimSpace(yooShopID) != "" && strings.TrimSpace(yooSecret) != "" && paymentMethodsJSONContainsType(payMethods, model.PaymentMethodYooKassaSBP) {
		dependencies = append(dependencies, "YooKassa SBP")
	}
	nowEnabled := latestPaymentOptionBoolFromDB(db, "NOWPaymentsEnabled", setting.NOWPaymentsEnabled)
	nowAPIKey := latestPaymentOptionFromDB(db, "NOWPaymentsAPIKey", setting.NOWPaymentsAPIKey)
	nowSecret := latestPaymentOptionFromDB(db, "NOWPaymentsIPNSecret", setting.NOWPaymentsIPNSecret)
	nowCallback := latestPaymentOptionFromDB(db, "NOWPaymentsIPNCallbackURL", setting.NOWPaymentsIPNCallbackURL)
	if code == "USDT" && complianceConfirmed && nowEnabled && strings.TrimSpace(nowAPIKey) != "" && strings.TrimSpace(nowSecret) != "" && strings.TrimSpace(nowCallback) != "" {
		dependencies = append(dependencies, "NOWPayments")
	}
	waffoEnabled := latestPaymentOptionBoolFromDB(db, "WaffoEnabled", setting.WaffoEnabled)
	waffoCurrency := latestPaymentOptionFromDB(db, "WaffoCurrency", setting.WaffoCurrency)
	waffoSandbox := latestPaymentOptionBoolFromDB(db, "WaffoSandbox", setting.WaffoSandbox)
	waffoReady := waffoEnabled && complianceConfirmed
	if waffoSandbox {
		waffoReady = waffoReady && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoSandboxApiKey", setting.WaffoSandboxApiKey)) != "" && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoSandboxPrivateKey", setting.WaffoSandboxPrivateKey)) != "" && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoSandboxPublicCert", setting.WaffoSandboxPublicCert)) != ""
	} else {
		waffoReady = waffoReady && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoApiKey", setting.WaffoApiKey)) != "" && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoPrivateKey", setting.WaffoPrivateKey)) != "" && strings.TrimSpace(latestPaymentOptionFromDB(db, "WaffoPublicCert", setting.WaffoPublicCert)) != ""
	}
	if waffoReady && strings.EqualFold(code, strings.TrimSpace(waffoCurrency)) {
		dependencies = append(dependencies, "Waffo")
	}
	config := setting.GetCreemConfig()
	config.APIKey = latestPaymentOptionFromDB(db, "CreemApiKey", config.APIKey)
	config.Products = latestPaymentOptionFromDB(db, "CreemProducts", config.Products)
	config.WebhookSecret = latestPaymentOptionFromDB(db, "CreemWebhookSecret", config.WebhookSecret)
	config.TestMode = latestPaymentOptionBoolFromDB(db, "CreemTestMode", config.TestMode)
	if complianceConfirmed && strings.TrimSpace(config.APIKey) != "" && strings.TrimSpace(config.Products) != "" && strings.TrimSpace(config.Products) != "[]" && (config.TestMode || strings.TrimSpace(config.WebhookSecret) != "") {
		var products []CreemProduct
		if common.Unmarshal([]byte(config.Products), &products) == nil {
			for _, product := range products {
				if strings.EqualFold(code, strings.TrimSpace(product.Currency)) {
					dependencies = append(dependencies, "Creem")
					break
				}
			}
		}
	}
	return dependencies
}

func latestPaymentOption(key, fallback string) string {
	return latestPaymentOptionFromDB(model.DB, key, fallback)
}

func latestPaymentOptionFromDB(db *gorm.DB, key, fallback string) string {
	if db == nil {
		return fallback
	}
	var option model.Option
	if err := db.Where("key = ?", key).First(&option).Error; err == nil {
		return option.Value
	}
	return fallback
}

func latestPaymentOptionBool(key string, fallback bool) bool {
	return latestPaymentOptionBoolFromDB(model.DB, key, fallback)
}

func latestPaymentOptionBoolFromDB(db *gorm.DB, key string, fallback bool) bool {
	value := latestPaymentOptionFromDB(db, key, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func paymentMethodListContains(methods, wanted string) bool {
	for _, method := range strings.Split(methods, ",") {
		if strings.EqualFold(strings.TrimSpace(method), wanted) {
			return true
		}
	}
	return false
}

func paymentMethodsJSONContainsType(value, wanted string) bool {
	methods, err := operation_setting.ParsePayMethodsJSON(value)
	if err != nil {
		return false
	}
	return paymentMethodsContainType(methods, wanted)
}

func paymentMethodsContainType(methods []map[string]string, wanted string) bool {
	for _, method := range methods {
		if strings.EqualFold(strings.TrimSpace(method["type"]), wanted) {
			return true
		}
	}
	return false
}

func ensurePlatformCurrencyCanBeDeleted(code string) error {
	return ensurePlatformCurrencyCanBeDeletedFromDB(model.DB, code)
}

func ensurePlatformCurrencyCanBeDeletedFromDB(db *gorm.DB, code string) error {
	dependencies := activePaymentCurrencyDependenciesFromDB(db, code)
	if len(dependencies) == 0 {
		return nil
	}
	return fmt.Errorf("currency %s is used by active payment methods: %s", strings.ToUpper(strings.TrimSpace(code)), strings.Join(dependencies, ", "))
}

func GetPlatformCurrencies(c *gin.Context) {
	currencies, err := model.ListPlatformCurrencies(true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	for i := range currencies {
		// Do not expose operational error text to regular users.
		currencies[i].LastSyncError = ""
	}
	common.ApiSuccess(c, currencies)
}

func AdminListPlatformCurrencies(c *gin.Context) {
	currencies, err := model.ListPlatformCurrencies(false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, currencies)
}

func AdminCreatePlatformCurrency(c *gin.Context) {
	var request platformCurrencyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid currency payload")
		return
	}
	currency := model.PlatformCurrency{
		Code: strings.ToUpper(strings.TrimSpace(request.Code)), Name: request.Name, Symbol: request.Symbol,
		SyncProvider: request.SyncProvider, ManualRateToUSD: request.ManualRateToUSD,
		Enabled: true, SyncEnabled: false,
	}
	if request.Enabled != nil {
		currency.Enabled = *request.Enabled
	}
	if request.SyncEnabled != nil {
		currency.SyncEnabled = *request.SyncEnabled
	}
	model.NormalizePlatformCurrency(&currency)
	if currency.Code == "" {
		common.ApiErrorMsg(c, "currency code is required")
		return
	}
	if err := validatePlatformCurrency(&currency); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	syncQuote, syncEnabled, err := preparePlatformCurrencySync(c.Request.Context(), &currency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&currency).Error; err != nil {
			return err
		}
		if syncEnabled {
			return tx.Create(&syncQuote).Error
		}
		return nil
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, currency)
}

func AdminUpdatePlatformCurrency(c *gin.Context) {
	code := strings.ToUpper(strings.TrimSpace(c.Param("code")))
	currency, err := model.GetPlatformCurrency(code)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.Status(http.StatusNotFound)
		return
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var request platformCurrencyRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid currency payload")
		return
	}
	previousSyncEnabled := currency.SyncEnabled
	previousSyncProvider := currency.SyncProvider
	nameChanged := strings.TrimSpace(request.Name) != ""
	symbolChanged := strings.TrimSpace(request.Symbol) != ""
	syncConfigRequested := strings.TrimSpace(request.SyncProvider) != "" || request.SyncEnabled != nil
	manualRateChanged := request.ManualRateToUSD > 0
	if nameChanged {
		currency.Name = request.Name
	}
	if symbolChanged {
		currency.Symbol = request.Symbol
	}
	if strings.TrimSpace(request.SyncProvider) != "" {
		currency.SyncProvider = request.SyncProvider
	}
	if request.Enabled != nil {
		currency.Enabled = *request.Enabled
	}
	if request.SyncEnabled != nil {
		currency.SyncEnabled = *request.SyncEnabled
	}
	if manualRateChanged {
		currency.ManualRateToUSD = request.ManualRateToUSD
	}
	model.NormalizePlatformCurrency(currency)
	syncConfigChanged := previousSyncEnabled != currency.SyncEnabled || previousSyncProvider != currency.SyncProvider
	if syncConfigChanged {
		invalidatePlatformCurrencySyncState(currency)
	} else if !currency.SyncEnabled {
		currency.RateToUSD = currency.ManualRateToUSD
	}
	if err := validatePlatformCurrency(currency); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var syncQuote model.CurrencyExchangeRate
	syncQuoteReady := false
	if syncConfigChanged && currency.SyncEnabled {
		syncQuote, syncQuoteReady, err = preparePlatformCurrencySync(c.Request.Context(), currency)
		if err != nil {
			common.ApiError(c, err)
			return
		}
	}
	updates := map[string]interface{}{}
	if nameChanged {
		updates["name"] = currency.Name
	}
	if symbolChanged {
		updates["symbol"] = currency.Symbol
	}
	if request.Enabled != nil {
		updates["enabled"] = currency.Enabled
	}
	if manualRateChanged {
		updates["manual_rate_to_usd"] = currency.ManualRateToUSD
	}
	if syncConfigRequested {
		updates["sync_enabled"] = currency.SyncEnabled
		updates["sync_provider"] = currency.SyncProvider
	}
	if syncConfigChanged {
		updates["rate_to_usd"] = currency.RateToUSD
		updates["last_sync_at"] = currency.LastSyncAt
		updates["last_sync_error"] = currency.LastSyncError
	} else if !currency.SyncEnabled && (syncConfigRequested || manualRateChanged) {
		updates["rate_to_usd"] = currency.RateToUSD
	}
	var expectedSyncEnabled *bool
	if syncConfigRequested || (!currency.SyncEnabled && manualRateChanged) {
		expectedSyncEnabled = &previousSyncEnabled
	}
	if err := model.UpdatePlatformCurrencySettingsWithTxGuard(currency.Code, updates, expectedSyncEnabled, previousSyncProvider, func(tx *gorm.DB) error {
		if syncQuoteReady {
			return tx.Create(&syncQuote).Error
		}
		return nil
	}); err != nil {
		if errors.Is(err, model.ErrPlatformCurrencySyncConfigChanged) {
			latest, reloadErr := model.GetPlatformCurrency(currency.Code)
			if reloadErr != nil {
				common.ApiError(c, reloadErr)
				return
			}
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "currency synchronization settings changed concurrently; reload and retry", "data": latest})
			return
		}
		common.ApiError(c, err)
		return
	}
	updated, err := model.GetPlatformCurrency(currency.Code)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, updated)
}

func AdminDeletePlatformCurrency(c *gin.Context) {
	if err := model.DeletePlatformCurrencyWithTxGuard(c.Param("code"), func(tx *gorm.DB) error {
		return ensurePlatformCurrencyCanBeDeletedFromDB(tx, c.Param("code"))
	}); err != nil {
		if strings.Contains(err.Error(), "USD is the required") {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

func AdminSyncPlatformCurrency(c *gin.Context) {
	if err := service.SyncPlatformCurrency(c.Request.Context(), c.Param("code")); err != nil {
		common.ApiError(c, err)
		return
	}
	currency, err := model.GetPlatformCurrency(c.Param("code"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, currency)
}

package controller

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/currency_exchange_rate_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var completionRatioMetaOptionKeys = []string{
	"ModelPrice",
	"ModelRatio",
	"CompletionRatio",
	"CacheRatio",
	"CreateCacheRatio",
	"ImageRatio",
	"AudioRatio",
	"AudioCompletionRatio",
}

const maskedOptionValue = "********"

// SaveCreemConfig keeps local requests ordered; the transactional row lock in
// mergeCreemConfigForUpdate also serializes partial updates across replicas.
var creemConfigUpdateMutex sync.Mutex

func isPaymentComplianceOptionKey(key string) bool {
	return strings.HasPrefix(key, "payment_setting.compliance_")
}

func isPositiveOptionValue(value string) bool {
	intValue, err := strconv.Atoi(strings.TrimSpace(value))
	if err == nil {
		return intValue > 0
	}
	floatValue, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return err == nil && floatValue > 0
}

func optionUpdateValue(key, updatingKey, updatingValue, currentValue string) string {
	if key == updatingKey {
		return updatingValue
	}
	return currentValue
}

func optionUpdateBool(key, updatingKey, updatingValue string, currentValue bool) bool {
	value := optionUpdateValue(key, updatingKey, updatingValue, strconv.FormatBool(currentValue))
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func validateEnabledPlatformCurrency(code, paymentName string) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return fmt.Errorf("%s payment currency is required", paymentName)
	}
	if _, err := service.GetPlatformCurrencyRate(code); err != nil {
		return fmt.Errorf("%s payment currency is unavailable: %w", paymentName, err)
	}
	return nil
}

func isWaffoTopUpEnabledAfterOptionUpdate(key, value string, complianceConfirmed bool) bool {
	if !complianceConfirmed ||
		!optionUpdateBool("WaffoEnabled", key, value, setting.WaffoEnabled) {
		return false
	}
	sandbox := optionUpdateBool("WaffoSandbox", key, value, setting.WaffoSandbox)
	if sandbox {
		return strings.TrimSpace(optionUpdateValue("WaffoSandboxApiKey", key, value, setting.WaffoSandboxApiKey)) != "" &&
			strings.TrimSpace(optionUpdateValue("WaffoSandboxPrivateKey", key, value, setting.WaffoSandboxPrivateKey)) != "" &&
			strings.TrimSpace(optionUpdateValue("WaffoSandboxPublicCert", key, value, setting.WaffoSandboxPublicCert)) != ""
	}
	return strings.TrimSpace(optionUpdateValue("WaffoApiKey", key, value, setting.WaffoApiKey)) != "" &&
		strings.TrimSpace(optionUpdateValue("WaffoPrivateKey", key, value, setting.WaffoPrivateKey)) != "" &&
		strings.TrimSpace(optionUpdateValue("WaffoPublicCert", key, value, setting.WaffoPublicCert)) != ""
}

func isCreemTopUpEnabledAfterOptionUpdate(key, value string, complianceConfirmed bool) bool {
	config := setting.GetCreemConfig()
	return isCreemTopUpEnabledWithConfig(
		optionUpdateValue("CreemApiKey", key, value, config.APIKey),
		optionUpdateValue("CreemProducts", key, value, config.Products),
		optionUpdateBool("CreemTestMode", key, value, config.TestMode),
		optionUpdateValue("CreemWebhookSecret", key, value, config.WebhookSecret),
		complianceConfirmed,
	)
}

func isCreemTopUpEnabledWithConfig(apiKey, products string, testMode bool, webhookSecret string, complianceConfirmed bool) bool {
	if !complianceConfirmed || strings.TrimSpace(apiKey) == "" {
		return false
	}
	products = strings.TrimSpace(products)
	return products != "" && products != "[]" && (testMode || strings.TrimSpace(webhookSecret) != "")
}

func isYooKassaTopUpEnabledAfterOptionUpdate(key, value string, complianceConfirmed bool) bool {
	config := setting.GetYooKassaConfig()
	if !complianceConfirmed ||
		!optionUpdateBool("YooKassaEnabled", key, value, config.Enabled) ||
		strings.TrimSpace(optionUpdateValue("YooKassaShopID", key, value, config.ShopID)) == "" ||
		strings.TrimSpace(optionUpdateValue("YooKassaSecretKey", key, value, config.SecretKey)) == "" {
		return false
	}
	methods := optionUpdateValue("YooKassaPaymentMethods", key, value, config.PaymentMethods)
	for _, method := range strings.Split(methods, ",") {
		if strings.EqualFold(strings.TrimSpace(method), "sbp") {
			return true
		}
	}
	return false
}

func isNOWPaymentsTopUpEnabledAfterOptionUpdate(key, value string, complianceConfirmed bool) bool {
	return complianceConfirmed &&
		optionUpdateBool("NOWPaymentsEnabled", key, value, setting.NOWPaymentsEnabled) &&
		strings.TrimSpace(optionUpdateValue("NOWPaymentsAPIKey", key, value, setting.NOWPaymentsAPIKey)) != "" &&
		strings.TrimSpace(optionUpdateValue("NOWPaymentsIPNSecret", key, value, setting.NOWPaymentsIPNSecret)) != "" &&
		strings.TrimSpace(optionUpdateValue("NOWPaymentsIPNCallbackURL", key, value, setting.NOWPaymentsIPNCallbackURL)) != ""
}

func validateDirectUSDTOptionUpdate(key, value string) error {
	enabled := setting.USDTTRC20Enabled
	address := setting.USDTTRC20ReceivingAddress
	apiKey := setting.USDTTRC20APIKey
	switch key {
	case "USDTTRC20Enabled":
		enabled = value == "true"
	case "USDTTRC20ReceivingAddress":
		address = value
	case "USDTTRC20APIKey":
		apiKey = value
	default:
		return nil
	}
	return setting.ValidateDirectUSDTConfigValues(enabled, address, apiKey)
}

func validateCreemProductCurrencies(productsJSON string) error {
	return validateCreemProductCurrenciesFromDB(model.DB, productsJSON)
}

func validateCreemProductCurrenciesFromDB(db *gorm.DB, productsJSON string) error {
	var products []CreemProduct
	if err := common.UnmarshalJsonStr(productsJSON, &products); err != nil {
		return fmt.Errorf("CreemProducts must be valid JSON")
	}
	if products == nil {
		return fmt.Errorf("CreemProducts must be a JSON array")
	}
	for _, product := range products {
		if strings.TrimSpace(product.ProductId) == "" {
			return errors.New("Creem productId is required")
		}
		if product.Price <= 0 {
			return errors.New("Creem product price must be greater than zero")
		}
		if product.Quota <= 0 {
			return errors.New("Creem product quota must be greater than zero")
		}
		if err := validateEnabledPlatformCurrencyFromDB(db, product.Currency, "Creem"); err != nil {
			return err
		}
	}
	return nil
}

func normalizeCreemProducts(productsJSON string) (string, error) {
	var products []CreemProduct
	if err := common.UnmarshalJsonStr(productsJSON, &products); err != nil {
		return "", errors.New("CreemProducts must be valid JSON")
	}
	if products == nil {
		return "", errors.New("CreemProducts must be a JSON array")
	}
	for i := range products {
		if strings.TrimSpace(products[i].ProductId) == "" {
			return "", errors.New("Creem productId is required")
		}
		if products[i].Price <= 0 {
			return "", errors.New("Creem product price must be greater than zero")
		}
		if products[i].Quota <= 0 {
			return "", errors.New("Creem product quota must be greater than zero")
		}
		currency := strings.ToUpper(strings.TrimSpace(products[i].Currency))
		if currency != "USD" && currency != "EUR" {
			return "", errors.New("Creem product currency must be USD or EUR")
		}
		configured, err := model.GetPlatformCurrency(currency)
		if err != nil || !configured.Enabled {
			return "", errors.New("Creem product currency is not enabled on the platform")
		}
		products[i].Currency = currency
	}
	normalized, err := common.Marshal(products)
	if err != nil {
		return "", errors.New("CreemProducts must be valid JSON")
	}
	return string(normalized), nil
}

type saveCreemConfigRequest struct {
	APIKey        *string `json:"api_key"`
	WebhookSecret *string `json:"webhook_secret"`
	TestMode      *bool   `json:"test_mode"`
	Products      *string `json:"products"`
}

// currentCreemConfigForUpdate reads the latest persisted values instead of
// relying solely on the process snapshot. This matters when a previous
// partial update has just committed, or when another instance has replicated
// a config change before this request arrives.
func currentCreemConfigForUpdate() (setting.CreemConfig, error) {
	return currentCreemConfigForUpdateFromDB(model.DB)
}

func currentCreemConfigForUpdateFromDB(db *gorm.DB) (setting.CreemConfig, error) {
	config := setting.GetCreemConfig()
	var options []model.Option
	if err := db.Where("key IN ?", []string{"CreemApiKey", "CreemWebhookSecret", "CreemTestMode", "CreemProducts"}).Find(&options).Error; err != nil {
		return config, err
	}
	for _, option := range options {
		switch option.Key {
		case "CreemApiKey":
			config.APIKey = option.Value
		case "CreemWebhookSecret":
			config.WebhookSecret = option.Value
		case "CreemProducts":
			config.Products = option.Value
		case "CreemTestMode":
			parsed, err := strconv.ParseBool(option.Value)
			if err != nil {
				return config, fmt.Errorf("invalid persisted CreemTestMode: %w", err)
			}
			config.TestMode = parsed
		}
	}
	return config, nil
}

func mergeCreemConfigForUpdate(db *gorm.DB, request *saveCreemConfigRequest, values map[string]string) (setting.CreemConfig, error) {
	if db != nil {
		current := setting.GetCreemConfig()
		defaults := map[string]string{
			"CreemApiKey":        current.APIKey,
			"CreemWebhookSecret": current.WebhookSecret,
			"CreemTestMode":      strconv.FormatBool(current.TestMode),
			"CreemProducts":      current.Products,
		}
		for _, key := range []string{"CreemApiKey", "CreemWebhookSecret", "CreemTestMode", "CreemProducts"} {
			option := model.Option{Key: key, Value: defaults[key]}
			if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&option).Error; err != nil {
				return setting.CreemConfig{}, err
			}
			if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&option, "key = ?", key).Error; err != nil {
				return setting.CreemConfig{}, err
			}
		}
	}
	dbForRead := model.DB
	if db != nil {
		dbForRead = db
	}
	config, err := currentCreemConfigForUpdateFromDB(dbForRead)
	if err != nil {
		return config, err
	}
	if request == nil {
		return config, nil
	}
	if request.APIKey != nil {
		config.APIKey = strings.TrimSpace(*request.APIKey)
	}
	if request.WebhookSecret != nil {
		config.WebhookSecret = strings.TrimSpace(*request.WebhookSecret)
	}
	if request.TestMode != nil {
		config.TestMode = *request.TestMode
	}
	if request.Products != nil {
		normalized, err := normalizeCreemProducts(*request.Products)
		if err != nil {
			return config, err
		}
		config.Products = normalized
	} else if strings.TrimSpace(config.Products) != "" && strings.TrimSpace(config.Products) != "[]" {
		normalized, err := normalizeCreemProducts(config.Products)
		if err != nil {
			return config, err
		}
		config.Products = normalized
	}
	if values != nil {
		values["CreemApiKey"] = config.APIKey
		values["CreemWebhookSecret"] = config.WebhookSecret
		values["CreemTestMode"] = strconv.FormatBool(config.TestMode)
		values["CreemProducts"] = config.Products
	}
	return config, nil
}

func SaveCreemConfig(c *gin.Context) {
	// Phase 1 retirement: keep the stored credentials read-only so verified
	// webhooks can settle existing pending orders, but never allow configuring
	// a provider that can no longer create a checkout.
	common.ApiErrorMsg(c, "Creem payments are retired and its configuration is read-only")
}

type savePaymentSettingsRequest struct {
	Options []OptionUpdateRequest   `json:"options"`
	Creem   *saveCreemConfigRequest `json:"creem"`
}

func optionUpdateString(value any) (string, error) {
	switch value := value.(type) {
	case string:
		return value, nil
	case bool:
		return strconv.FormatBool(value), nil
	case float64:
		return common.Interface2String(value), nil
	case int:
		return common.Interface2String(value), nil
	default:
		return "", errors.New("payment option value must be a scalar")
	}
}

func preparePaymentOptionUpdate(option OptionUpdateRequest) (string, string, error) {
	value, err := optionUpdateString(option.Value)
	if err != nil {
		return "", "", err
	}
	if isCreemConfigOption(option.Key) || isPaymentComplianceOptionKey(option.Key) {
		return "", "", errors.New("Creem and compliance settings must be updated through their dedicated atomic APIs")
	}
	switch option.Key {
	case "NOWPaymentsPriceCurrency", "NOWPaymentsPayCurrency":
		value = "usdt"
	case "PayMethods":
		methods, err := operation_setting.ParsePayMethodsJSON(value)
		if err != nil {
			return "", "", errors.New("PayMethods must be valid JSON")
		}
		if err := operation_setting.ValidatePayMethods(methods); err != nil {
			return "", "", err
		}
		operation_setting.NormalizePayMethods(methods)
		// Creem checkout is retired. Drop a stale catalog entry on the next
		// payment-settings save instead of allowing it to be re-published.
		filteredMethods := make([]map[string]string, 0, len(methods))
		for _, method := range methods {
			if method == nil || strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodCreem) {
				continue
			}
			filteredMethods = append(filteredMethods, method)
		}
		methods = filteredMethods
		for _, method := range methods {
			if method == nil {
				continue
			}
			currency := strings.TrimSpace(method["currency"])
			if strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodNOWPayments) {
				currency = "USDT"
			}
			if currency == "" {
				continue
			}
			if err := service.ValidatePaymentMethodCurrency(method["type"], currency); err != nil {
				return "", "", err
			}
			if !strings.EqualFold(currency, "USD") {
				configured, err := model.GetPlatformCurrency(currency)
				if err != nil || !configured.Enabled {
					return "", "", errors.New("payment currency is not enabled on the platform")
				}
			}
		}
		encoded, err := common.Marshal(methods)
		if err != nil {
			return "", "", errors.New("PayMethods must be valid JSON")
		}
		value = string(encoded)
	case "YooKassaSecretKey", "NOWPaymentsAPIKey", "NOWPaymentsIPNSecret", "TelegramBotToken", "USDTTRC20APIKey", "USDTTONAPIKey", "USDTSolanaAPIKey":
		if value == maskedOptionValue {
			return "", "", nil
		}
	}
	return option.Key, value, nil
}

func creemConfigForAtomicSave(request *saveCreemConfigRequest) (setting.CreemConfig, map[string]string, error) {
	config, err := currentCreemConfigForUpdate()
	if err != nil {
		return config, nil, err
	}
	values := make(map[string]string, 4)
	if request == nil {
		return config, values, nil
	}
	if request.APIKey == nil && request.WebhookSecret == nil && request.TestMode == nil && request.Products == nil {
		return config, values, nil
	}
	if request.APIKey != nil {
		config.APIKey = strings.TrimSpace(*request.APIKey)
	}
	if request.WebhookSecret != nil {
		config.WebhookSecret = strings.TrimSpace(*request.WebhookSecret)
	}
	if request.TestMode != nil {
		config.TestMode = *request.TestMode
	}
	if request.Products != nil {
		normalized, err := normalizeCreemProducts(*request.Products)
		if err != nil {
			return config, nil, err
		}
		config.Products = normalized
	} else if strings.TrimSpace(config.Products) != "" && strings.TrimSpace(config.Products) != "[]" {
		normalized, err := normalizeCreemProducts(config.Products)
		if err != nil {
			return config, nil, err
		}
		config.Products = normalized
	}
	values["CreemApiKey"] = config.APIKey
	values["CreemWebhookSecret"] = config.WebhookSecret
	values["CreemTestMode"] = strconv.FormatBool(config.TestMode)
	values["CreemProducts"] = config.Products
	return config, values, nil
}

func prospectiveOptionValue(values map[string]string, key, current string) string {
	if value, ok := values[key]; ok {
		return value
	}
	return current
}

func prospectiveOptionBool(values map[string]string, key string, current bool) bool {
	value := prospectiveOptionValue(values, key, strconv.FormatBool(current))
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}

func prospectiveYooKassaSBPEnabledFromDB(db *gorm.DB, values map[string]string) bool {
	payMethods := prospectiveOptionValue(values, "PayMethods", latestPaymentOptionFromDB(db, "PayMethods", operation_setting.PayMethods2JsonString()))
	return paymentMethodsJSONContainsType(payMethods, model.PaymentMethodYooKassaSBP)
}

func validatePaymentSettingsReadiness(values map[string]string) error {
	return validatePaymentSettingsReadinessFromDB(model.DB, values)
}

func validatePaymentSettingsReadinessFromDB(db *gorm.DB, values map[string]string) error {
	if err := validateManualTopupSettingsFromDB(db, values); err != nil {
		return err
	}
	complianceConfirmed := latestPaymentOptionBoolFromDB(db, "payment_setting.compliance_confirmed", isPaymentComplianceConfirmed())
	waffoEnabled := prospectiveOptionBool(values, "WaffoEnabled", latestPaymentOptionBoolFromDB(db, "WaffoEnabled", setting.WaffoEnabled))
	if waffoEnabled && complianceConfirmed {
		currency := prospectiveOptionValue(values, "WaffoCurrency", latestPaymentOptionFromDB(db, "WaffoCurrency", setting.WaffoCurrency))
		if err := validateEnabledPlatformCurrencyFromDB(db, currency, "Waffo"); err != nil {
			return err
		}
	}
	yooKassaConfig := setting.GetYooKassaConfig()
	if prospectiveOptionBool(values, "YooKassaEnabled", latestPaymentOptionBoolFromDB(db, "YooKassaEnabled", yooKassaConfig.Enabled)) && complianceConfirmed && prospectiveYooKassaSBPEnabledFromDB(db, values) {
		if err := validateEnabledPlatformCurrencyFromDB(db, "RUB", "YooKassa"); err != nil {
			return err
		}
	}
	if prospectiveOptionBool(values, "NOWPaymentsEnabled", latestPaymentOptionBoolFromDB(db, "NOWPaymentsEnabled", setting.NOWPaymentsEnabled)) && complianceConfirmed {
		if err := validateEnabledPlatformCurrencyFromDB(db, "USDT", "NOWPayments"); err != nil {
			return err
		}
	}
	return nil
}

func validateManualTopupSettingsFromDB(db *gorm.DB, values map[string]string) error {
	settings := operation_setting.GetPaymentSetting()
	enabled := prospectiveOptionBool(
		values,
		"payment_setting.manual_topup_enabled",
		latestPaymentOptionBoolFromDB(
			db,
			"payment_setting.manual_topup_enabled",
			settings.ManualTopupEnabled,
		),
	)
	if !enabled {
		return nil
	}

	contactURL := strings.TrimSpace(prospectiveOptionValue(
		values,
		"payment_setting.manual_topup_contact_url",
		latestPaymentOptionFromDB(
			db,
			"payment_setting.manual_topup_contact_url",
			settings.ManualTopupContactURL,
		),
	))
	parsedURL, err := url.ParseRequestURI(contactURL)
	if err != nil || parsedURL.Scheme != "https" || !strings.EqualFold(parsedURL.Host, "t.me") || parsedURL.Path == "" || parsedURL.Path == "/" {
		return errors.New("manual large payment contact URL must be a Telegram link starting with https://t.me/")
	}

	minimumText := prospectiveOptionValue(
		values,
		"payment_setting.manual_topup_min_amount",
		latestPaymentOptionFromDB(
			db,
			"payment_setting.manual_topup_min_amount",
			strconv.FormatFloat(settings.ManualTopupMinAmount, 'f', -1, 64),
		),
	)
	minimum, err := strconv.ParseFloat(strings.TrimSpace(minimumText), 64)
	if err != nil || math.IsNaN(minimum) || math.IsInf(minimum, 0) || minimum <= 0 {
		return errors.New("manual large payment minimum must be greater than zero")
	}
	return nil
}

func validateEnabledPlatformCurrencyFromDB(db *gorm.DB, code, paymentName string) error {
	if db == nil {
		return validateEnabledPlatformCurrency(code, paymentName)
	}
	var currency model.PlatformCurrency
	if err := db.Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&currency).Error; err != nil {
		return fmt.Errorf("%s payment currency is unavailable: %w", paymentName, err)
	}
	if !currency.Enabled {
		return fmt.Errorf("%s payment currency is unavailable: currency %s is disabled", paymentName, strings.ToUpper(strings.TrimSpace(code)))
	}
	if currency.SyncEnabled {
		if currency.RateToUSD <= 0 || currency.LastSyncAt == nil || time.Since(*currency.LastSyncAt) > 48*time.Hour {
			return fmt.Errorf("%s payment currency is unavailable: currency %s has no synchronized USD rate", paymentName, strings.ToUpper(strings.TrimSpace(code)))
		}
	} else if currency.ManualRateToUSD <= 0 && currency.RateToUSD <= 0 {
		return fmt.Errorf("%s payment currency is unavailable: currency %s has no USD rate", paymentName, strings.ToUpper(strings.TrimSpace(code)))
	}
	return nil
}

func isCreemTopUpEnabledWithConfigAndCompliance(config setting.CreemConfig, complianceConfirmed bool) bool {
	products := strings.TrimSpace(config.Products)
	return complianceConfirmed && strings.TrimSpace(config.APIKey) != "" && products != "" && products != "[]" && (config.TestMode || strings.TrimSpace(config.WebhookSecret) != "")
}

// SavePaymentSettings commits generic payment options and a partial Creem
// configuration in one transaction. Existing single-option endpoints remain
// available for backwards compatibility.
func SavePaymentSettings(c *gin.Context) {
	creemConfigUpdateMutex.Lock()
	defer creemConfigUpdateMutex.Unlock()

	var request savePaymentSettingsRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "invalid payment settings payload")
		return
	}
	if request.Creem != nil {
		common.ApiErrorMsg(c, "Creem payments are retired and its configuration is read-only")
		return
	}
	if len(request.Options) == 0 && request.Creem == nil {
		common.ApiErrorMsg(c, "no payment settings changes")
		return
	}
	values := make(map[string]string, len(request.Options)+4)
	for _, option := range request.Options {
		key, value, err := preparePaymentOptionUpdate(option)
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		if key != "" {
			values[key] = value
		}
	}
	if len(values) == 0 {
		common.ApiErrorMsg(c, "no payment settings changes")
		return
	}
	if err := validatePaymentSettingsReadiness(values); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if err := model.UpdateOptionsBulkWithPaymentCurrencyGuardAndPrepareTx(values, nil, func(tx *gorm.DB) error {
		return validatePaymentSettingsReadinessFromDB(tx, values)
	}); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

// validatePaymentCurrencyReadiness prevents a generic option update from
// activating a checkout path whose settlement currency is absent or disabled.
// Currency management has the inverse guard; this covers the order where an
// operator configures the catalog/credentials first and enables a provider
// only afterwards.
func validatePaymentCurrencyReadiness(key, value string) error {
	return validatePaymentCurrencyReadinessFromDB(model.DB, key, value)
}

func validatePaymentCurrencyReadinessFromDB(db *gorm.DB, key, value string) error {
	values := map[string]string{key: value}
	if key == "WaffoCurrency" {
		return validateEnabledPlatformCurrencyFromDB(db, value, "Waffo")
	}
	return validatePaymentSettingsReadinessFromDB(db, values)
}

func isPaymentCurrencyGuardOption(key, value string) bool {
	if strings.HasPrefix(key, "payment_setting.manual_topup_") {
		return true
	}
	if strings.HasPrefix(key, "Waffo") ||
		strings.HasPrefix(key, "YooKassa") ||
		strings.HasPrefix(key, "NOWPayments") {
		return true
	}
	if key != "PayMethods" {
		return false
	}
	value = strings.ToLower(value)
	return strings.Contains(value, "nowpayments") || strings.Contains(value, "yookassa") ||
		strings.Contains(value, "waffo") || strings.Contains(value, "currency")
}

// validatePaymentCurrencyReadinessForComplianceConfirmation uses the same
// checkout contracts as option updates, but evaluates them as if compliance
// were already confirmed. Without this, credentials/catalogs configured while
// checkout is gated could bypass currency validation at activation time.
func validatePaymentCurrencyReadinessForComplianceConfirmation() error {
	return validatePaymentCurrencyReadinessForComplianceConfirmationFromDB(model.DB)
}

func validatePaymentCurrencyReadinessForComplianceConfirmationFromDB(db *gorm.DB) error {
	waffoEnabled := latestPaymentOptionBoolFromDB(db, "WaffoEnabled", setting.WaffoEnabled)
	waffoSandbox := latestPaymentOptionBoolFromDB(db, "WaffoSandbox", setting.WaffoSandbox)
	waffoReady := waffoEnabled
	if waffoSandbox {
		waffoReady = waffoReady && latestPaymentOptionFromDB(db, "WaffoSandboxApiKey", setting.WaffoSandboxApiKey) != "" && latestPaymentOptionFromDB(db, "WaffoSandboxPrivateKey", setting.WaffoSandboxPrivateKey) != "" && latestPaymentOptionFromDB(db, "WaffoSandboxPublicCert", setting.WaffoSandboxPublicCert) != ""
	} else {
		waffoReady = waffoReady && latestPaymentOptionFromDB(db, "WaffoApiKey", setting.WaffoApiKey) != "" && latestPaymentOptionFromDB(db, "WaffoPrivateKey", setting.WaffoPrivateKey) != "" && latestPaymentOptionFromDB(db, "WaffoPublicCert", setting.WaffoPublicCert) != ""
	}
	if waffoReady {
		currency := latestPaymentOptionFromDB(db, "WaffoCurrency", setting.WaffoCurrency)
		if err := validateEnabledPlatformCurrencyFromDB(db, currency, "Waffo"); err != nil {
			return err
		}
	}
	yooKassaConfig := setting.GetYooKassaConfig()
	if latestPaymentOptionBoolFromDB(db, "YooKassaEnabled", yooKassaConfig.Enabled) && latestPaymentOptionFromDB(db, "YooKassaShopID", yooKassaConfig.ShopID) != "" && latestPaymentOptionFromDB(db, "YooKassaSecretKey", yooKassaConfig.SecretKey) != "" && paymentMethodsJSONContainsType(latestPaymentOptionFromDB(db, "PayMethods", operation_setting.PayMethods2JsonString()), model.PaymentMethodYooKassaSBP) {
		if err := validateEnabledPlatformCurrencyFromDB(db, "RUB", "YooKassa"); err != nil {
			return err
		}
	}
	if latestPaymentOptionBoolFromDB(db, "NOWPaymentsEnabled", setting.NOWPaymentsEnabled) && latestPaymentOptionFromDB(db, "NOWPaymentsAPIKey", setting.NOWPaymentsAPIKey) != "" && latestPaymentOptionFromDB(db, "NOWPaymentsIPNSecret", setting.NOWPaymentsIPNSecret) != "" && latestPaymentOptionFromDB(db, "NOWPaymentsIPNCallbackURL", setting.NOWPaymentsIPNCallbackURL) != "" {
		if err := validateEnabledPlatformCurrencyFromDB(db, "USDT", "NOWPayments"); err != nil {
			return err
		}
	}
	return nil
}

func collectModelNamesFromOptionValue(raw string, modelNames map[string]struct{}) {
	if strings.TrimSpace(raw) == "" {
		return
	}

	var parsed map[string]any
	if err := common.UnmarshalJsonStr(raw, &parsed); err != nil {
		return
	}

	for modelName := range parsed {
		modelNames[modelName] = struct{}{}
	}
}

func buildCompletionRatioMetaValue(optionValues map[string]string) string {
	modelNames := make(map[string]struct{})
	for _, key := range completionRatioMetaOptionKeys {
		collectModelNamesFromOptionValue(optionValues[key], modelNames)
	}

	meta := make(map[string]ratio_setting.CompletionRatioInfo, len(modelNames))
	for modelName := range modelNames {
		meta[modelName] = ratio_setting.GetCompletionRatioInfo(modelName)
	}

	jsonBytes, err := common.Marshal(meta)
	if err != nil {
		return "{}"
	}
	return string(jsonBytes)
}

func GetOptions(c *gin.Context) {
	var options []*model.Option
	optionValues := make(map[string]string)
	model.RefreshModelRequestRateLimitDurationMetadata()
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		value := common.Interface2String(v)
		isSensitiveKey := strings.HasSuffix(k, "Token") ||
			strings.HasSuffix(k, "Secret") ||
			strings.HasSuffix(k, "Key") ||
			strings.HasSuffix(k, "secret") ||
			strings.HasSuffix(k, "api_key")
		if isSensitiveKey {
			if k == "YooKassaSecretKey" || k == "NOWPaymentsAPIKey" || k == "NOWPaymentsIPNSecret" || k == "TelegramBotToken" || k == "USDTTRC20APIKey" || k == "USDTTONAPIKey" || k == "USDTSolanaAPIKey" {
				if value != "" {
					value = maskedOptionValue
				}
				options = append(options, &model.Option{
					Key:   k,
					Value: value,
				})
			}
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: value,
		})
		for _, optionKey := range completionRatioMetaOptionKeys {
			if optionKey == k {
				optionValues[k] = value
				break
			}
		}
	}
	common.OptionMapRWMutex.Unlock()
	options = append(options, &model.Option{
		Key:   "CompletionRatioMeta",
		Value: buildCompletionRatioMetaValue(optionValues),
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
}

type OptionUpdateRequest struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

func isCreemConfigOption(key string) bool {
	switch key {
	case "CreemApiKey", "CreemProducts", "CreemTestMode", "CreemWebhookSecret":
		return true
	default:
		return false
	}
}

func UpdateOption(c *gin.Context) {
	var option OptionUpdateRequest
	err := common.DecodeJson(c.Request.Body, &option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	switch option.Value.(type) {
	case bool:
		option.Value = common.Interface2String(option.Value.(bool))
	case float64:
		option.Value = common.Interface2String(option.Value.(float64))
	case int:
		option.Value = common.Interface2String(option.Value.(int))
	default:
		option.Value = fmt.Sprintf("%v", option.Value)
	}
	if isCreemConfigOption(option.Key) {
		common.ApiErrorMsg(c, "Creem payments are retired and its configuration is read-only")
		return
	}
	switch option.Key {
	case "NOWPaymentsPriceCurrency", "NOWPaymentsPayCurrency":
		option.Value = "usdt"
	case "CreemProducts":
		normalized, err := normalizeCreemProducts(option.Value.(string))
		if err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		option.Value = normalized
	case "PayMethods":
		methods, err := operation_setting.ParsePayMethodsJSON(option.Value.(string))
		if err != nil {
			common.ApiErrorMsg(c, "PayMethods must be valid JSON")
			return
		}
		if err := operation_setting.ValidatePayMethods(methods); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
		// Normalize gateway-owned currencies before validation and persistence.
		// This keeps legacy/stale JSON (for example Stripe/RUB or YooKassa/USD)
		// aligned with the provider contract instead of rejecting a repairable
		// configuration.
		operation_setting.NormalizePayMethods(methods)
		filteredMethods := make([]map[string]string, 0, len(methods))
		for _, method := range methods {
			if method == nil || strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodCreem) {
				continue
			}
			filteredMethods = append(filteredMethods, method)
		}
		methods = filteredMethods
		for _, method := range methods {
			if method == nil {
				continue
			}
			currency := strings.TrimSpace(method["currency"])
			if currency == "" {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodNOWPayments) {
				currency = "USDT"
			}
			if err := service.ValidatePaymentMethodCurrency(method["type"], currency); err != nil {
				common.ApiErrorMsg(c, err.Error())
				return
			}
			if !strings.EqualFold(currency, "USD") {
				configured, err := model.GetPlatformCurrency(currency)
				if err != nil {
					common.ApiErrorMsg(c, "payment currency is not configured on the platform")
					return
				}
				if !configured.Enabled {
					common.ApiErrorMsg(c, "payment currency is disabled on the platform")
					return
				}
			}
		}
		normalized, err := common.Marshal(methods)
		if err != nil {
			common.ApiErrorMsg(c, "PayMethods must be valid JSON")
			return
		}
		option.Value = string(normalized)
	case "YooKassaSecretKey", "NOWPaymentsAPIKey", "NOWPaymentsIPNSecret", "TelegramBotToken", "USDTTRC20APIKey", "USDTTONAPIKey", "USDTSolanaAPIKey":
		if option.Value == maskedOptionValue {
			c.JSON(http.StatusOK, gin.H{
				"success": true,
				"message": "",
			})
			return
		}
	case "QuotaForInviter", "QuotaForInvitee", "ReferralDepositPercent", "ReferralCashbackPercent":
		if isPositiveOptionValue(option.Value.(string)) && !operation_setting.IsPaymentComplianceConfirmed() {
			common.ApiErrorI18n(c, i18n.MsgPaymentComplianceRequired)
			return
		}
	default:
		if isPaymentComplianceOptionKey(option.Key) {
			common.ApiErrorMsg(c, "合规确认字段不允许通过通用设置接口修改")
			return
		}
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client Id 以及 GitHub Client Secret！",
			})
			return
		}
	case "discord.enabled":
		if option.Value == "true" && system_setting.GetDiscordSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Discord OAuth，请先填入 Discord Client Id 以及 Discord Client Secret！",
			})
			return
		}
	case "oidc.enabled":
		if option.Value == "true" && system_setting.GetOIDCSettings().ClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 OIDC 登录，请先填入 OIDC Client Id 以及 OIDC Client Secret！",
			})
			return
		}
	case "LinuxDOOAuthEnabled":
		if option.Value == "true" && common.LinuxDOClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 LinuxDO OAuth，请先填入 LinuxDO Client Id 以及 LinuxDO Client Secret！",
			})
			return
		}
	case "EmailDomainRestrictionEnabled":
		if option.Value == "true" && len(common.EmailDomainWhitelist) == 0 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用邮箱域名限制，请先填入限制的邮箱域名！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})

			return
		}
	case "TelegramOAuthEnabled":
		if option.Value == "true" && (common.TelegramBotToken == "" || common.TelegramBotName == "") {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Telegram OAuth，请先填入 Telegram Bot Token 和 Bot Name！",
			})
			return
		}
	case "GroupRatio":
		err = ratio_setting.CheckGroupRatio(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "ModelRequestRateLimitGroup":
		err = setting.CheckModelRequestRateLimitGroup(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "currency_exchange_rate.provider":
		err = currency_exchange_rate_setting.ValidateProvider(option.Value.(string))
		if err != nil {
			common.ApiError(c, err)
			return
		}
	case "currency_exchange_rate.update_interval":
		err = currency_exchange_rate_setting.ValidateUpdateInterval(option.Value.(string))
		if err != nil {
			common.ApiError(c, err)
			return
		}
	case "AutomaticDisableStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "AutomaticRetryStatusCodes":
		_, err = operation_setting.ParseHTTPStatusCodeRanges(option.Value.(string))
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.api_info":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "ApiInfo")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.announcements":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "Announcements")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	case "console_setting.faq":
		err = console_setting.ValidateConsoleSettings(option.Value.(string), "FAQ")
		if err != nil {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": err.Error(),
			})
			return
		}
	}
	if strings.HasPrefix(option.Key, "USDTTRC20") {
		if err := validateDirectUSDTOptionUpdate(option.Key, option.Value.(string)); err != nil {
			common.ApiErrorMsg(c, err.Error())
			return
		}
	}
	if err := validatePaymentCurrencyReadiness(option.Key, option.Value.(string)); err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	if model.IsModelPricingOption(option.Key) {
		err = model.UpdatePricingOptionManual(option.Key, option.Value.(string))
	} else if isPaymentCurrencyGuardOption(option.Key, option.Value.(string)) {
		err = model.UpdateOptionWithPaymentCurrencyTxGuard(option.Key, option.Value.(string), func(tx *gorm.DB) error {
			return validatePaymentCurrencyReadinessFromDB(tx, option.Key, option.Value.(string))
		})
	} else {
		err = model.UpdateOption(option.Key, option.Value.(string))
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 出于安全考虑只记录被修改的配置项名称，不记录配置值（可能含密钥等敏感信息）。
	recordManageAudit(c, "option.update", map[string]interface{}{
		"key": option.Key,
	})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
}

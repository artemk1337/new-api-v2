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
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func isDirectUSDTPaymentMethod(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case model.DirectCryptoProvider, model.DirectUSDTTRC20Provider, operation_setting.DirectUSDTTONPaymentMethod, operation_setting.DirectUSDTSolanaPaymentMethod:
		return true
	default:
		return false
	}
}

func topUpPayMethods() ([]map[string]string, error) {
	fallback := operation_setting.PayMethodsSnapshot()
	methods, err := model.GetPayMethodsFromDB(model.DB)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fallback, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load PayMethods: %w", err)
	}
	return operation_setting.CanonicalizePayMethods(methods), nil
}

// directCryptoMethodForUser is stricter than the generic historical gateway
// guard: Crypto has no legacy public fallback. New invoice creation, quote and
// status all require the single parent method to be present and visible.
func directCryptoMethodForUser(c *gin.Context) (map[string]string, bool) {
	methods, err := topUpPayMethods()
	if err != nil {
		common.ApiError(c, err)
		return nil, false
	}
	for _, method := range methods {
		if method == nil || !strings.EqualFold(strings.TrimSpace(method["type"]), model.DirectCryptoProvider) {
			continue
		}
		if operation_setting.IsPayMethodAdminOnly(method) && c.GetInt("role") < common.RoleAdminUser {
			common.ApiErrorMsg(c, "Payment method is not available")
			return nil, false
		}
		return method, true
	}
	common.ApiErrorMsg(c, "USDT payments are not available")
	return nil, false
}

func paymentMethodAllowedForUser(c *gin.Context, paymentMethod string) bool {
	methods, err := topUpPayMethods()
	if err != nil {
		common.ApiError(c, err)
		return false
	}
	for _, method := range methods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), strings.TrimSpace(paymentMethod)) {
			if operation_setting.IsPayMethodAdminOnly(method) && c.GetInt("role") < common.RoleAdminUser {
				common.ApiErrorMsg(c, "Payment method is not available")
				return false
			}
			return true
		}
	}
	return true
}

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()
	// Keep all Creem fields in this response derived from one immutable
	// configuration snapshot. GetCreemConfig is atomic, but reading it once for
	// readiness and again for products could otherwise mix two published
	// configurations during an admin update.
	creemConfig := setting.GetCreemConfig()
	enableCreem, creemProducts := creemTopUpInfoForConfig(creemConfig)
	yooKassaConfig := setting.GetYooKassaConfig()

	// 获取支付方式
	payMethods, payMethodsErr := topUpPayMethods()
	if payMethodsErr != nil {
		common.ApiError(c, payMethodsErr)
		return
	}
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}
	directNetworks := []string(nil)
	var directConfig map[string]string
	for _, method := range payMethods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), model.DirectCryptoProvider) {
			directConfig = method
			break
		}
	}
	if complianceConfirmed && directConfig != nil {
		directNetworks = model.DirectUSDTReadyNetworks()
	}
	// Remove the parent before adding one canonical method below. This keeps
	// malformed/duplicate catalog data from publishing multiple Crypto cards.
	filteredPayMethods := make([]map[string]string, 0, len(payMethods))
	for _, method := range payMethods {
		if method == nil || !strings.EqualFold(strings.TrimSpace(method["type"]), model.DirectCryptoProvider) {
			filteredPayMethods = append(filteredPayMethods, method)
		}
	}
	payMethods = filteredPayMethods
	hasYooKassaSBP := false
	for _, method := range payMethods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodYooKassaSBP) {
			hasYooKassaSBP = true
			break
		}
	}
	enableYooKassa := isYooKassaTopUpEnabledForConfig(yooKassaConfig) && isYooKassaPaymentMethodEnabledForConfig(yooKassaConfig, "sbp") && hasYooKassaSBP
	if !enableYooKassa {
		filtered := make([]map[string]string, 0, len(payMethods))
		for _, method := range payMethods {
			if method == nil || !strings.EqualFold(strings.TrimSpace(method["type"]), model.PaymentMethodYooKassaSBP) {
				filtered = append(filtered, method)
			}
		}
		payMethods = filtered
	}

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if isStripeTopUpEnabled() {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"currency":  "USD",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.FormatFloat(setting.StripeMinTopUp, 'f', -1, 64),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// Waffo Pancake displayed above the legacy Waffo gateway.
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		hasWaffoPancake := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffoPancake {
				hasWaffoPancake = true
				break
			}
		}

		if !hasWaffoPancake {
			payMethods = append(payMethods, map[string]string{
				"name":      "Waffo Pancake",
				"type":      model.PaymentMethodWaffoPancake,
				"currency":  "USD",
				"color":     "rgba(var(--semi-orange-5), 1)",
				"min_topup": strconv.FormatFloat(setting.WaffoPancakeMinTopUp, 'f', -1, 64),
			})
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodWaffo {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      model.PaymentMethodWaffo,
				"currency":  strings.ToUpper(strings.TrimSpace(setting.WaffoCurrency)),
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.FormatFloat(setting.WaffoMinTopUp, 'f', -1, 64),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	enableNOWPayments := isNOWPaymentsTopUpEnabled()
	if enableNOWPayments {
		hasNOWPayments := false
		for _, method := range payMethods {
			if method["type"] == model.PaymentMethodNOWPayments {
				hasNOWPayments = true
				break
			}
		}
		if !hasNOWPayments {
			payMethods = append(payMethods, map[string]string{"name": "Crypto / NOWPayments", "type": model.PaymentMethodNOWPayments, "currency": "USDT", "color": "#F7931A", "min_topup": strconv.FormatFloat(getMinTopup(), 'f', -1, 64)})
		}
	}

	// Crypto is one parent method. Networks are a server-derived list, never
	// independent PayMethods entries, so their policy cannot diverge.
	if len(directNetworks) > 0 && directConfig != nil {
		directMethod := map[string]string{
			"name":      "Crypto",
			"type":      model.DirectCryptoProvider,
			"currency":  "USDT",
			"color":     "#26A17B",
			"min_topup": "10",
		}
		for key, value := range directConfig {
			directMethod[key] = value
		}
		directMethod["name"] = "Crypto"
		directMethod["type"] = model.DirectCryptoProvider
		directMethod["currency"] = "USDT"
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(directMethod["min_topup"]), 64); err != nil || parsed < 10 {
			directMethod["min_topup"] = "10"
		}
		payMethods = append(payMethods, directMethod)
	}
	// Synthetic provider entries inherit the persisted visibility flag. This
	// prevents an admin-only Stripe/NOWPayments method from being re-added as a
	// public synthetic card when its integration is enabled.
	for i, method := range payMethods {
		if method == nil || operation_setting.IsPayMethodAdminOnly(method) {
			continue
		}
		for _, configured := range operation_setting.PayMethodsSnapshot() {
			if configured != nil && strings.EqualFold(strings.TrimSpace(configured["type"]), strings.TrimSpace(method["type"])) && operation_setting.IsPayMethodAdminOnly(configured) {
				copyMethod := make(map[string]string, len(method)+1)
				for key, value := range method {
					copyMethod[key] = value
				}
				copyMethod["admin_only"] = "true"
				payMethods[i] = copyMethod
				break
			}
		}
	}
	payMethods = operation_setting.FilterPayMethodsForRole(payMethods, c.GetInt("role") >= common.RoleAdminUser)
	directVisible := false
	for _, method := range payMethods {
		if method != nil && strings.EqualFold(strings.TrimSpace(method["type"]), model.DirectCryptoProvider) {
			directVisible = true
			break
		}
	}
	if !directVisible {
		directNetworks = nil
	}
	userGroup, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		userGroup = "default"
	}
	minimumTopUp := 0.0
	for i, method := range payMethods {
		copyMethod := make(map[string]string, len(method)+1)
		for key, value := range method {
			copyMethod[key] = value
		}
		copyMethod["currency"] = getPaymentMethodCurrency(copyMethod)
		ratio := common.GetTopupGroupRatio(getPaymentTopupGroupFromMethods(payMethods, copyMethod["type"], userGroup))
		copyMethod["topup_ratio"] = strconv.FormatFloat(ratio, 'f', -1, 64)
		// Preload the amount-independent quote inputs with the wallet page. The
		// browser can render provider amounts immediately from these values; the
		// payment endpoints still rebuild a fresh server quote on submit.
		displayConfig, configErr := service.GetPaymentQuoteDisplayConfigWithPayMethods(copyMethod["type"], userGroup, payMethods)
		if configErr == nil {
			copyMethod["currency"] = displayConfig.Currency
			copyMethod["rate_to_usd"] = strconv.FormatFloat(displayConfig.RateToUSD, 'f', -1, 64)
			copyMethod["base_amount_multiplier"] = strconv.FormatFloat(displayConfig.BaseAmountMultiplier, 'f', -1, 64)
			copyMethod["topup_ratio"] = strconv.FormatFloat(displayConfig.Coefficient, 'f', -1, 64)
			copyMethod["rounding_decimals"] = strconv.Itoa(displayConfig.RoundingDecimals)
		} else {
			// Keep the method visible when a synchronized rate is unavailable;
			// the UI can show a placeholder and payment creation will return the
			// authoritative rate/configuration error.
			copyMethod["rate_to_usd"] = "0"
		}
		if minimum, configured := service.PaymentMethodMinimumWithMethods(copyMethod["type"], payMethods); configured {
			copyMethod["min_topup"] = strconv.FormatFloat(minimum, 'f', -1, 64)
			// Convert the provider-denominated minimum into the amount the
			// wallet sends to the quote endpoint. This keeps the shared form
			// minimum meaningful when visible methods use different currencies.
			denominator := displayConfig.BaseAmountMultiplier * displayConfig.Coefficient * displayConfig.RateToUSD
			if configErr == nil && denominator > 0 && !math.IsNaN(denominator) && !math.IsInf(denominator, 0) {
				inputMinimum := minimum / denominator
				if inputMinimum > 0 && (minimumTopUp == 0 || inputMinimum < minimumTopUp) {
					minimumTopUp = inputMinimum
				}
			}
		}
		payMethods[i] = copyMethod
	}
	waffoPayMethods := make([]map[string]interface{}, 0)
	if enableWaffo {
		configuredMethods := setting.GetWaffoPayMethods()
		waffoPayMethods = make([]map[string]interface{}, 0, len(configuredMethods))
		for _, method := range configuredMethods {
			publicMethod := map[string]interface{}{
				"name":          method.Name,
				"icon":          method.Icon,
				"payMethodType": method.PayMethodType,
				"payMethodName": method.PayMethodName,
			}
			if displayConfig, configErr := service.GetPaymentQuoteDisplayConfig(model.PaymentMethodWaffo, userGroup); configErr == nil {
				publicMethod["currency"] = displayConfig.Currency
				publicMethod["rate_to_usd"] = displayConfig.RateToUSD
				publicMethod["base_amount_multiplier"] = displayConfig.BaseAmountMultiplier
				publicMethod["topup_ratio"] = displayConfig.Coefficient
				publicMethod["rounding_decimals"] = displayConfig.RoundingDecimals
				if currency, currencyErr := model.GetPlatformCurrency(displayConfig.Currency); currencyErr == nil {
					publicMethod["currency_symbol"] = currency.Symbol
				}
			}
			waffoPayMethods = append(waffoPayMethods, publicMethod)
		}
	}

	data := gin.H{
		"enable_online_topup":              isEpayTopUpEnabled(),
		"enable_stripe_topup":              isStripeTopUpEnabled(),
		"enable_creem_topup":               enableCreem,
		"enable_waffo_topup":               enableWaffo,
		"enable_waffo_pancake_topup":       enableWaffoPancake,
		"enable_yookassa_topup":            enableYooKassa,
		"enable_nowpayments_topup":         enableNOWPayments,
		"enable_redemption":                complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return waffoPayMethods
			}
			return nil
		}(),
		"creem_products":  creemProducts,
		"pay_methods":     payMethods,
		"crypto_networks": directNetworks,
		// This is the minimum of the methods visible to the requesting user.
		// Returning zero when none are eligible keeps the wallet fail-closed.
		"min_topup":               minimumTopUp,
		"stripe_min_topup":        setting.StripeMinTopUp,
		"waffo_min_topup":         setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"yookassa_min_topup":      getMinTopup(),
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"cashback":                operation_setting.GetPaymentSetting().AmountCashback,
		"topup_link":              common.TopUpLink,
	}
	manualTopup := operation_setting.GetPaymentSetting()
	if manualTopup.ManualTopupEnabled && strings.TrimSpace(manualTopup.ManualTopupContactURL) != "" && manualTopup.ManualTopupMinAmount > 0 {
		rubRate, rateErr := service.GetPlatformCurrencyRate("RUB")
		if rateErr == nil && rubRate > 0 && !math.IsNaN(rubRate) && !math.IsInf(rubRate, 0) {
			minimumBackendAmount := manualTopup.ManualTopupMinAmount / rubRate
			if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
				minimumBackendAmount *= common.GetQuotaPerUnit()
			}
			if minimumBackendAmount > 0 && !math.IsNaN(minimumBackendAmount) && !math.IsInf(minimumBackendAmount, 0) {
				data["manual_topup_enabled"] = true
				data["manual_topup_min_amount"] = manualTopup.ManualTopupMinAmount
				data["manual_topup_min_amount_backend"] = minimumBackendAmount
				data["manual_topup_contact_url"] = manualTopup.ManualTopupContactURL
			}
		}
	}
	if currencies, currencyErr := model.ListPlatformCurrencies(true); currencyErr == nil {
		data["currencies"] = currencies
	}
	common.ApiSuccess(c, data)
}

func creemTopUpInfoForConfig(config setting.CreemConfig) (bool, string) {
	return isCreemTopUpEnabledForConfig(config), config.Products
}

// getPaymentMethodCurrency resolves the currency exposed in top-up metadata.
// Built-in gateways own their settlement currency, so their canonical
// provider configuration must take precedence over legacy PayMethods JSON.
// Unknown legacy EPay methods retain their configured currency and fall back
// to USD for backwards compatibility.
func getPaymentMethodCurrency(method map[string]string) string {
	if currency, err := service.PaymentMethodCurrency(method["type"]); err == nil && strings.TrimSpace(currency) != "" {
		return strings.ToUpper(strings.TrimSpace(currency))
	}
	if currency := strings.TrimSpace(method["currency"]); currency != "" {
		return strings.ToUpper(currency)
	}
	return "USD"
}

type EpayRequest struct {
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
}

type AmountRequest struct {
	Amount        float64 `json:"amount"`
	PaymentMethod string  `json:"payment_method"`
}

// GetTopUpQuote returns a server-calculated payment amount in the configured
// method currency without creating an order.
func GetTopUpQuote(c *gin.Context) {
	var req AmountRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid parameters")
		return
	}
	// Network-specific IDs are accepted only for backwards-compatible quote
	// requests. They must use the single Crypto catalog entry before the common
	// visibility check, otherwise a legacy ID could bypass crypto_direct's
	// admin_only policy.
	if isDirectUSDTPaymentMethod(req.PaymentMethod) {
		req.PaymentMethod = model.DirectCryptoProvider
	}
	if !paymentMethodAllowedForUser(c, req.PaymentMethod) {
		return
	}
	if isDirectUSDTPaymentMethod(req.PaymentMethod) {
		if !operation_setting.IsPaymentComplianceConfirmed() {
			common.ApiErrorMsg(c, "Payment compliance is not confirmed")
			return
		}
		if !model.IsDirectUSDTNetworkMethodConfigured(model.DirectCryptoProvider) {
			common.ApiErrorMsg(c, "Payment method does not exist")
			return
		}
		if len(model.DirectUSDTReadyNetworks()) == 0 {
			common.ApiErrorMsg(c, "Payment method is not configured")
			return
		}
	}
	var payMethods []map[string]string
	if strings.EqualFold(strings.TrimSpace(req.PaymentMethod), model.PaymentMethodYooKassaSBP) {
		var payMethodsErr error
		payMethods, payMethodsErr = topUpPayMethods()
		if payMethodsErr != nil {
			common.ApiError(c, payMethodsErr)
			return
		}
		if !isYooKassaSBPAvailableForMethods(setting.GetYooKassaConfig(), payMethods) {
			common.ApiErrorMsg(c, "Payment method does not exist")
			return
		}
	}
	group, err := model.GetUserGroup(c.GetInt("id"), true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var quote service.PaymentQuote
	if len(payMethods) > 0 {
		quote, err = service.BuildPaymentQuoteWithPayMethods(req.Amount, req.PaymentMethod, group, payMethods)
	} else {
		quote, err = service.BuildPaymentQuote(req.Amount, req.PaymentMethod, group)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, quote)
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func getPaymentTopupGroup(paymentMethod, userGroup string) string {
	payMethods, err := topUpPayMethods()
	if err != nil {
		return userGroup
	}
	return getPaymentTopupGroupFromMethods(payMethods, paymentMethod, userGroup)
}

func getPaymentTopupGroupFromMethods(payMethods []map[string]string, paymentMethod, userGroup string) string {
	for _, method := range payMethods {
		if method["type"] == paymentMethod && method["topup_group"] != "" {
			return method["topup_group"]
		}
	}
	return userGroup
}

func getMinTopup() float64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromFloat(minTopup)
		dQuotaPerUnit := decimal.NewFromFloat(common.GetQuotaPerUnit())
		minTopup = dMinTopup.Mul(dQuotaPerUnit).InexactFloat64()
	}
	return minTopup
}

func isTopUpPaymentAmountRepresentable(amount float64, decimalPlaces int32) bool {
	paymentAmount := decimal.NewFromFloat(amount)
	return paymentAmount.Equal(paymentAmount.Round(decimalPlaces))
}

func validateEpayPaymentAmount(topUp *model.TopUp, amount string) error {
	actual, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return fmt.Errorf("%w: invalid EPay payment amount: %v", service.ErrPaymentSnapshotValidation, err)
	}
	return service.ValidateAndBackfillLegacyPaymentSnapshot(topUp, model.PaymentProviderEpay, "USD", actual.InexactFloat64())
}

func epayTopUpPaymentMethodMatches(topUp *model.TopUp, callbackMethod string) bool {
	if topUp == nil {
		return false
	}
	expected := strings.TrimSpace(topUp.PaymentMethod)
	actual := strings.TrimSpace(callbackMethod)
	return expected != "" && actual != "" && strings.EqualFold(expected, actual)
}

func RequestEpay(c *gin.Context) {
	if !isEpayTopUpEnabled() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "在线充值未启用"})
		return
	}
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if !paymentMethodAllowedForUser(c, req.PaymentMethod) {
		return
	}
	id := c.GetInt("id")
	if user, userErr := model.GetUserById(id, false); userErr != nil || user == nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Epay user does not exist user_id=%d error=%v", id, userErr))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, req.PaymentMethod, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	if quote.Currency != "USD" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "Epay payment account currency is not configured for this payment method"})
		return
	}
	payMoney := quote.ChargedAmount
	providerPayMoney := decimal.NewFromFloat(payMoney).Round(2).InexactFloat64()
	if providerPayMoney < 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(providerPayMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付金额必须精确到分"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(paymentReturnPath("/console/log"))
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromFloat(amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.GetQuotaPerUnit())
		amount = dAmount.Div(dQuotaPerUnit).InexactFloat64()
	}
	topUp := &model.TopUp{
		UserId:            id,
		Amount:            int64(amount),
		RequestedAmount:   req.Amount,
		Money:             providerPayMoney,
		TradeNo:           tradeNo,
		PaymentMethod:     req.PaymentMethod,
		PaymentMethodName: model.PaymentMethodDisplayName(req.PaymentMethod),
		PaymentProvider:   model.PaymentProviderEpay,
		QuotaToAdd:        getTopUpQuotaToAdd(req.Amount),
		CreateTime:        time.Now().Unix(),
		Status:            common.TopUpStatusPending,
	}
	service.ApplyPaymentQuote(topUp, quote)
	topUp.PaymentChargedAmount = providerPayMoney
	topUp.Money = providerPayMoney
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 创建充值订单失败 user_id=%d trade_no=%s payment_method=%s amount=%g error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	uri, params, err := purchaseEpay(client, &epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%g", req.Amount),
		Money:          strconv.FormatFloat(providerPayMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 拉起支付失败 user_id=%d trade_no=%s payment_method=%s amount=%g error=%q", id, tradeNo, req.PaymentMethod, req.Amount, err.Error()))
		if isPermanentEpayPurchaseError(err) {
			if failErr := model.FailTopUpOrder(tradeNo, model.PaymentProviderEpay); failErr != nil {
				logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 永久失败订单关闭失败 trade_no=%s error=%q", tradeNo, failErr.Error()))
			}
		}
		// The provider may have accepted the order before the client observed
		// an error. Keep the durable pending row so a late callback can still
		// settle it; reconciliation can expire genuinely abandoned orders.
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值订单创建成功 user_id=%d trade_no=%s payment_method=%s amount=%g money=%.2f uri=%q params=%q", id, tradeNo, req.PaymentMethod, req.Amount, payMoney, uri, common.GetJsonString(params)))
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || verifyInfo == nil || !verifyInfo.VerifyStatus {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
		if topUp == nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调订单不存在 trade_no=%s callback_type=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, c.ClientIP(), common.GetJsonString(verifyInfo)))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if topUp.PaymentProvider != model.PaymentProviderEpay {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 订单支付网关不匹配 trade_no=%s order_provider=%s callback_type=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentProvider, verifyInfo.Type, c.ClientIP()))
			// The callback is authenticated, but this trade number belongs to a
			// different local provider. It cannot settle this order; acknowledge it
			// without mutation so EPay does not retry a permanent mismatch.
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		if !epayTopUpPaymentMethodMatches(topUp, verifyInfo.Type) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调支付方式与订单快照不匹配 trade_no=%s expected_method=%s actual_method=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentMethod, verifyInfo.Type, c.ClientIP()))
			// The callback is authenticated, but it belongs to another payment
			// method than the immutable order snapshot. It can never settle this
			// order safely; acknowledge it without changing balance or history.
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		if topUp.Status == common.TopUpStatusSuccess || topUp.Status == common.TopUpStatusFailed || topUp.Status == common.TopUpStatusExpired {
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		if err := validateEpayPaymentAmount(topUp, verifyInfo.Money); err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调金额与订单快照不匹配 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), err.Error()))
			if service.IsPermanentPaymentSnapshotError(err) {
				_, _ = c.Writer.Write([]byte("success"))
				return
			}
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if err := model.RechargeEpay(verifyInfo.ServiceTradeNo, verifyInfo.Type, c.ClientIP()); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值处理失败 trade_no=%s user_id=%d client_ip=%s error=%q", topUp.TradeNo, topUp.UserId, c.ClientIP(), err.Error()))
			if model.IsPermanentTopUpError(err, topUp) {
				_, _ = c.Writer.Write([]byte("success"))
				return
			}
			// Do not acknowledge a successful callback until local settlement
			// succeeds; returning fail makes EPay retry transient DB failures.
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 充值成功 trade_no=%s user_id=%d client_ip=%s money=%.2f", topUp.TradeNo, topUp.UserId, c.ClientIP(), topUp.Money))
		_, _ = c.Writer.Write([]byte("success"))
	} else if isTerminalEpayTradeStatus(verifyInfo.TradeStatus) {
		// EPay can report an explicit terminal rejection after checkout. Close
		// only on that provider state; intermediate/unknown statuses remain
		// pending so a later success callback can still reconcile the order.
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		topUp, lookupErr := model.GetTopUpByTradeNoWithError(verifyInfo.ServiceTradeNo)
		if lookupErr != nil && !errors.Is(lookupErr, model.ErrTopUpNotFound) {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 终态订单查询失败 trade_no=%s error=%q", verifyInfo.ServiceTradeNo, lookupErr.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if topUp != nil && topUp.PaymentProvider != model.PaymentProviderEpay {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 终态回调订单支付网关不匹配 trade_no=%s expected_provider=%s actual_provider=%s client_ip=%s", verifyInfo.ServiceTradeNo, model.PaymentProviderEpay, topUp.PaymentProvider, c.ClientIP()))
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		if topUp != nil && !epayTopUpPaymentMethodMatches(topUp, verifyInfo.Type) {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 终态回调支付方式与订单快照不匹配 trade_no=%s expected_method=%s actual_method=%s client_ip=%s", verifyInfo.ServiceTradeNo, topUp.PaymentMethod, verifyInfo.Type, c.ClientIP()))
			_, _ = c.Writer.Write([]byte("success"))
			return
		}
		if err := model.UpdatePendingTopUpStatus(verifyInfo.ServiceTradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed); err != nil &&
			!errors.Is(err, model.ErrTopUpNotFound) && !errors.Is(err, model.ErrTopUpStatusInvalid) {
			if errors.Is(err, model.ErrPaymentMethodMismatch) {
				_, _ = c.Writer.Write([]byte("success"))
				return
			}
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 终态订单更新失败 trade_no=%s error=%q", verifyInfo.ServiceTradeNo, err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 终态订单已关闭 trade_no=%s trade_status=%s", verifyInfo.ServiceTradeNo, verifyInfo.TradeStatus))
		_, _ = c.Writer.Write([]byte("success"))
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
		// Intermediate/unknown statuses are acknowledgements only. Keep the
		// local order pending until EPay reports success or a terminal state.
		_, _ = c.Writer.Write([]byte("success"))
	}
}

func isTerminalEpayTradeStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "TRADE_CLOSED", "TRADE_FAILED", "TRADE_CANCEL", "TRADE_CANCELED":
		return true
	default:
		return false
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	quote, err := service.BuildPaymentQuote(req.Amount, req.PaymentMethod, group)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return
	}
	payMoney := quote.ChargedAmount
	if payMoney <= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	if req.Amount != math.Trunc(req.Amount) && !isTopUpPaymentAmountRepresentable(payMoney, 2) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "支付金额必须精确到分"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

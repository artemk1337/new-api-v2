package service

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"golang.org/x/text/currency"
)

// ErrPaymentSnapshotValidation marks a provider callback that is permanently
// incompatible with the persisted payment snapshot. Database errors during
// legacy backfill intentionally remain unwrapped so webhook handlers retry.
var ErrPaymentSnapshotValidation = errors.New("payment snapshot validation failed")

func IsPermanentPaymentSnapshotError(err error) bool {
	return errors.Is(err, ErrPaymentSnapshotValidation)
}

func paymentSnapshotValidationError(message string) error {
	return fmt.Errorf("%w: %s", ErrPaymentSnapshotValidation, message)
}

// PaymentQuote is a server-side quote for one top-up request. Amounts in the
// payment currency are derived from the platform credit amount and never from
// client-supplied totals.
type PaymentQuote struct {
	Currency          string  `json:"currency"`
	RateToUSD         float64 `json:"rate_to_usd"`
	Coefficient       float64 `json:"coefficient"`
	BaseAmountUSD     float64 `json:"base_amount_usd"`
	CommissionUSD     float64 `json:"commission_usd"`
	CashbackPercent   float64 `json:"cashback_percent"`
	CashbackAmountUSD float64 `json:"cashback_amount_usd"`
	CreditedAmountUSD float64 `json:"credited_amount_usd"`
	ChargedAmountUSD  float64 `json:"charged_amount_usd"`
	ChargedAmount     float64 `json:"charged_amount"`
}

// PaymentQuoteDisplayConfig contains the public, amount-independent inputs
// needed to render a payment-method amount in the wallet.  It is deliberately
// not a quote: the amount is supplied by the browser and the server rebuilds
// the authoritative PaymentQuote when the payment is created.
//
// The display formula is:
//
//	charged_amount = ceil(amount * base_amount_multiplier * coefficient * rate_to_usd,
//	provider precision)
//
// where rate_to_usd means "1 USD = N units of the provider currency".  The
// multiplier already includes provider unit pricing and token-display
// conversion, exactly as BuildPaymentQuote does.
type PaymentQuoteDisplayConfig struct {
	Currency             string  `json:"currency"`
	RateToUSD            float64 `json:"rate_to_usd"`
	Coefficient          float64 `json:"coefficient"`
	BaseAmountMultiplier float64 `json:"base_amount_multiplier"`
	RoundingDecimals     int     `json:"rounding_decimals"`
}

// PaymentAmountRoundingDecimals returns the number of fractional digits the
// provider accepts for a settlement amount. Waffo's zero-decimal currencies
// are provider constraints, so keep the currency list next to quote/display
// logic and reuse it when creating the provider snapshot.
func PaymentAmountRoundingDecimals(paymentMethod, currency string) int {
	if strings.EqualFold(strings.TrimSpace(paymentMethod), model.PaymentMethodWaffo) {
		currency = strings.ToUpper(strings.TrimSpace(currency))
		if currency == "IDR" || currency == "JPY" || currency == "KRW" || currency == "VND" {
			return 0
		}
	}
	return 2
}

// TopUpCredit is the immutable accounting result for a payment quote.
// Cashback is intentionally kept separate from the credited base amount so
// the UI and settlement code can show the fee and reward independently.
type TopUpCredit struct {
	CreditedAmountUSD float64
	CashbackPercent   float64
	CashbackAmountUSD float64
	TotalAmountUSD    float64
}

// CalculateTopUpCredit calculates the balance credit represented by a payment
// quote. The requested/base amount is credited in full; the commission is
// charged on top of it. Cashback is calculated from that credited/base amount.
func CalculateTopUpCredit(baseAmountUSD, commissionUSD float64) TopUpCredit {
	if !isFinitePositive(baseAmountUSD) {
		return TopUpCredit{}
	}
	if !isFiniteNonNegative(commissionUSD) {
		commissionUSD = 0
	}

	cashbackPercent := operation_setting.GetPaymentSetting().AmountCashback.CashbackPercentForAmount(baseAmountUSD)
	if !isFiniteNonNegative(cashbackPercent) {
		cashbackPercent = 0
	} else if cashbackPercent > 100 {
		cashbackPercent = 100
	}
	cashbackAmountUSD := baseAmountUSD * cashbackPercent / 100
	return TopUpCredit{
		CreditedAmountUSD: baseAmountUSD,
		CashbackPercent:   cashbackPercent,
		CashbackAmountUSD: cashbackAmountUSD,
		TotalAmountUSD:    baseAmountUSD + cashbackAmountUSD,
	}
}

// CalculateTopUpQuota returns the balance credit represented by a payment
// quote in the internal quota unit. Quota conversion happens after the
// credited base amount and cashback are combined.
func CalculateTopUpQuota(baseAmountUSD, commissionUSD float64) int {
	totalAmountUSD := CalculateTopUpCredit(baseAmountUSD, commissionUSD).TotalAmountUSD
	return calculateTopUpQuotaAmount(totalAmountUSD)
}

func calculateTopUpQuotaAmount(totalAmountUSD float64) int {
	if !isFinitePositive(totalAmountUSD) {
		return 0
	}
	return int(decimal.NewFromFloat(totalAmountUSD).
		Mul(decimal.NewFromFloat(common.GetQuotaPerUnit())).IntPart())
}

func isFinitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value > 0
}

func isFiniteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func PaymentMethodCurrency(paymentMethod string) (string, error) {
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	// Built-in gateways own their settlement currency. Resolve these contracts
	// before legacy PayMethods so stale JSON cannot override a provider rule.
	switch paymentMethod {
	case model.PaymentMethodStripe, model.PaymentMethodWaffoPancake:
		return "USD", nil
	case model.PaymentMethodYooKassaSBP:
		return "RUB", nil
	case model.PaymentMethodWaffo:
		currency := strings.ToUpper(strings.TrimSpace(setting.WaffoCurrency))
		if currency == "" {
			return "", fmt.Errorf("Waffo currency is not configured")
		}
		return currency, nil
	case model.PaymentMethodNOWPayments:
		return "USDT", nil
	}
	for _, method := range operation_setting.PayMethodsSnapshot() {
		if strings.ToLower(strings.TrimSpace(method["type"])) == paymentMethod {
			// EPay methods have no per-method settlement currency contract;
			// they always settle in USD. Legacy PayMethods.currency is ignored.
			return "USD", nil
		}
	}
	return "", fmt.Errorf("payment method %s does not exist", paymentMethod)
}

func ValidatePaymentMethodCurrency(paymentMethod, currency string) error {
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if currency == "" {
		return fmt.Errorf("payment currency is required")
	}
	switch paymentMethod {
	case model.PaymentMethodYooKassaSBP:
		if currency != "RUB" {
			return fmt.Errorf("YooKassa SBP supports RUB only")
		}
	case model.PaymentMethodStripe:
		if currency != "USD" {
			return fmt.Errorf("Stripe top-up supports USD only")
		}
	case model.PaymentMethodNOWPayments:
		if currency != "USDT" {
			return fmt.Errorf("NOWPayments payment method currency is fixed to USDT")
		}
	case model.PaymentMethodWaffo:
		configured := strings.ToUpper(strings.TrimSpace(setting.WaffoCurrency))
		if configured == "" {
			configured = "USD"
		}
		if currency != configured {
			return fmt.Errorf("Waffo currency is fixed to %s", configured)
		}
	default:
		// Legacy EPay methods have no currency parameter; their account currency
		// is configured out of band, so USD is the only safe default.
		if currency != "USD" {
			return fmt.Errorf("legacy EPay payment methods support USD only")
		}
	}
	return nil
}

func paymentMethodGroup(paymentMethod, userGroup string) string {
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	for _, method := range operation_setting.PayMethodsSnapshot() {
		if strings.ToLower(strings.TrimSpace(method["type"])) == paymentMethod && strings.TrimSpace(method["topup_group"]) != "" {
			return method["topup_group"]
		}
	}
	return userGroup
}

// paymentMethodMinimum returns the minimum amount in the settlement currency
// configured for a gateway.  Provider minimums must be compared with
// PaymentQuote.ChargedAmount (the provider amount), never with the wallet
// amount or the USD accounting amount.
func paymentMethodMinimum(paymentMethod string) (float64, bool) {
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	switch paymentMethod {
	case model.PaymentMethodStripe:
		if setting.StripeMinTopUp > 0 {
			return setting.StripeMinTopUp, true
		}
	case model.PaymentMethodWaffo:
		if setting.WaffoMinTopUp > 0 {
			return setting.WaffoMinTopUp, true
		}
	case model.PaymentMethodWaffoPancake:
		if setting.WaffoPancakeMinTopUp > 0 {
			return setting.WaffoPancakeMinTopUp, true
		}
	}
	// Epay and legacy provider entries keep their optional per-method minimum
	// in PayMethods.  Missing or malformed values intentionally mean that no
	// method-specific minimum is configured; the shared minimum remains the
	// compatibility fallback enforced by each gateway endpoint.
	for _, method := range operation_setting.PayMethodsSnapshot() {
		if method == nil || strings.ToLower(strings.TrimSpace(method["type"])) != paymentMethod {
			continue
		}
		minimum, err := decimal.NewFromString(strings.TrimSpace(method["min_topup"]))
		if err != nil || !minimum.IsPositive() {
			return 0, false
		}
		return minimum.InexactFloat64(), true
	}
	return 0, false
}

// GetPaymentQuoteDisplayConfig resolves all amount-independent inputs used by
// BuildPaymentQuote.  Keep this function free of a requested amount so the
// top-up info endpoint can preload it once when the wallet opens.
func GetPaymentQuoteDisplayConfig(paymentMethod, userGroup string) (PaymentQuoteDisplayConfig, error) {
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	currency, err := PaymentMethodCurrency(paymentMethod)
	if err != nil {
		return PaymentQuoteDisplayConfig{}, err
	}
	if err := ValidatePaymentMethodCurrency(paymentMethod, currency); err != nil {
		return PaymentQuoteDisplayConfig{}, err
	}

	baseAmountMultiplier := 1.0
	switch paymentMethod {
	case model.PaymentMethodStripe:
		baseAmountMultiplier *= setting.StripeUnitPrice
	case model.PaymentMethodWaffo:
		baseAmountMultiplier *= setting.WaffoUnitPrice
	}
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		baseAmountMultiplier /= common.GetQuotaPerUnit()
	}
	coefficient := common.GetTopupGroupRatio(paymentMethodGroup(paymentMethod, userGroup))
	if coefficient <= 1 {
		coefficient = 1
	}
	rate, err := GetPlatformCurrencyRate(currency)
	if err != nil {
		return PaymentQuoteDisplayConfig{}, err
	}
	return PaymentQuoteDisplayConfig{
		Currency:             currency,
		RateToUSD:            rate,
		Coefficient:          coefficient,
		BaseAmountMultiplier: baseAmountMultiplier,
		RoundingDecimals:     PaymentAmountRoundingDecimals(paymentMethod, currency),
	}, nil
}

func validatePaymentMethodMinimum(paymentMethod string, quote PaymentQuote) error {
	minimum, configured := paymentMethodMinimum(paymentMethod)
	if !configured || quote.ChargedAmount+1e-9 >= minimum {
		return nil
	}
	return fmt.Errorf("amount must be at least %s %s", decimal.NewFromFloat(minimum).StringFixed(2), quote.Currency)
}

func BuildPaymentQuote(amount float64, paymentMethod, userGroup string) (PaymentQuote, error) {
	if !isFinitePositive(amount) {
		return PaymentQuote{}, fmt.Errorf("amount must be greater than zero")
	}
	paymentMethod = strings.ToLower(strings.TrimSpace(paymentMethod))
	// Provider integration settings own settlement currency. The legacy EPay
	// Price/PayMethods currency fields are intentionally ignored.
	displayConfig, err := GetPaymentQuoteDisplayConfig(paymentMethod, userGroup)
	if err != nil {
		return PaymentQuote{}, err
	}
	baseUSD := amount * displayConfig.BaseAmountMultiplier
	currency := displayConfig.Currency
	rate := displayConfig.RateToUSD
	coefficient := displayConfig.Coefficient
	commission := 0.0
	if coefficient > 1 {
		commission = baseUSD * (coefficient - 1)
	}
	chargedUSD := baseUSD + commission
	// Providers settle in their own currency and may reject values with more
	// fractional digits than they support. Round the amount upward before it is
	// sent to a provider. Downward rounding could make the provider charge less
	// than the immutable quote while the original quota is still credited.
	providerDecimals := PaymentAmountRoundingDecimals(paymentMethod, currency)
	chargedAmount := decimal.NewFromFloat(chargedUSD).
		Mul(decimal.NewFromFloat(rate)).
		RoundUp(int32(providerDecimals))
	chargedAmountInUSD := chargedAmount.Div(decimal.NewFromFloat(rate))
	chargedUSD = chargedAmountInUSD.InexactFloat64()
	commission = math.Max(0, chargedUSD-baseUSD)
	credit := CalculateTopUpCredit(baseUSD, commission)
	quote := PaymentQuote{Currency: currency, RateToUSD: rate, Coefficient: coefficient,
		BaseAmountUSD: baseUSD, CommissionUSD: commission, ChargedAmountUSD: chargedUSD,
		CashbackPercent: credit.CashbackPercent, CashbackAmountUSD: credit.CashbackAmountUSD,
		CreditedAmountUSD: credit.CreditedAmountUSD, ChargedAmount: chargedAmount.InexactFloat64()}
	if err := validatePaymentMethodMinimum(paymentMethod, quote); err != nil {
		return PaymentQuote{}, err
	}
	return quote, nil
}

func ApplyPaymentQuote(topUp *model.TopUp, quote PaymentQuote) {
	topUp.PaymentCurrency = quote.Currency
	topUp.PaymentRateToUSD = quote.RateToUSD
	topUp.PaymentCoefficient = quote.Coefficient
	topUp.PaymentBaseAmount = quote.BaseAmountUSD
	topUp.PaymentCommission = quote.CommissionUSD
	topUp.PaymentChargedAmount = quote.ChargedAmount
	if topUp.RequestedAmount > 0 {
		topUp.QuotaToAdd = CalculateTopUpQuota(quote.BaseAmountUSD, quote.CommissionUSD)
	}
	// Money remains the historical provider amount field.
	topUp.Money = quote.ChargedAmount
}

// ApplyPaymentSnapshot records a provider's already-calculated amount when a
// gateway has its own unit-price/catalog semantics. Callers must pass the
// exact amount sent to that gateway; callbacks then use this immutable value.
func ApplyPaymentSnapshot(topUp *model.TopUp, currency string, rate, baseAmount, coefficient, chargedAmount float64) {
	if coefficient <= 1 {
		coefficient = 1
	}
	topUp.PaymentCurrency = strings.ToUpper(strings.TrimSpace(currency))
	if topUp.PaymentCurrency == "" {
		topUp.PaymentCurrency = "USD"
	}
	topUp.PaymentRateToUSD = rate
	topUp.PaymentCoefficient = coefficient
	topUp.PaymentBaseAmount = baseAmount
	topUp.PaymentCommission = 0
	if coefficient > 1 {
		topUp.PaymentCommission = baseAmount * (coefficient - 1)
	}
	topUp.PaymentChargedAmount = chargedAmount
	topUp.Money = chargedAmount
	if topUp.RequestedAmount > 0 {
		topUp.QuotaToAdd = CalculateTopUpQuota(topUp.PaymentBaseAmount, topUp.PaymentCommission)
	}
}

// ApplyPaymentQuoteSnapshot records a provider-rounded settlement amount while
// keeping the accounting fields from the immutable quote. The quote rate and
// base amount must remain authoritative after quote creation; re-reading a
// currency rate here could turn a transient rate change into an inflated
// balance credit.
func ApplyPaymentQuoteSnapshot(topUp *model.TopUp, quote PaymentQuote, chargedAmount float64) {
	ApplyPaymentSnapshot(topUp, quote.Currency, quote.RateToUSD, quote.BaseAmountUSD, quote.Coefficient, chargedAmount)
	// ApplyPaymentSnapshot derives commission from base*coefficient. That is
	// only the pre-rounding value; BuildPaymentQuote may have rounded the
	// provider charge upward, so keep the quote's immutable commission instead
	// of recomputing it from the rounded amount.
	topUp.PaymentCommission = quote.CommissionUSD
	if topUp.RequestedAmount > 0 {
		topUp.QuotaToAdd = calculateTopUpQuotaAmount(quote.CreditedAmountUSD + quote.CashbackAmountUSD)
	}
}

func ValidatePaymentSnapshot(topUp *model.TopUp, currency string, amount float64) error {
	if topUp == nil {
		return paymentSnapshotValidationError("payment snapshot is missing")
	}
	expectedCurrency := strings.ToUpper(strings.TrimSpace(topUp.PaymentCurrency))
	if expectedCurrency == "" {
		return paymentSnapshotValidationError("payment currency is missing")
	}
	if !strings.EqualFold(expectedCurrency, currency) {
		return paymentSnapshotValidationError("payment currency mismatch")
	}
	expectedAmount := topUp.PaymentChargedAmount
	if expectedAmount <= 0 {
		expectedAmount = topUp.Money
	}
	if math.Abs(expectedAmount-amount) > 0.000001 {
		return paymentSnapshotValidationError("payment amount mismatch")
	}
	return nil
}

// IsLegacyPaymentSnapshot identifies orders created before immutable payment
// snapshots were introduced. Those rows have no captured base/charged amount;
// PaymentCurrency may be the database migration default (USD), which is not
// evidence of the currency used by the provider.
func IsLegacyPaymentSnapshot(topUp *model.TopUp) bool {
	return topUp != nil && topUp.PaymentBaseAmount <= 0 && topUp.PaymentChargedAmount <= 0
}

// ValidateAndBackfillLegacyPaymentSnapshot validates a provider callback for
// an old order and persists only the trusted settlement currency and amount.
// For gateways whose callback carries the currency (Waffo/Creem), that signed
// provider value is the only safe source for a legacy row. We deliberately do
// not invent a historical rate or USD base amount; legacy quota fields remain
// authoritative for accounting.
func ValidateAndBackfillLegacyPaymentSnapshot(topUp *model.TopUp, provider, currency string, amount float64) error {
	if topUp == nil {
		return paymentSnapshotValidationError("payment snapshot is missing")
	}
	if !IsLegacyPaymentSnapshot(topUp) {
		return ValidatePaymentSnapshot(topUp, currency, amount)
	}

	provider = strings.ToLower(strings.TrimSpace(provider))
	normalizedCurrency := strings.ToUpper(strings.TrimSpace(currency))
	if !isProviderCurrencyCode(normalizedCurrency) {
		return paymentSnapshotValidationError("payment currency is missing")
	}
	switch provider {
	case model.PaymentProviderNOWPayments:
		if !isNOWPaymentsLegacyCurrency(normalizedCurrency) {
			return paymentSnapshotValidationError("payment currency mismatch")
		}
	case model.PaymentProviderStripe, model.PaymentProviderEpay, model.PaymentProviderWaffoPancake:
		if normalizedCurrency != "USD" {
			return paymentSnapshotValidationError("payment currency mismatch")
		}
	case model.PaymentProviderYooKassa:
		if normalizedCurrency != "RUB" {
			return paymentSnapshotValidationError("payment currency mismatch")
		}
	case model.PaymentProviderWaffo, model.PaymentProviderCreem:
		// Waffo and Creem include the settlement currency in their signed/
		// verified provider callback. Accept that value for legacy rows because
		// the old schema did not persist it.
	default:
		return paymentSnapshotValidationError("legacy payment currency is ambiguous")
	}

	if !isFinitePositive(topUp.Money) || !isFinitePositive(amount) {
		return paymentSnapshotValidationError("payment amount mismatch")
	}
	providerDecimals := 2
	if provider == model.PaymentProviderWaffo {
		providerDecimals = PaymentAmountRoundingDecimals(model.PaymentMethodWaffo, normalizedCurrency)
	}
	expectedAmount := decimal.NewFromFloat(topUp.Money).Round(int32(providerDecimals)).InexactFloat64()
	if math.Abs(expectedAmount-amount) > 0.000001 {
		return paymentSnapshotValidationError("payment amount mismatch")
	}
	// A detached in-memory TopUp (for example, a pure validator caller) has no
	// row to backfill. Keep validation useful in that context; real webhook
	// settlement always loads a persisted row with a non-zero id and database.
	if model.DB == nil || topUp.Id <= 0 {
		topUp.PaymentCurrency = normalizedCurrency
		topUp.PaymentChargedAmount = amount
		return nil
	}
	updates := map[string]interface{}{
		"payment_currency":       normalizedCurrency,
		"payment_charged_amount": amount,
	}
	result := model.DB.Model(&model.TopUp{}).
		Where("id = ? AND (payment_base_amount <= 0 OR payment_base_amount IS NULL) AND (payment_charged_amount <= 0 OR payment_charged_amount IS NULL)", topUp.Id).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	// A concurrent callback may have performed the same backfill. Reloading is
	// unnecessary for validation; the CAS settlement below remains authoritative.
	topUp.PaymentCurrency = normalizedCurrency
	topUp.PaymentChargedAmount = amount
	return nil
}

func isProviderCurrencyCode(currency string) bool {
	if len(currency) < 3 || len(currency) > 8 {
		return false
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

func isNOWPaymentsLegacyCurrency(value string) bool {
	if value == "USDT" {
		return true
	}
	_, err := currency.ParseISO(value)
	return err == nil
}

package currency_exchange_rate_setting

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ProviderCBR       = "cbr"
	ProviderBybitP2P  = "bybit_p2p"
	ProviderCoinGecko = "coingecko"

	UpdateIntervalMinute = "minute"
	UpdateIntervalHour   = "hour"
	UpdateIntervalDay    = "day"
)

type Config struct {
	Provider       string `json:"provider"`
	UpdateInterval string `json:"update_interval"`
}

var defaultConfig = Config{
	Provider:       ProviderBybitP2P,
	UpdateInterval: UpdateIntervalDay,
}

func init() {
	config.GlobalConfig.Register("currency_exchange_rate", &defaultConfig)
}

func GetConfig() *Config {
	return &defaultConfig
}

func ValidateProvider(provider string) error {
	if provider != ProviderCBR && provider != ProviderBybitP2P && provider != ProviderCoinGecko {
		return fmt.Errorf("unsupported currency exchange rate provider: %q", provider)
	}
	return nil
}

// SupportedProvidersForCurrency returns providers that can quote 1 USD in the
// requested platform currency. The registry stores USD-based rates, so the
// legacy USDT/RUB Bybit P2P pair is intentionally not a platform-currency
// provider.
func SupportedProvidersForCurrency(currency string) []string {
	switch strings.ToUpper(strings.TrimSpace(currency)) {
	case "USDT":
		return []string{ProviderCoinGecko}
	case "AED", "AMD", "AUD", "AZN", "BGN", "BRL", "BYN", "CAD", "CHF", "CNY", "CZK", "DKK", "EGP", "EUR", "GBP", "GEL", "HKD", "HUF", "IDR", "INR", "JPY", "KGS", "KRW", "KZT", "MDL", "NOK", "NZD", "PLN", "QAR", "RON", "RSD", "RUB", "SEK", "SGD", "THB", "TJS", "TMT", "TRY", "UAH", "UZS", "VND", "XDR", "ZAR":
		return []string{ProviderCBR}
	default:
		return nil
	}
}

func IsProviderSupportedForCurrency(provider, currency string) bool {
	return slices.Contains(SupportedProvidersForCurrency(currency), strings.ToLower(strings.TrimSpace(provider)))
}

func ValidateUpdateInterval(interval string) error {
	switch interval {
	case UpdateIntervalMinute, UpdateIntervalHour, UpdateIntervalDay:
		return nil
	default:
		return fmt.Errorf("unsupported currency exchange rate update interval: %q", interval)
	}
}

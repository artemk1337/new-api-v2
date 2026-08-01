package currency_exchange_rate_setting

import (
	"fmt"

	"github.com/QuantumNous/new-api/setting/config"
)

const (
	ProviderCBR      = "cbr"
	ProviderBybitP2P = "bybit_p2p"

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
	if provider != ProviderCBR && provider != ProviderBybitP2P {
		return fmt.Errorf("unsupported currency exchange rate provider: %q", provider)
	}
	return nil
}

func ValidateUpdateInterval(interval string) error {
	switch interval {
	case UpdateIntervalMinute, UpdateIntervalHour, UpdateIntervalDay:
		return nil
	default:
		return fmt.Errorf("unsupported currency exchange rate update interval: %q", interval)
	}
}

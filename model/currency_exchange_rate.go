package model

import (
	"strings"
	"time"
)

// CurrencyExchangeRate stores one observed quote currency amount for one unit
// of the base currency.
type CurrencyExchangeRate struct {
	ID            int64     `json:"id" gorm:"primaryKey"`
	BaseCurrency  string    `json:"base_currency" gorm:"type:varchar(4);not null;uniqueIndex:idx_currency_exchange_rate_pair_recorded,priority:1"`
	QuoteCurrency string    `json:"quote_currency" gorm:"type:varchar(4);not null;uniqueIndex:idx_currency_exchange_rate_pair_recorded,priority:2"`
	Provider      string    `json:"provider" gorm:"type:varchar(32);not null;index"`
	Rate          float64   `json:"rate" gorm:"not null"`
	RecordedAt    time.Time `json:"recorded_at" gorm:"not null;uniqueIndex:idx_currency_exchange_rate_pair_recorded,priority:3"`
}

func (CurrencyExchangeRate) TableName() string {
	return "currency_exchange_rates"
}

func CreateCurrencyExchangeRate(rate *CurrencyExchangeRate) error {
	rate.BaseCurrency = strings.ToUpper(strings.TrimSpace(rate.BaseCurrency))
	rate.QuoteCurrency = strings.ToUpper(strings.TrimSpace(rate.QuoteCurrency))
	rate.RecordedAt = rate.RecordedAt.UTC()
	return DB.Create(rate).Error
}

// GetLatestCurrencyExchangeRate returns the most recent valid observation for
// a pair. Historical rows are append-only, so this remains compatible with
// installations that already have only USDT/RUB or USD/RUB observations.
func GetLatestCurrencyExchangeRate(baseCurrency, quoteCurrency string) (*CurrencyExchangeRate, error) {
	return getLatestCurrencyExchangeRate(baseCurrency, quoteCurrency, "")
}

func GetLatestCurrencyExchangeRateByProvider(baseCurrency, quoteCurrency, provider string) (*CurrencyExchangeRate, error) {
	return getLatestCurrencyExchangeRate(baseCurrency, quoteCurrency, provider)
}

func getLatestCurrencyExchangeRate(baseCurrency, quoteCurrency, provider string) (*CurrencyExchangeRate, error) {
	var rate CurrencyExchangeRate
	query := DB.Where("base_currency = ? AND quote_currency = ? AND rate > 0",
		strings.ToUpper(strings.TrimSpace(baseCurrency)), strings.ToUpper(strings.TrimSpace(quoteCurrency)))
	if strings.TrimSpace(provider) != "" {
		query = query.Where("provider = ?", strings.TrimSpace(provider))
	}
	err := query.Order("recorded_at DESC, id DESC").First(&rate).Error
	if err != nil {
		return nil, err
	}
	return &rate, nil
}

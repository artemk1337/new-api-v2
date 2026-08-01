package model

import "time"

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
	rate.RecordedAt = rate.RecordedAt.UTC()
	return DB.Create(rate).Error
}

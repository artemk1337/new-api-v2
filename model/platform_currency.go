package model

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/setting/currency_exchange_rate_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PlatformCurrency describes a currency that can be displayed or selected by
// a payment method. Rates are expressed as: 1 USD = RateToUSD units of Code.
// Payment methods can use enabled registry rows as their settlement currency.
type PlatformCurrency struct {
	Code            string     `json:"code" gorm:"primaryKey;type:varchar(8)"`
	Name            string     `json:"name" gorm:"type:varchar(64);not null"`
	Symbol          string     `json:"symbol" gorm:"type:varchar(8);not null"`
	Enabled         bool       `json:"enabled" gorm:"not null"`
	SyncEnabled     bool       `json:"sync_enabled" gorm:"not null"`
	SyncProvider    string     `json:"sync_provider" gorm:"type:varchar(32);not null"`
	ManualRateToUSD float64    `json:"manual_rate_to_usd" gorm:"not null"`
	RateToUSD       float64    `json:"rate_to_usd" gorm:"not null"`
	LastSyncAt      *time.Time `json:"last_sync_at,omitempty"`
	LastSyncError   string     `json:"last_sync_error,omitempty" gorm:"type:varchar(512);not null"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (PlatformCurrency) TableName() string { return "platform_currencies" }

func NormalizePlatformCurrency(currency *PlatformCurrency) {
	currency.Code = strings.ToUpper(strings.TrimSpace(currency.Code))
	currency.Name = strings.TrimSpace(currency.Name)
	currency.Symbol = strings.TrimSpace(currency.Symbol)
	currency.SyncProvider = strings.ToLower(strings.TrimSpace(currency.SyncProvider))
	if currency.SyncEnabled && !currency_exchange_rate_setting.IsProviderSupportedForCurrency(currency.SyncProvider, currency.Code) {
		providers := currency_exchange_rate_setting.SupportedProvidersForCurrency(currency.Code)
		if len(providers) == 0 {
			currency.SyncEnabled = false
			currency.SyncProvider = ""
		} else {
			currency.SyncProvider = providers[0]
		}
	}
	if currency.Code == "USD" {
		currency.ManualRateToUSD = 1
		currency.RateToUSD = 1
	}
	if currency.RateToUSD <= 0 {
		currency.RateToUSD = currency.ManualRateToUSD
	}
}

func ListPlatformCurrencies(enabledOnly bool) ([]PlatformCurrency, error) {
	query := DB.Order("code ASC")
	if enabledOnly {
		query = query.Where("enabled = ?", true)
	}
	var currencies []PlatformCurrency
	if err := query.Find(&currencies).Error; err != nil {
		return nil, err
	}
	return currencies, nil
}

func GetPlatformCurrency(code string) (*PlatformCurrency, error) {
	var currency PlatformCurrency
	err := DB.Where("code = ?", strings.ToUpper(strings.TrimSpace(code))).First(&currency).Error
	if err != nil {
		return nil, err
	}
	return &currency, nil
}

func SavePlatformCurrency(currency *PlatformCurrency) error {
	NormalizePlatformCurrency(currency)
	return DB.Save(currency).Error
}

// UpdatePlatformCurrencySettings persists only explicitly requested admin
// fields. Sync-source changes are guarded by their observed prior config so a
// stale editor cannot restore an old provider after another admin switches it.
func UpdatePlatformCurrencySettings(code string, updates map[string]interface{}, expectedSyncEnabled *bool, expectedSyncProvider string) error {
	return UpdatePlatformCurrencySettingsWithGuard(code, updates, expectedSyncEnabled, expectedSyncProvider, nil)
}

// UpdatePlatformCurrencySettingsWithGuard serializes currency mutations with
// payment-option activation by locking the mandatory USD registry row in the
// same database transaction. The callback runs while that lock is held.
func UpdatePlatformCurrencySettingsWithGuard(code string, updates map[string]interface{}, expectedSyncEnabled *bool, expectedSyncProvider string, guard func() error) error {
	if guard == nil {
		return UpdatePlatformCurrencySettingsWithTxGuard(code, updates, expectedSyncEnabled, expectedSyncProvider, nil)
	}
	return UpdatePlatformCurrencySettingsWithTxGuard(code, updates, expectedSyncEnabled, expectedSyncProvider, func(*gorm.DB) error {
		return guard()
	})
}

// UpdatePlatformCurrencySettingsWithTxGuard runs the guard on the same
// transaction that owns the USD mutex, allowing callers to read a consistent
// latest payment-settings snapshot while cross-replica mutations are blocked.
func UpdatePlatformCurrencySettingsWithTxGuard(code string, updates map[string]interface{}, expectedSyncEnabled *bool, expectedSyncProvider string, guard func(*gorm.DB) error) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return errors.New("currency code is required")
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if guard != nil {
			if err := lockPlatformCurrencyGuard(tx); err != nil {
				return err
			}
			if err := guard(tx); err != nil {
				return err
			}
		}
		query := tx.Model(&PlatformCurrency{}).Where("code = ?", code)
		if expectedSyncEnabled != nil {
			query = query.Where("sync_enabled = ? AND sync_provider = ?", *expectedSyncEnabled, strings.ToLower(strings.TrimSpace(expectedSyncProvider)))
		}
		result := query.Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			if expectedSyncEnabled != nil {
				return ErrPlatformCurrencySyncConfigChanged
			}
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

var ErrPlatformCurrencySyncConfigChanged = errors.New("platform currency synchronization configuration changed")

func updatePlatformCurrencySyncState(tx *gorm.DB, code, provider string, updates map[string]interface{}) error {
	code = strings.ToUpper(strings.TrimSpace(code))
	provider = strings.ToLower(strings.TrimSpace(provider))
	if code == "" {
		return errors.New("currency code is required")
	}
	result := tx.Model(&PlatformCurrency{}).
		Where("code = ? AND sync_enabled = ? AND sync_provider = ?", code, true, provider).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrPlatformCurrencySyncConfigChanged
	}
	return nil
}

// CommitPlatformCurrencySyncQuote atomically saves a quote and publishes it
// only if the currency is still synchronized by the provider that was queried.
func CommitPlatformCurrencySyncQuote(code, provider string, rate float64, syncedAt time.Time) error {
	syncedAt = syncedAt.UTC()
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := updatePlatformCurrencySyncState(tx, code, provider, map[string]interface{}{
			"rate_to_usd":     rate,
			"last_sync_at":    syncedAt,
			"last_sync_error": "",
		}); err != nil {
			return err
		}
		return tx.Create(&CurrencyExchangeRate{
			BaseCurrency: "USD", QuoteCurrency: strings.ToUpper(strings.TrimSpace(code)),
			Provider: provider, Rate: rate, RecordedAt: syncedAt,
		}).Error
	})
}

// RecordPlatformCurrencySyncError records a failure only for the same active
// provider snapshot. An in-flight old provider must not overwrite diagnostics
// after an administrator switches the currency configuration.
func RecordPlatformCurrencySyncError(code, provider, syncErr string) error {
	return updatePlatformCurrencySyncState(DB, code, provider, map[string]interface{}{
		"last_sync_error": syncErr,
	})
}

func DeletePlatformCurrency(code string) error {
	return DeletePlatformCurrencyWithGuard(code, nil)
}

func DeletePlatformCurrencyWithGuard(code string, guard func() error) error {
	if guard == nil {
		return DeletePlatformCurrencyWithTxGuard(code, nil)
	}
	return DeletePlatformCurrencyWithTxGuard(code, func(*gorm.DB) error { return guard() })
}

func DeletePlatformCurrencyWithTxGuard(code string, guard func(*gorm.DB) error) error {
	if strings.EqualFold(strings.TrimSpace(code), "USD") {
		return errors.New("USD is the required base currency and cannot be deleted")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if guard != nil {
			if err := lockPlatformCurrencyGuard(tx); err != nil {
				return err
			}
			if err := guard(tx); err != nil {
				return err
			}
		}
		return tx.Delete(&PlatformCurrency{}, "code = ?", strings.ToUpper(strings.TrimSpace(code))).Error
	})
}

// lockPlatformCurrencyGuard uses the mandatory USD row as a small, portable
// cross-database mutex. GORM emits FOR UPDATE where supported; SQLite
// serializes the surrounding write transaction instead.
func lockPlatformCurrencyGuard(tx *gorm.DB) error {
	var guard PlatformCurrency
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guard, "code = ?", "USD").Error
}

var platformCurrencySeedMutex sync.Mutex

// SeedDefaultPlatformCurrencies is idempotent and only inserts missing rows.
func SeedDefaultPlatformCurrencies() error {
	platformCurrencySeedMutex.Lock()
	defer platformCurrencySeedMutex.Unlock()

	defaults := []PlatformCurrency{
		{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1},
		{Code: "RUB", Name: "Russian Ruble", Symbol: "₽", Enabled: true, ManualRateToUSD: 90, RateToUSD: 90, SyncProvider: "cbr"},
		{Code: "USDT", Name: "Tether", Symbol: "₮", Enabled: true, ManualRateToUSD: 1, RateToUSD: 1, SyncProvider: "coingecko"},
		{Code: "EUR", Name: "Euro", Symbol: "€", Enabled: true, ManualRateToUSD: 0.92, RateToUSD: 0.92, SyncProvider: "cbr"},
		{Code: "CNY", Name: "Chinese Yuan", Symbol: "¥", Enabled: true, ManualRateToUSD: 7.2, RateToUSD: 7.2, SyncProvider: "cbr"},
	}
	for _, currency := range defaults {
		NormalizePlatformCurrency(&currency)
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&currency).Error; err != nil {
			return err
		}
	}
	var existing []PlatformCurrency
	if err := DB.Find(&existing).Error; err != nil {
		return err
	}
	for i := range existing {
		before := existing[i]
		NormalizePlatformCurrency(&existing[i])
		updates := map[string]interface{}{}
		if before.SyncEnabled != existing[i].SyncEnabled || before.SyncProvider != existing[i].SyncProvider {
			rateToUSD := 0.0
			if !existing[i].SyncEnabled {
				rateToUSD = existing[i].ManualRateToUSD
			}
			updates["sync_enabled"] = existing[i].SyncEnabled
			updates["sync_provider"] = existing[i].SyncProvider
			updates["rate_to_usd"] = rateToUSD
			updates["last_sync_at"] = nil
			updates["last_sync_error"] = ""
		}
		if existing[i].Code == "USD" {
			updates["enabled"] = true
			updates["manual_rate_to_usd"] = 1
			updates["rate_to_usd"] = 1
		}
		if len(updates) > 0 {
			if err := DB.Model(&PlatformCurrency{}).Where("code = ?", existing[i].Code).Updates(updates).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func IsPlatformCurrencyNotFound(err error) bool { return err == gorm.ErrRecordNotFound }

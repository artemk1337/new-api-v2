package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/bytedance/gopkg/util/gopool"
)

const (
	currencyExchangeRateProviderCBR       = "cbr"
	currencyExchangeRateProviderBybitP2P  = "bybit_p2p"
	currencyExchangeRateProviderCoinGecko = "coingecko"
	currencyExchangeRateCBRURL            = "https://www.cbr-xml-daily.ru/daily_json.js"
	currencyExchangeRateBybitP2PURL       = "https://api2.bybit.com/fiat/otc/item/online"
	currencyExchangeRateCoinGeckoURL      = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=usd"
	currencyExchangeRateTimeout           = 15 * time.Second
	// Bybit P2P API side 0 returns USDT sell ads: the price paid when buying
	// USDT for RUB. This product decision is fixed for now.
	currencyExchangeRateBybitP2PSideBuyUSDTForRUB = "0"
	currencyExchangeRateBybitP2PRequestSize       = "20"
	currencyExchangeRateScheduleCheckInterval     = time.Second
)

var (
	currencyExchangeRateHTTPClient   = &http.Client{Timeout: currencyExchangeRateTimeout}
	currencyExchangeRateTaskOnce     sync.Once
	currencyExchangeRateRunning      atomic.Bool
	platformCurrencySyncRunning      atomic.Bool
	currencyExchangeRateFetchForPair = FetchCurrencyExchangeRateForPair
)

type cbrDailyResponse struct {
	Valute map[string]struct {
		Nominal float64 `json:"Nominal"`
		Value   float64 `json:"Value"`
	} `json:"Valute"`
}

type coinGeckoUSDTResponse struct {
	Tether struct {
		USD float64 `json:"usd"`
	} `json:"tether"`
}

type bybitP2PRequest struct {
	UserID     string `json:"userId"`
	TokenID    string `json:"tokenId"`
	CurrencyID string `json:"currencyId"`
	PaymentID  string `json:"paymentId"`
	Side       string `json:"side"`
	Size       string `json:"size"`
	Page       string `json:"page"`
	Amount     string `json:"amount"`
	AuthMaker  bool   `json:"authMaker"`
	CanTrade   bool   `json:"canTrade"`
}

type bybitP2PResponse struct {
	RetCode int    `json:"ret_code"`
	RetMsg  string `json:"ret_msg"`
	Result  struct {
		Items []struct {
			Price string `json:"price"`
		} `json:"items"`
	} `json:"result"`
}

type currencyExchangeRateQuote struct {
	BaseCurrency  string
	QuoteCurrency string
	Rate          float64
}

func currencyExchangeRateUpdateInterval(value string) time.Duration {
	switch strings.TrimSpace(value) {
	case "minute":
		return time.Minute
	case "hour":
		return time.Hour
	case "day", "":
		return 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func currencyExchangeRateUpdateDue(lastAttempt time.Time, now time.Time, interval time.Duration) bool {
	return lastAttempt.IsZero() || !now.Before(lastAttempt.Add(interval))
}

func currencyExchangeRateOption(key, fallback string) string {
	common.OptionMapRWMutex.RLock()
	defer common.OptionMapRWMutex.RUnlock()
	if value := strings.TrimSpace(common.OptionMap[key]); value != "" {
		return value
	}
	return fallback
}

func fetchCBRUSDRUB(ctx context.Context) (float64, error) {
	return fetchCBRPair(ctx, "USD", "RUB")
}

func fetchCBRPair(ctx context.Context, baseCurrency, quoteCurrency string) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, currencyExchangeRateCBRURL, nil)
	if err != nil {
		return 0, err
	}
	response, err := currencyExchangeRateHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("CBR returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var payload cbrDailyResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	toRUB := func(code string) (float64, error) {
		if code == "RUB" {
			return 1, nil
		}
		quote, ok := payload.Valute[code]
		if !ok || quote.Nominal <= 0 || quote.Value <= 0 {
			return 0, fmt.Errorf("CBR %s quote is missing or invalid", code)
		}
		return quote.Value / quote.Nominal, nil
	}
	base, err := toRUB(strings.ToUpper(baseCurrency))
	if err != nil {
		return 0, err
	}
	quote, err := toRUB(strings.ToUpper(quoteCurrency))
	if err != nil {
		return 0, err
	}
	return base / quote, nil
}

func fetchCoinGeckoUSDTUSD(ctx context.Context) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, currencyExchangeRateCoinGeckoURL, nil)
	if err != nil {
		return 0, err
	}
	response, err := currencyExchangeRateHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("CoinGecko returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	var payload coinGeckoUSDTResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	if payload.Tether.USD <= 0 {
		return 0, fmt.Errorf("CoinGecko USDT/USD quote is missing or invalid")
	}
	return payload.Tether.USD, nil
}

func parseBybitP2PUSDTRUB(body []byte) (float64, error) {
	var payload bybitP2PResponse
	if err := common.Unmarshal(body, &payload); err != nil {
		return 0, err
	}
	if payload.RetCode != 0 {
		return 0, fmt.Errorf("Bybit P2P returned code %d: %s", payload.RetCode, payload.RetMsg)
	}
	if len(payload.Result.Items) == 0 {
		return 0, fmt.Errorf("Bybit P2P returned no USDT/RUB ads")
	}
	rates := make([]float64, 0, len(payload.Result.Items))
	for _, item := range payload.Result.Items {
		rate, err := strconv.ParseFloat(item.Price, 64)
		if err == nil && rate > 0 {
			rates = append(rates, rate)
		}
	}
	if len(rates) == 0 {
		return 0, fmt.Errorf("Bybit P2P returned no valid USDT/RUB prices")
	}
	// The endpoint returns the most relevant ads first. The median of their
	// valid prices is robust against an individual stale or outlier ad.
	sort.Float64s(rates)
	middle := len(rates) / 2
	if len(rates)%2 == 1 {
		return rates[middle], nil
	}
	return (rates[middle-1] + rates[middle]) / 2, nil
}

func fetchBybitP2PUSDTRUB(ctx context.Context) (float64, error) {
	body, err := common.Marshal(bybitP2PRequest{
		UserID:     "",
		TokenID:    "USDT",
		CurrencyID: "RUB",
		PaymentID:  "",
		Side:       currencyExchangeRateBybitP2PSideBuyUSDTForRUB,
		Size:       currencyExchangeRateBybitP2PRequestSize,
		Page:       "1",
		Amount:     "",
		AuthMaker:  false,
		CanTrade:   false,
	})
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, currencyExchangeRateBybitP2PURL, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := currencyExchangeRateHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Bybit P2P returned status %d", response.StatusCode)
	}
	body, err = io.ReadAll(response.Body)
	if err != nil {
		return 0, err
	}
	return parseBybitP2PUSDTRUB(body)
}

func FetchCurrencyExchangeRate(ctx context.Context, provider string) (currencyExchangeRateQuote, error) {
	base := "USD"
	if strings.TrimSpace(provider) == currencyExchangeRateProviderBybitP2P {
		base = "USDT"
	}
	return FetchCurrencyExchangeRateForPair(ctx, provider, base, "RUB")
}

// FetchCurrencyExchangeRateForPair returns the amount of quote currency for
// one unit of base currency. Providers are read-only adapters; callers decide
// whether to persist the quote.
func FetchCurrencyExchangeRateForPair(ctx context.Context, provider, baseCurrency, quoteCurrency string) (currencyExchangeRateQuote, error) {
	baseCurrency = strings.ToUpper(strings.TrimSpace(baseCurrency))
	quoteCurrency = strings.ToUpper(strings.TrimSpace(quoteCurrency))
	if baseCurrency == quoteCurrency {
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: 1}, nil
	}
	switch strings.TrimSpace(provider) {
	case currencyExchangeRateProviderCBR:
		rate, err := fetchCBRPair(ctx, baseCurrency, quoteCurrency)
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: rate}, err
	case currencyExchangeRateProviderBybitP2P:
		if baseCurrency != "USDT" || quoteCurrency != "RUB" {
			return currencyExchangeRateQuote{}, fmt.Errorf("bybit_p2p supports only USDT/RUB")
		}
		rate, err := fetchBybitP2PUSDTRUB(ctx)
		return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: rate}, err
	case currencyExchangeRateProviderCoinGecko:
		if baseCurrency == "USDT" && quoteCurrency == "USD" {
			rate, err := fetchCoinGeckoUSDTUSD(ctx)
			return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: rate}, err
		}
		if baseCurrency == "USD" && quoteCurrency == "USDT" {
			rate, err := fetchCoinGeckoUSDTUSD(ctx)
			if err != nil {
				return currencyExchangeRateQuote{}, err
			}
			return currencyExchangeRateQuote{BaseCurrency: baseCurrency, QuoteCurrency: quoteCurrency, Rate: 1 / rate}, nil
		}
		return currencyExchangeRateQuote{}, fmt.Errorf("coingecko supports only USDT/USD")
	default:
		return currencyExchangeRateQuote{}, fmt.Errorf("unsupported currency exchange rate provider %q", provider)
	}
}

// SyncPlatformCurrency records a successful quote and updates the registry's
// current rate. A failed sync keeps the last known good rate and only records
// diagnostic metadata.
func SyncPlatformCurrency(ctx context.Context, code string) error {
	currency, err := model.GetPlatformCurrency(code)
	if err != nil {
		return err
	}
	if !currency.SyncEnabled {
		return fmt.Errorf("currency %s has synchronization disabled", currency.Code)
	}
	provider := strings.TrimSpace(currency.SyncProvider)
	if provider == "" {
		return fmt.Errorf("currency %s has no synchronization provider", currency.Code)
	}
	quote, err := currencyExchangeRateFetchForPair(ctx, provider, "USD", currency.Code)
	if err != nil {
		if stateErr := model.RecordPlatformCurrencySyncError(currency.Code, provider, err.Error()); errors.Is(stateErr, model.ErrPlatformCurrencySyncConfigChanged) {
			return nil
		}
		return err
	}
	now := time.Now().UTC()
	err = model.CommitPlatformCurrencySyncQuote(currency.Code, provider, quote.Rate, now)
	if errors.Is(err, model.ErrPlatformCurrencySyncConfigChanged) {
		return nil
	}
	return err
}

// UpdatePlatformCurrencies synchronizes all enabled registry rows. One bad
// provider must not prevent other currencies from refreshing; the first error
// is returned for observability while successful rows are retained.
func UpdatePlatformCurrencies(ctx context.Context) error {
	if !platformCurrencySyncRunning.CompareAndSwap(false, true) {
		return nil
	}
	defer platformCurrencySyncRunning.Store(false)
	currencies, err := model.ListPlatformCurrencies(true)
	if err != nil {
		return err
	}
	var firstErr error
	for _, code := range platformCurrencySyncCandidates(currencies) {
		if err := SyncPlatformCurrency(ctx, code); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func platformCurrencySyncCandidates(currencies []model.PlatformCurrency) []string {
	codes := make([]string, 0, len(currencies))
	for _, currency := range currencies {
		if currency.SyncEnabled {
			codes = append(codes, currency.Code)
		}
	}
	return codes
}

// GetPlatformCurrencyRate returns 1 USD in the requested platform currency.
// A synchronized currency uses only its current committed snapshot; historical
// observations are not a fallback after the synchronization source changes.
func GetPlatformCurrencyRate(code string) (float64, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "USD" {
		return 1, nil
	}
	currency, err := model.GetPlatformCurrency(code)
	if err != nil {
		return 0, err
	}
	if !currency.Enabled {
		return 0, fmt.Errorf("currency %s is disabled", code)
	}
	if currency.SyncEnabled {
		if currency.RateToUSD > 0 && currency.LastSyncAt != nil && time.Since(*currency.LastSyncAt) <= 48*time.Hour {
			return currency.RateToUSD, nil
		}
		return 0, fmt.Errorf("currency %s has no synchronized USD rate", code)
	}
	if currency.ManualRateToUSD > 0 {
		return currency.ManualRateToUSD, nil
	}
	if currency.RateToUSD > 0 {
		return currency.RateToUSD, nil
	}
	return 0, fmt.Errorf("currency %s has no valid USD rate", code)
}

func UpdateCurrencyExchangeRate(ctx context.Context) error {
	if !currencyExchangeRateRunning.CompareAndSwap(false, true) {
		return nil
	}
	defer currencyExchangeRateRunning.Store(false)

	provider := currencyExchangeRateOption("currency_exchange_rate.provider", currencyExchangeRateProviderBybitP2P)
	quote, err := FetchCurrencyExchangeRate(ctx, provider)
	if err != nil {
		return err
	}
	return model.CreateCurrencyExchangeRate(&model.CurrencyExchangeRate{
		BaseCurrency:  quote.BaseCurrency,
		QuoteCurrency: quote.QuoteCurrency,
		Provider:      provider,
		Rate:          quote.Rate,
		RecordedAt:    time.Now().UTC(),
	})
}

// StartCurrencyExchangeRateTask synchronizes the configured USD-based platform
// currencies on the master node. The interval is read before every run so an
// option update takes effect without restarting the process.
func StartCurrencyExchangeRateTask() {
	currencyExchangeRateTaskOnce.Do(func() {
		if !common.IsMasterNode {
			return
		}
		gopool.Go(func() {
			ctx := context.Background()
			lastAttempt := time.Time{}
			ticker := time.NewTicker(currencyExchangeRateScheduleCheckInterval)
			defer ticker.Stop()
			for {
				now := time.Now()
				interval := currencyExchangeRateUpdateInterval(currencyExchangeRateOption("currency_exchange_rate.update_interval", "day"))
				if currencyExchangeRateUpdateDue(lastAttempt, now, interval) {
					lastAttempt = now
					if err := UpdatePlatformCurrencies(ctx); err != nil {
						logger.LogWarn(ctx, fmt.Sprintf("platform currency update failed: %v", err))
					}
				}
				<-ticker.C
			}
		})
	})
}

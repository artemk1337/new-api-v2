package service

import (
	"bytes"
	"context"
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
	currencyExchangeRateProviderCBR      = "cbr"
	currencyExchangeRateProviderBybitP2P = "bybit_p2p"
	currencyExchangeRateCBRURL           = "https://www.cbr-xml-daily.ru/daily_json.js"
	currencyExchangeRateBybitP2PURL      = "https://api2.bybit.com/fiat/otc/item/online"
	currencyExchangeRateTimeout          = 15 * time.Second
	// Bybit P2P API side 0 returns USDT sell ads: the price paid when buying
	// USDT for RUB. This product decision is fixed for now.
	currencyExchangeRateBybitP2PSideBuyUSDTForRUB = "0"
	currencyExchangeRateBybitP2PRequestSize       = "20"
	currencyExchangeRateScheduleCheckInterval     = time.Second
)

var (
	currencyExchangeRateHTTPClient = &http.Client{Timeout: currencyExchangeRateTimeout}
	currencyExchangeRateTaskOnce   sync.Once
	currencyExchangeRateRunning    atomic.Bool
)

type cbrDailyResponse struct {
	Valute map[string]struct {
		Nominal float64 `json:"Nominal"`
		Value   float64 `json:"Value"`
	} `json:"Valute"`
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
	usd, ok := payload.Valute["USD"]
	if !ok || usd.Nominal <= 0 || usd.Value <= 0 {
		return 0, fmt.Errorf("CBR USD quote is missing or invalid")
	}
	return usd.Value / usd.Nominal, nil
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
	switch strings.TrimSpace(provider) {
	case currencyExchangeRateProviderCBR:
		rate, err := fetchCBRUSDRUB(ctx)
		return currencyExchangeRateQuote{BaseCurrency: "USD", QuoteCurrency: "RUB", Rate: rate}, err
	case currencyExchangeRateProviderBybitP2P:
		rate, err := fetchBybitP2PUSDTRUB(ctx)
		return currencyExchangeRateQuote{BaseCurrency: "USDT", QuoteCurrency: "RUB", Rate: rate}, err
	default:
		return currencyExchangeRateQuote{}, fmt.Errorf("unsupported currency exchange rate provider %q", provider)
	}
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

// StartCurrencyExchangeRateTask records the configured exchange-rate pair only
// on the master node. The interval is read before every run so an option update
// takes effect without restarting the process.
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
					if err := UpdateCurrencyExchangeRate(ctx); err != nil {
						logger.LogWarn(ctx, fmt.Sprintf("currency exchange rate update failed: %v", err))
					}
				}
				<-ticker.C
			}
		})
	})
}

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFetchCurrencyExchangeRateFromCBR(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, currencyExchangeRateCBRURL, request.URL.String())
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"Valute":{"USD":{"Nominal":1,"Value":78.2223}}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })

	quote, err := FetchCurrencyExchangeRate(context.Background(), currencyExchangeRateProviderCBR)
	require.NoError(t, err)
	assert.Equal(t, "USD", quote.BaseCurrency)
	assert.Equal(t, "RUB", quote.QuoteCurrency)
	assert.Equal(t, 78.2223, quote.Rate)
}

func TestCurrencyExchangeRateUpdateInterval(t *testing.T) {
	assert.Equal(t, time.Minute, currencyExchangeRateUpdateInterval("minute"))
	assert.Equal(t, time.Hour, currencyExchangeRateUpdateInterval("hour"))
	assert.Equal(t, 24*time.Hour, currencyExchangeRateUpdateInterval("day"))
	assert.Equal(t, 24*time.Hour, currencyExchangeRateUpdateInterval("unexpected"))
}

func TestCurrencyExchangeRateUpdateDueUsesCurrentInterval(t *testing.T) {
	now := time.Date(2026, time.August, 1, 12, 0, 0, 0, time.UTC)
	lastAttempt := now.Add(-time.Minute)

	assert.True(t, currencyExchangeRateUpdateDue(time.Time{}, now, time.Hour))
	assert.False(t, currencyExchangeRateUpdateDue(lastAttempt, now, 24*time.Hour))
	assert.True(t, currencyExchangeRateUpdateDue(lastAttempt, now, time.Minute))
}

func TestParseBybitP2PUSDTRUBUsesMedianOfValidPrices(t *testing.T) {
	rate, err := parseBybitP2PUSDTRUB([]byte(`{
		"ret_code": 0,
		"ret_msg": "SUCCESS",
		"result": {"items": [
			{"price": "95.00"},
			{"price": "91.21"},
			{"price": "not-a-price"},
			{"price": "90.50"},
			{"price": "92.00"}
		]}
	}`))
	require.NoError(t, err)
	assert.InDelta(t, 91.605, rate, 0.000001)
}

func TestFetchCurrencyExchangeRateFromBybitP2P(t *testing.T) {
	saved := currencyExchangeRateHTTPClient
	currencyExchangeRateHTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, currencyExchangeRateBybitP2PURL, request.URL.String())
		var payload bybitP2PRequest
		require.NoError(t, common.DecodeJson(request.Body, &payload))
		assert.Equal(t, "USDT", payload.TokenID)
		assert.Equal(t, "RUB", payload.CurrencyID)
		assert.Equal(t, currencyExchangeRateBybitP2PSideBuyUSDTForRUB, payload.Side)
		assert.Equal(t, currencyExchangeRateBybitP2PRequestSize, payload.Size)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ret_code":0,"result":{"items":[{"price":"91.00"},{"price":"92.00"},{"price":"93.00"}]}}`)),
			Header:     make(http.Header),
		}, nil
	})}
	t.Cleanup(func() { currencyExchangeRateHTTPClient = saved })

	quote, err := FetchCurrencyExchangeRate(context.Background(), currencyExchangeRateProviderBybitP2P)
	require.NoError(t, err)
	assert.Equal(t, "USDT", quote.BaseCurrency)
	assert.Equal(t, "RUB", quote.QuoteCurrency)
	assert.Equal(t, 92.0, quote.Rate)
}

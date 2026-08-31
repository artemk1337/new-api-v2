package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPollDirectUSDTTRC20OnceSettlesOnlyVerifiedTransfer(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_watcher.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.PaymentMetadata{}, &model.DirectCryptoPayment{}))

	previousDB := model.DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousEndpoint := DirectUSDTTRC20TronGridBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	model.DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		DirectUSDTTRC20TronGridBaseURL = previousEndpoint
		DirectUSDTTRC20HTTPClient = previousClient
	})

	userID := 2001
	require.NoError(t, db.Create(&model.User{
		Id: userID, Username: "direct-watcher-user", Password: "password123", AffCode: "watcher-2001",
	}).Error)
	now := time.Now().Unix()
	baseUnits := uint64(10_000_000)
	tradeNo := "direct-watcher-order"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: userID, TradeNo: tradeNo, Amount: 10, RequestedAmount: 10,
		PaymentMethod: model.DirectUSDTTRC20Provider, PaymentProvider: model.DirectUSDTTRC20Provider,
		PaymentCurrency: "USD", PaymentRateToUSD: 1, PaymentCoefficient: 1,
		PaymentBaseAmount: 10, PaymentChargedAmount: 10, Money: 10, QuotaToAdd: 5_000_000,
		CreateTime: now, Status: common.TopUpStatusPending,
	}).Error)
	payment := &model.DirectCryptoPayment{
		TradeNo: tradeNo, UserId: userID, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: setting.USDTTRC20ReceivingAddress, BaseUnits: baseUnits, ExpectedUnits: baseUnits + 7,
		SuffixUnits: 7, Status: model.DirectCryptoPending, CreatedAt: now, ExpiresAt: now + 30*60, UpdatedAt: now,
	}
	require.NoError(t, db.Create(payment).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts/"+setting.USDTTRC20ReceivingAddress+"/transactions/trc20", r.URL.Path)
		assert.Equal(t, "true", r.URL.Query().Get("only_confirmed"))
		assert.Equal(t, "true", r.URL.Query().Get("only_to"))
		assert.Equal(t, setting.USDTTRC20Contract, r.URL.Query().Get("contract_address"))
		assert.Equal(t, "test-read-only-key", r.Header.Get("TRON-PRO-API-KEY"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"transaction_id":"tx-watcher","from":"TFrom","to":%q,"value":"%d","event_index":0,"block_timestamp":%d,"block_number":123,"token_info":{"address":%q},"transaction_ret":[{"contractRet":"SUCCESS"}]}],"meta":{}}`, setting.USDTTRC20ReceivingAddress, payment.ExpectedUnits, now*1000, setting.USDTTRC20Contract)
	}))
	defer server.Close()
	DirectUSDTTRC20TronGridBaseURL = server.URL
	DirectUSDTTRC20HTTPClient = server.Client()

	require.NoError(t, PollDirectUSDTTRC20Once(context.Background()))
	var settled model.DirectCryptoPayment
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&settled).Error)
	assert.Equal(t, model.DirectCryptoPaid, settled.Status)
	assert.Equal(t, "tx-watcher:0", settled.EventID)
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, 5_000_000, user.Quota)
}

func TestPollDirectUSDTTRC20OnceReconcilesSnapshotWhenDisabledAndCurrentConfigInvalid(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_watcher_disabled.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.PaymentMetadata{}, &model.DirectCryptoPayment{}))

	previousDB := model.DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousEndpoint := DirectUSDTTRC20TronGridBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	model.DB = db
	setting.USDTTRC20Enabled = false
	setting.USDTTRC20ReceivingAddress = "not-a-tron-address"
	setting.USDTTRC20APIKey = "still-usable-read-only-key"
	const snapshotAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		DirectUSDTTRC20TronGridBaseURL = previousEndpoint
		DirectUSDTTRC20HTTPClient = previousClient
	})

	userID := 2003
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "direct-watcher-disabled", Password: "password123", AffCode: "watcher-2003"}).Error)
	now := time.Now().Unix()
	tradeNo := "direct-watcher-disabled-order"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: userID, TradeNo: tradeNo, Amount: 10, RequestedAmount: 10,
		PaymentMethod: model.DirectUSDTTRC20Provider, PaymentProvider: model.DirectUSDTTRC20Provider,
		PaymentCurrency: "USD", PaymentRateToUSD: 1, PaymentCoefficient: 1,
		PaymentBaseAmount: 10, PaymentChargedAmount: 10, Money: 10, QuotaToAdd: 5_000_000,
		CreateTime: now, Status: common.TopUpStatusPending,
	}).Error)
	payment := &model.DirectCryptoPayment{
		TradeNo: tradeNo, UserId: userID, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: snapshotAddress, BaseUnits: 10_000_000, ExpectedUnits: 10_000_007,
		SuffixUnits: 7, Status: model.DirectCryptoPending, CreatedAt: now, ExpiresAt: now + 30*60, UpdatedAt: now,
	}
	require.NoError(t, db.Create(payment).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/accounts/"+snapshotAddress+"/transactions/trc20", r.URL.Path)
		assert.Equal(t, "still-usable-read-only-key", r.Header.Get("TRON-PRO-API-KEY"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"transaction_id":"tx-disabled","from":"TFrom","to":%q,"value":"%d","event_index":0,"block_timestamp":%d,"block_number":123,"token_info":{"address":%q},"transaction_ret":[{"contractRet":"SUCCESS"}]}],"meta":{}}`, snapshotAddress, payment.ExpectedUnits, now*1000, setting.USDTTRC20Contract)
	}))
	defer server.Close()
	DirectUSDTTRC20TronGridBaseURL = server.URL
	DirectUSDTTRC20HTTPClient = server.Client()

	require.NoError(t, PollDirectUSDTTRC20Once(context.Background()))
	var settled model.DirectCryptoPayment
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&settled).Error)
	assert.Equal(t, model.DirectCryptoPaid, settled.Status)
	var user model.User
	require.NoError(t, db.First(&user, userID).Error)
	assert.Equal(t, 5_000_000, user.Quota)
}

func TestPollDirectUSDTTRC20OnceReturnsProviderErrors(t *testing.T) {
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousEndpoint := DirectUSDTTRC20TronGridBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	t.Cleanup(func() {
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		DirectUSDTTRC20TronGridBaseURL = previousEndpoint
		DirectUSDTTRC20HTTPClient = previousClient
	})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporary failure", http.StatusBadGateway)
	}))
	defer server.Close()
	DirectUSDTTRC20TronGridBaseURL = server.URL
	DirectUSDTTRC20HTTPClient = server.Client()

	assert.Error(t, PollDirectUSDTTRC20Once(context.Background()))
}

func TestPollDirectUSDTTRC20OnceSkipsOldEventAndSettlesLaterEvent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_watcher_poison.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.PaymentMetadata{}, &model.DirectCryptoPayment{}))

	previousDB := model.DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousEndpoint := DirectUSDTTRC20TronGridBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	model.DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		DirectUSDTTRC20TronGridBaseURL = previousEndpoint
		DirectUSDTTRC20HTTPClient = previousClient
	})

	userID := 2002
	require.NoError(t, db.Create(&model.User{Id: userID, Username: "direct-watcher-poison", Password: "password123", AffCode: "watcher-2002"}).Error)
	now := time.Now().Unix()
	tradeNo := "direct-watcher-poison-order"
	require.NoError(t, db.Create(&model.TopUp{
		UserId: userID, TradeNo: tradeNo, Amount: 10, RequestedAmount: 10,
		PaymentMethod: model.DirectUSDTTRC20Provider, PaymentProvider: model.DirectUSDTTRC20Provider,
		PaymentCurrency: "USD", PaymentRateToUSD: 1, PaymentCoefficient: 1,
		PaymentBaseAmount: 10, PaymentChargedAmount: 10, Money: 10, QuotaToAdd: 5_000_000,
		CreateTime: now, Status: common.TopUpStatusPending,
	}).Error)
	payment := &model.DirectCryptoPayment{
		TradeNo: tradeNo, UserId: userID, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: setting.USDTTRC20ReceivingAddress, BaseUnits: 10_000_000, ExpectedUnits: 10_000_007,
		SuffixUnits: 7, Status: model.DirectCryptoPending, CreatedAt: now, ExpiresAt: now + 30*60, UpdatedAt: now,
	}
	require.NoError(t, db.Create(payment).Error)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"data":[{"transaction_id":"tx-old","from":"TFrom","to":%q,"value":"%d","event_index":0,"block_timestamp":%d,"block_number":123,"token_info":{"address":%q},"transaction_ret":[{"contractRet":"SUCCESS"}]},{"transaction_id":"tx-new","from":"TFrom","to":%q,"value":"%d","event_index":1,"block_timestamp":%d,"block_number":124,"token_info":{"address":%q},"transaction_ret":[{"contractRet":"SUCCESS"}]}],"meta":{}}`, setting.USDTTRC20ReceivingAddress, payment.ExpectedUnits, (now-1)*1000, setting.USDTTRC20Contract, setting.USDTTRC20ReceivingAddress, payment.ExpectedUnits, now*1000, setting.USDTTRC20Contract)
	}))
	defer server.Close()
	DirectUSDTTRC20TronGridBaseURL = server.URL
	DirectUSDTTRC20HTTPClient = server.Client()

	require.NoError(t, PollDirectUSDTTRC20Once(context.Background()))
	var settled model.DirectCryptoPayment
	require.NoError(t, db.Where("trade_no = ?", tradeNo).First(&settled).Error)
	assert.Equal(t, model.DirectCryptoPaid, settled.Status)
	assert.Equal(t, "tx-new:1", settled.EventID)
}

func TestFetchDirectUSDTTransfersReturnsErrorWhenPageLimitLeavesFingerprint(t *testing.T) {
	previousEndpoint := DirectUSDTTRC20TronGridBaseURL
	previousClient := DirectUSDTTRC20HTTPClient
	t.Cleanup(func() {
		DirectUSDTTRC20TronGridBaseURL = previousEndpoint
		DirectUSDTTRC20HTTPClient = previousClient
	})
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":[{"transaction_id":"tx-page","value":"1"}],"meta":{"fingerprint":"still-more"}}`)
	}))
	defer server.Close()
	DirectUSDTTRC20TronGridBaseURL = server.URL
	DirectUSDTTRC20HTTPClient = server.Client()

	_, err := fetchDirectUSDTTransfers(context.Background(), "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", "", time.Now())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pagination limit")
	assert.Equal(t, directUSDTMaxPages, requests)
}

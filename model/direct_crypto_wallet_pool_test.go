package model

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestMigrateDirectCryptoPaymentIndexesBackfillsLegacyWalletKey(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_crypto_indexes.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&DirectCryptoPayment{}))
	legacy := &DirectCryptoPayment{TradeNo: "legacy-index", UserId: 1, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract, Address: "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", ExpectedUnits: 10_000_001, BaseUnits: 10_000_000, Status: DirectCryptoExpired, ExpiresAt: 1, CreatedAt: 1}
	require.NoError(t, db.Create(legacy).Error)
	if db.Migrator().HasIndex(&DirectCryptoPayment{}, "idx_direct_crypto_payment_expected_units") {
		require.NoError(t, db.Migrator().DropIndex(&DirectCryptoPayment{}, "idx_direct_crypto_payment_expected_units"))
	}
	require.NoError(t, db.Exec("CREATE INDEX idx_direct_crypto_payment_expected_units ON direct_crypto_payments(expected_units)").Error)
	// AutoMigrate runs before the compatibility helper during startup; it must
	// tolerate the pre-fix non-unique index and let the helper repair it.
	require.NoError(t, db.AutoMigrate(&DirectCryptoPayment{}))
	require.NoError(t, MigrateDirectCryptoPaymentIndexes(db))
	var stored DirectCryptoPayment
	require.NoError(t, db.First(&stored, "trade_no = ?", legacy.TradeNo).Error)
	require.Equal(t, "TRON:TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", stored.WalletKey)
	// The historical global uniqueness guard is retained for rollback safety;
	// another wallet receives the same base amount with a different suffix.
	require.Error(t, db.Create(&DirectCryptoPayment{
		TradeNo: "same-amount-other-wallet", UserId: 2, Network: "TRON", Token: "USDT",
		Contract: setting.USDTTRC20Contract, Address: "TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj",
		WalletKey: "other-wallet", ExpectedUnits: legacy.ExpectedUnits, BaseUnits: legacy.BaseUnits,
		SuffixUnits: legacy.SuffixUnits, Status: DirectCryptoExpired, ExpiresAt: 1, CreatedAt: 1,
	}).Error)
}

func TestCreateDirectUSDTOrderUsesWalletPoolAndCentsSnapshot(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2101)
	createDirectCryptoPaymentTestUser(t, 2102)
	oldPool, oldRound, oldRead := setting.USDTReceivingWallets, setting.USDTTRC20RoundToCents, directUSDTTRC20RandRead
	setting.USDTReceivingWallets = `[{"key":"tron-a","network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},{"key":"tron-b","network":"TRON","address":"TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj"}]`
	setting.USDTTRC20RoundToCents = true
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) { buffer[0], buffer[1] = 0, 0; return len(buffer), nil }
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTRC20RoundToCents, directUSDTTRC20RandRead = oldPool, oldRound, oldRead
	})

	topUp, payment := newDirectCryptoPaymentTestOrder(2101, 10_000_001)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.Equal(t, uint64(10_000_000), payment.BaseUnits)
	require.True(t, payment.RoundToCents)
	require.Equal(t, 10.0, topUp.RequestedAmount)
	require.Equal(t, 10.0, topUp.PaymentBaseAmount)
	require.Equal(t, uint32(10_000), payment.SuffixUnits)
	require.Equal(t, "tron-a", payment.WalletKey)

	topUp2, payment2 := newDirectCryptoPaymentTestOrder(2102, 10_000_001)
	require.NoError(t, CreateDirectUSDTOrder(topUp2, payment2))
	require.Equal(t, "tron-b", payment2.WalletKey)
}

func TestDirectUSDTNetworkReadyRejectsInvalidCustomProviderEndpoint(t *testing.T) {
	oldPool, oldEndpoint, oldKey := setting.USDTReceivingWallets, setting.USDTTONAPIBaseURL, setting.USDTTONAPIKey
	setting.USDTReceivingWallets = `[{"key":"ton-1","network":"TON","address":"0:` + strings.Repeat("0", 64) + `"}]`
	setting.USDTTONAPIBaseURL = "http://toncenter.com/api/v3"
	setting.USDTTONAPIKey = "test-read-only-key"
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTONAPIBaseURL, setting.USDTTONAPIKey = oldPool, oldEndpoint, oldKey
	})

	require.False(t, DirectUSDTNetworkReady("TON"))
}

func TestCreateDirectUSDTOrderQuarantinesLegacySuffixByNetworkAndAddress(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2106)
	oldPool, oldLimit, oldRead := setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead
	address := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTReceivingWallets = `[{"key":"custom-tron","network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}]`
	setting.USDTTRC20AmountTailLimitUnits = 2
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) { buffer[0], buffer[1] = 0, 0; return len(buffer), nil }
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead = oldPool, oldLimit, oldRead
	})

	baseUnits := uint64(10_000_000)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "legacy-custom-wallet-suffix", UserId: 9999, Network: "TRON", Token: "USDT",
		Contract: setting.USDTTRC20Contract, Address: address, ExpectedUnits: baseUnits + 1,
		BaseUnits: baseUnits, SuffixUnits: 1, Status: DirectCryptoExpired,
		CreatedAt: time.Now().Add(-time.Hour).Unix(), ExpiresAt: time.Now().Add(-time.Minute).Unix(),
	}).Error)

	topUp, payment := newDirectCryptoPaymentTestOrder(2106, baseUnits)
	require.ErrorIs(t, CreateDirectUSDTOrder(topUp, payment), ErrDirectPaymentAmountExhausted)
}

func TestCreateDirectUSDTOrderFallsBackAfterFirstWalletSuffixExhaustion(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2107)
	oldPool, oldLimit, oldRead := setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead
	firstAddress := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	secondAddress := "TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj"
	setting.USDTReceivingWallets = `[{"key":"first","network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},{"key":"second","network":"TRON","address":"TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj"}]`
	setting.USDTTRC20AmountTailLimitUnits = 3
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) { buffer[0], buffer[1] = 0, 0; return len(buffer), nil }
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead = oldPool, oldLimit, oldRead
	})

	baseUnits := uint64(10_000_000)
	now := time.Now().Unix()
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "legacy-first-wallet-suffix", UserId: 9998, Network: "TRON", Token: "USDT", WalletKey: "first",
		Contract: setting.USDTTRC20Contract, Address: firstAddress, ExpectedUnits: baseUnits + 1,
		BaseUnits: baseUnits, SuffixUnits: 1, Status: DirectCryptoPending,
		CreatedAt: now, ExpiresAt: now + int64(time.Hour/time.Second),
	}).Error)

	topUp, payment := newDirectCryptoPaymentTestOrder(2107, baseUnits)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.Equal(t, "second", payment.WalletKey)
	require.Equal(t, secondAddress, payment.Address)
}

func TestCreateDirectUSDTOrderTreatsLegacyAddressAsOccupiedWallet(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2109)
	oldPool, oldLimit, oldRead := setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead
	firstAddress := "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	secondAddress := "TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj"
	setting.USDTReceivingWallets = `[{"key":"first","network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"},{"key":"second","network":"TRON","address":"TA4Y62o6YC2Zsck9rZVGTvqW1AQ7X9zTnj"}]`
	setting.USDTTRC20AmountTailLimitUnits = 3
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) { buffer[0], buffer[1] = 0, 0; return len(buffer), nil }
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTRC20AmountTailLimitUnits, directUSDTTRC20RandRead = oldPool, oldLimit, oldRead
	})

	baseUnits := uint64(10_000_000)
	now := time.Now().Unix()
	// This row predates WalletKey and must still occupy the matching address.
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "legacy-pending-address", UserId: 9997, Network: "TRON", Token: "USDT",
		Contract: setting.USDTTRC20Contract, Address: firstAddress, ExpectedUnits: baseUnits + 1,
		BaseUnits: baseUnits, SuffixUnits: 1, Status: DirectCryptoPending,
		CreatedAt: now, ExpiresAt: now + int64(time.Hour/time.Second),
	}).Error)

	topUp, payment := newDirectCryptoPaymentTestOrder(2109, baseUnits)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.Equal(t, "second", payment.WalletKey)
	require.Equal(t, secondAddress, payment.Address)
}

func TestCancelDirectUSDTOrderClosesBothRows(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2103)
	topUp, payment := newDirectCryptoPaymentTestOrder(2103, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, CancelDirectCryptoPayment(payment.TradeNo, 2103))
	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	require.Equal(t, common.TopUpStatusCancelled, storedTopUp.Status)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	require.Equal(t, DirectCryptoCancelled, storedPayment.Status)
}

func TestCancelDirectUSDTOrderDoesNotSplitSuccessfulTopUpAndInvoice(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2108)
	topUp, payment := newDirectCryptoPaymentTestOrder(2108, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", payment.TradeNo).Update("status", common.TopUpStatusSuccess).Error)

	require.ErrorIs(t, CancelDirectCryptoPayment(payment.TradeNo, 2108), ErrDirectPaymentAlreadySettled)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	require.Equal(t, DirectCryptoPending, storedPayment.Status)
}

func TestCancelledWalletReservationCanBeReusedWithAnotherSuffix(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2104)
	oldPool, oldRound, oldRead := setting.USDTReceivingWallets, setting.USDTTRC20RoundToCents, directUSDTTRC20RandRead
	setting.USDTReceivingWallets = `[{"key":"tron-a","network":"TRON","address":"TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"}]`
	setting.USDTTRC20RoundToCents = true
	sequence := 0
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		sequence++
		buffer[0], buffer[1] = byte(sequence), 0
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTReceivingWallets, setting.USDTTRC20RoundToCents, directUSDTTRC20RandRead = oldPool, oldRound, oldRead
	})

	firstTopUp, firstPayment := newDirectCryptoPaymentTestOrder(2104, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(firstTopUp, firstPayment))
	require.NoError(t, CancelDirectCryptoPayment(firstPayment.TradeNo, 2104))

	secondTopUp, secondPayment := newDirectCryptoPaymentTestOrder(2104, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(secondTopUp, secondPayment))
	require.NotEqual(t, firstPayment.ExpectedUnits, secondPayment.ExpectedUnits)
}

func TestPendingDirectUSDTOrderRejectsSameUserBaseAmountInLegacyMode(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 2105)
	firstTopUp, firstPayment := newDirectCryptoPaymentTestOrder(2105, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(firstTopUp, firstPayment))
	secondTopUp, secondPayment := newDirectCryptoPaymentTestOrder(2105, 10_000_000)
	require.ErrorIs(t, CreateDirectUSDTOrder(secondTopUp, secondPayment), ErrDirectPaymentDuplicatePending)
}

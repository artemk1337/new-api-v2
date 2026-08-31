package model

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupDirectCryptoPaymentTest(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_crypto_payment.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &PaymentMetadata{}, &DirectCryptoPayment{}))

	previousDB := DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousLimit := setting.USDTTRC20MaxCreationsPerHour
	DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	setting.USDTTRC20MaxCreationsPerHour = 0
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		setting.USDTTRC20MaxCreationsPerHour = previousLimit
	})
	return db
}

func createDirectCryptoPaymentTestUser(t *testing.T, userID int) {
	t.Helper()
	require.NoError(t, DB.Create(&User{Id: userID, Username: fmt.Sprintf("direct-payment-test-%d", userID), Password: "password123", AffCode: fmt.Sprintf("direct-%d", userID)}).Error)
}

func newDirectCryptoPaymentTestOrder(userID int, suffixBase uint64) (*TopUp, *DirectCryptoPayment) {
	now := time.Now().Unix()
	tradeNo := "direct-test-" + time.Now().Format("150405.000000000")
	topUp := &TopUp{
		UserId: userID, TradeNo: tradeNo, Amount: int64(suffixBase / 1_000_000), RequestedAmount: float64(suffixBase) / 1_000_000,
		PaymentMethod: DirectCryptoProvider, PaymentProvider: DirectCryptoProvider,
		PaymentCurrency: "USD", PaymentRateToUSD: 1, PaymentCoefficient: 1,
		PaymentBaseAmount: float64(suffixBase) / 1_000_000, PaymentChargedAmount: float64(suffixBase) / 1_000_000,
		Money: float64(suffixBase) / 1_000_000, QuotaToAdd: 5_000_000,
		CreateTime: now, Status: common.TopUpStatusPending,
	}
	payment := &DirectCryptoPayment{
		TradeNo: tradeNo, UserId: userID, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: setting.USDTTRC20ReceivingAddress, BaseUnits: suffixBase,
		Status: DirectCryptoPending, CreatedAt: now, ExpiresAt: now + int64(24*time.Hour/time.Second), UpdatedAt: now,
	}
	return topUp, payment
}

func TestCreateDirectUSDTOrderReservesRandomMicroAmount(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1001)
	topUp, payment := newDirectCryptoPaymentTestOrder(1001, 10_000_000)

	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	assert.GreaterOrEqual(t, payment.SuffixUnits, uint32(1))
	assert.LessOrEqual(t, payment.SuffixUnits, uint32(9999))
	assert.Equal(t, payment.BaseUnits+uint64(payment.SuffixUnits), payment.ExpectedUnits)
	assert.Equal(t, fmt.Sprintf("10.%06d", payment.SuffixUnits), DirectUSDTAmountString(payment.ExpectedUnits))

	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.InDelta(t, float64(payment.ExpectedUnits)/1_000_000, storedTopUp.PaymentChargedAmount, 1e-9)
}

func TestCreateDirectUSDTOrderExhaustsConfiguredSuffixRange(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1003)
	previousPrecision := setting.USDTTRC20AmountPrecision
	previousRead := directUSDTTRC20RandRead
	setting.USDTTRC20AmountPrecision = 3
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		buffer[0], buffer[1] = 0, 0
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTTRC20AmountPrecision = previousPrecision
		directUSDTTRC20RandRead = previousRead
	})

	topUp, payment := newDirectCryptoPaymentTestOrder(1003, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	topUp, payment = newDirectCryptoPaymentTestOrder(1003, 10_000_000)
	assert.ErrorIs(t, CreateDirectUSDTOrder(topUp, payment), ErrDirectPaymentAmountExhausted)
}

func TestCreateDirectUSDTOrderRejectsAbsentPersistedMethod(t *testing.T) {
	db := setupDirectCryptoPaymentTest(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"alipay"}]`}).Error)
	createDirectCryptoPaymentTestUser(t, 1009)
	topUp, payment := newDirectCryptoPaymentTestOrder(1009, 10_000_000)
	assert.ErrorIs(t, CreateDirectUSDTOrder(topUp, payment), ErrDirectPaymentDisabled)
}

func TestCreateDirectUSDTOrderUsesCanonicalMethodWithoutLegacyFlag(t *testing.T) {
	db := setupDirectCryptoPaymentTest(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"usdt_trc20_direct"}]`}).Error)
	setting.USDTTRC20Enabled = false
	createDirectCryptoPaymentTestUser(t, 1010)
	topUp, payment := newDirectCryptoPaymentTestOrder(1010, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
}

func TestCreateDirectUSDTOrderSnapshotsParentMinimumAndTTL(t *testing.T) {
	db := setupDirectCryptoPaymentTest(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"crypto_direct","min_topup":"23","pending_ttl_minutes":"41"}]`}).Error)
	createDirectCryptoPaymentTestUser(t, 1011)
	topUp, payment := newDirectCryptoPaymentTestOrder(1011, 23_000_000)

	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	assert.Equal(t, DirectCryptoProvider, topUp.PaymentMethod)
	assert.Equal(t, DirectCryptoProvider, topUp.PaymentProvider)
	assert.Equal(t, 23.0, topUp.PaymentMinimumAmount)
	assert.Equal(t, int64(41*60), topUp.PaymentPendingTTLSeconds)
	assert.Equal(t, payment.CreatedAt+int64(41*60), payment.ExpiresAt)

	belowMinimum, belowPayment := newDirectCryptoPaymentTestOrder(1011, 22_000_000)
	assert.Error(t, CreateDirectUSDTOrder(belowMinimum, belowPayment))
}

func TestCreateDirectUSDTOrderEnforcesHourlyLimitWhenConfigured(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1002)
	setting.USDTTRC20MaxCreationsPerHour = 1

	topUp, payment := newDirectCryptoPaymentTestOrder(1002, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	topUp, payment = newDirectCryptoPaymentTestOrder(1002, 11_000_000)
	assert.ErrorIs(t, CreateDirectUSDTOrder(topUp, payment), ErrDirectPaymentLimitExceeded)

	setting.USDTTRC20MaxCreationsPerHour = 0
	topUp, payment = newDirectCryptoPaymentTestOrder(1002, 11_000_000)
	assert.NoError(t, CreateDirectUSDTOrder(topUp, payment))
}

func TestCreateDirectUSDTOrderRejectsInvalidSnapshot(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1005)
	topUp, payment := newDirectCryptoPaymentTestOrder(1005, 10_000_000)

	payment.ExpiresAt = payment.CreatedAt + DirectUSDTMaxPendingSeconds + 1
	assert.Error(t, CreateDirectUSDTOrder(topUp, payment))

	payment.ExpiresAt = payment.CreatedAt + int64(24*time.Hour/time.Second)
	payment.Address = setting.USDTTRC20Contract
	assert.Error(t, CreateDirectUSDTOrder(topUp, payment))
}

func TestExpireStalePendingTopUpsExpiresDirectSnapshotsTogether(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1006)
	topUp, payment := newDirectCryptoPaymentTestOrder(1006, 10_000_000)
	createdAt := time.Now().Add(-2 * time.Hour).Unix()
	topUp.CreateTime = createdAt
	payment.CreatedAt = createdAt
	payment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	require.NoError(t, ExpireStalePendingTopUps(topUp.UserId))
	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusExpired, storedTopUp.Status)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	assert.Equal(t, DirectCryptoExpired, storedPayment.Status)
}

func TestGetDirectCryptoPaymentStatusRepairsExpiredTopUp(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1007)
	topUp, payment := newDirectCryptoPaymentTestOrder(1007, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", topUp.TradeNo).Update("status", common.TopUpStatusExpired).Error)

	status, err := GetDirectCryptoPaymentStatus(payment.TradeNo, time.Now().Unix())
	require.NoError(t, err)
	assert.Equal(t, DirectCryptoExpired, status.Status)
	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", topUp.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusExpired, storedTopUp.Status)
}

func TestGetActivePendingDirectUSDTPaymentAddressesIncludesExpiredPendingRows(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "direct-address-old", UserId: 1008, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: "ToldAddress", ExpectedUnits: 10_000_001, BaseUnits: 10_000_000, SuffixUnits: 1,
		Status: DirectCryptoPending, CreatedAt: 1, ExpiresAt: 2, UpdatedAt: 1,
	}).Error)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "direct-address-new", UserId: 1008, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: "TnewAddress", ExpectedUnits: 11_000_001, BaseUnits: 11_000_000, SuffixUnits: 1,
		Status: DirectCryptoPending, CreatedAt: 1, ExpiresAt: time.Now().Add(time.Hour).Unix(), UpdatedAt: 1,
	}).Error)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "direct-address-expired", UserId: 1008, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: "TexpiredAddress", ExpectedUnits: 12_000_001, BaseUnits: 12_000_000, SuffixUnits: 1,
		Status: DirectCryptoExpired, CreatedAt: time.Now().Add(-time.Hour).Unix(), ExpiresAt: time.Now().Add(-time.Minute).Unix(), UpdatedAt: 1,
	}).Error)

	addresses, err := GetActivePendingDirectUSDTPaymentAddresses(time.Now().Unix())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"ToldAddress", "TnewAddress", "TexpiredAddress"}, addresses)
}

func TestSettleDirectUSDTTRC20EventIsIdempotentAndRejectsDifferentEvent(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1003)
	topUp, payment := newDirectCryptoPaymentTestOrder(1003, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-a", EventIndex: "0", EventID: "tx-a:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: 19, Confirmed: true,
		BlockTimestamp: payment.CreatedAt * 1000,
	}
	require.NoError(t, SettleDirectUSDTTRC20Event(event))

	var user User
	require.NoError(t, DB.First(&user, 1003).Error)
	assert.Equal(t, topUp.QuotaToAdd, user.Quota)
	require.NoError(t, SettleDirectUSDTTRC20Event(event))
	require.NoError(t, DB.First(&user, 1003).Error)
	assert.Equal(t, topUp.QuotaToAdd, user.Quota)

	differentEvent := event
	differentEvent.TxHash = "tx-b"
	differentEvent.EventID = "tx-b:0"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(differentEvent), ErrDirectPaymentAlreadySettled)
	require.NoError(t, DB.First(&user, 1003).Error)
	assert.Equal(t, topUp.QuotaToAdd, user.Quota)
}

func TestSettleDirectUSDTTRC20EventRejectsUnconfirmedWrongAssetAndAmount(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1004)
	topUp, payment := newDirectCryptoPaymentTestOrder(1004, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	baseEvent := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-invalid", EventIndex: "0", EventID: "tx-invalid:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: 0, Confirmed: false,
		BlockTimestamp: payment.CreatedAt * 1000,
	}
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(baseEvent), ErrDirectPaymentInvalid)

	wrongAmount := baseEvent
	wrongAmount.Confirmed = true
	wrongAmount.Confirmations = uint64(setting.USDTTRC20MinConfirmations)
	wrongAmount.AmountUnits++
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(wrongAmount), ErrDirectPaymentAmountMismatch)

	wrongContract := baseEvent
	wrongContract.Confirmed = true
	wrongContract.Confirmations = uint64(setting.USDTTRC20MinConfirmations)
	wrongContract.Contract = "TWrongContract"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(wrongContract), ErrDirectPaymentInvalid)

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
}

func TestSettleDirectUSDTTRC20EventUsesBlockTimestampAfterLocalExpiry(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1009)
	topUp, payment := newDirectCryptoPaymentTestOrder(1009, 10_000_000)
	createdAt := time.Now().Add(-time.Hour).Unix()
	topUp.CreateTime = createdAt
	payment.CreatedAt = createdAt
	payment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-late-observed", EventIndex: "0", EventID: "tx-late-observed:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		// The watcher sees this event after ExpiresAt, but the block itself was
		// mined inside the immutable order window.
		BlockTimestamp: (payment.ExpiresAt - 1) * 1000,
	}
	require.NoError(t, SettleDirectUSDTTRC20Event(event))

	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	assert.Equal(t, DirectCryptoPaid, storedPayment.Status)
	var user User
	require.NoError(t, DB.First(&user, 1009).Error)
	assert.Equal(t, storedTopUp.QuotaToAdd, user.Quota)
}

func TestSettleDirectUSDTTRC20EventCanReviveExpiredSnapshotOnlyForPreExpiryEvent(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1010)
	topUp, payment := newDirectCryptoPaymentTestOrder(1010, 10_000_000)
	createdAt := time.Now().Add(-2 * time.Hour).Unix()
	topUp.CreateTime = createdAt
	payment.CreatedAt = createdAt
	payment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, DB.Model(&TopUp{}).Where("trade_no = ?", payment.TradeNo).Update("status", common.TopUpStatusExpired).Error)
	require.NoError(t, DB.Model(&DirectCryptoPayment{}).Where("trade_no = ?", payment.TradeNo).Update("status", DirectCryptoExpired).Error)

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-expired-snapshot", EventIndex: "0", EventID: "tx-expired-snapshot:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: (payment.CreatedAt + 10) * 1000,
	}
	require.NoError(t, SettleDirectUSDTTRC20Event(event))

	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusSuccess, storedTopUp.Status)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	assert.Equal(t, DirectCryptoPaid, storedPayment.Status)
}

func TestSettleDirectUSDTTRC20EventExpiresBothSnapshotsForPostExpiryEvent(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1011)
	topUp, payment := newDirectCryptoPaymentTestOrder(1011, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-post-expiry", EventIndex: "0", EventID: "tx-post-expiry:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: (payment.ExpiresAt + 1) * 1000,
	}
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(event), ErrDirectPaymentExpired)

	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusExpired, storedTopUp.Status)
	var storedPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedPayment).Error)
	assert.Equal(t, DirectCryptoExpired, storedPayment.Status)
	var user User
	require.NoError(t, DB.First(&user, 1011).Error)
	assert.Zero(t, user.Quota)
}

func TestManualCompleteTopUpRejectsDirectUSDTWithoutVerifiedEvent(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1012)
	topUp, payment := newDirectCryptoPaymentTestOrder(1012, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	assert.ErrorIs(t, ManualCompleteTopUp(payment.TradeNo, "127.0.0.1"), ErrDirectPaymentInvalid)
	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.Equal(t, common.TopUpStatusPending, storedTopUp.Status)
	var user User
	require.NoError(t, DB.First(&user, 1012).Error)
	assert.Zero(t, user.Quota)
}

func TestDirectUSDTAmountString(t *testing.T) {
	assert.Equal(t, "10.000001", DirectUSDTAmountString(10_000_001))
	assert.Equal(t, "123.456789", DirectUSDTAmountString(123_456_789))
}

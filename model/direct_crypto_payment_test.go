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
	require.NoError(t, db.AutoMigrate(&User{}, &TopUp{}, &PaymentMetadata{}, &DirectCryptoPayment{}, &DirectCryptoReconciliation{}))

	previousDB := DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousLimit := setting.USDTTRC20MaxCreationsPerHour
	previousRoundToCents := setting.USDTTRC20RoundToCents
	previousWallets := setting.USDTReceivingWallets
	DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	setting.USDTTRC20MaxCreationsPerHour = 0
	setting.USDTTRC20RoundToCents = false
	setting.USDTReceivingWallets = ""
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		setting.USDTTRC20MaxCreationsPerHour = previousLimit
		setting.USDTTRC20RoundToCents = previousRoundToCents
		setting.USDTReceivingWallets = previousWallets
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
	payment.Network = "tron"

	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	assert.Equal(t, "TRON", payment.Network)
	assert.GreaterOrEqual(t, payment.SuffixUnits, uint32(1))
	assert.LessOrEqual(t, payment.SuffixUnits, uint32(9999))
	assert.True(t, payment.RoundPolicyCaptured)
	assert.Equal(t, payment.BaseUnits+uint64(payment.SuffixUnits), payment.ExpectedUnits)
	assert.Equal(t, fmt.Sprintf("10.%06d", payment.SuffixUnits), DirectUSDTAmountString(payment.ExpectedUnits))

	var storedTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&storedTopUp).Error)
	assert.InDelta(t, float64(payment.ExpectedUnits)/1_000_000, storedTopUp.PaymentChargedAmount, 1e-9)
}

func TestDirectUSDTEventDestinationUsesImmutableDestination(t *testing.T) {
	payment := DirectCryptoPayment{Address: "wallet-owner", Destination: "token-account"}
	assert.Equal(t, "token-account", directUSDTEventDestination(payment))

	payment.Destination = ""
	assert.Equal(t, "wallet-owner", directUSDTEventDestination(payment))
}

func TestCreateDirectUSDTOrderExhaustsConfiguredSuffixRange(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1003)
	createDirectCryptoPaymentTestUser(t, 1004)
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousRead := directUSDTTRC20RandRead
	setting.USDTTRC20AmountTailLimitUnits = 2
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		buffer[0], buffer[1] = 0, 0
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		directUSDTTRC20RandRead = previousRead
	})

	topUp, payment := newDirectCryptoPaymentTestOrder(1003, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	topUp, payment = newDirectCryptoPaymentTestOrder(1004, 10_000_000)
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

func TestCreateDirectUSDTOrderClampsInputExpiryAfterPolicySnapshot(t *testing.T) {
	db := setupDirectCryptoPaymentTest(t)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"crypto_direct","pending_ttl_minutes":"2880"}]`}).Error)
	createDirectCryptoPaymentTestUser(t, 1012)
	topUp, payment := newDirectCryptoPaymentTestOrder(1012, 10_000_000)
	payment.ExpiresAt = payment.CreatedAt + int64(48*time.Hour/time.Second)

	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.Equal(t, int64(24*time.Hour/time.Second), topUp.PaymentPendingTTLSeconds)
	require.Equal(t, payment.CreatedAt+int64(24*time.Hour/time.Second), payment.ExpiresAt)
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

	payment.ExpiresAt = payment.CreatedAt + DirectUSDTLegacyMaxPendingSeconds + 1
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

func TestCreateDirectUSDTOrderExpiresStalePendingBeforeDuplicateCheck(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1014)
	oldTopUp, oldPayment := newDirectCryptoPaymentTestOrder(1014, 10_000_000)
	createdAt := time.Now().Add(-2 * time.Hour).Unix()
	oldTopUp.CreateTime = createdAt
	oldPayment.CreatedAt = createdAt
	oldPayment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(oldTopUp, oldPayment))

	newTopUp, newPayment := newDirectCryptoPaymentTestOrder(1014, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(newTopUp, newPayment))

	var storedOldTopUp TopUp
	require.NoError(t, DB.Where("trade_no = ?", oldTopUp.TradeNo).First(&storedOldTopUp).Error)
	assert.Equal(t, common.TopUpStatusExpired, storedOldTopUp.Status)
	var storedOldPayment DirectCryptoPayment
	require.NoError(t, DB.Where("trade_no = ?", oldPayment.TradeNo).First(&storedOldPayment).Error)
	assert.Equal(t, DirectCryptoExpired, storedOldPayment.Status)
	assert.NotEqual(t, oldPayment.TradeNo, newPayment.TradeNo)
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

func TestGetActivePendingDirectUSDTPaymentAddressesIncludesReconciliationRows(t *testing.T) {
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
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "direct-address-cancelled", UserId: 1008, Network: "TRON", Token: "USDT", Contract: setting.USDTTRC20Contract,
		Address: "TcancelledAddress", ExpectedUnits: 13_000_001, BaseUnits: 13_000_000, SuffixUnits: 1,
		Status: DirectCryptoCancelled, CreatedAt: time.Now().Add(-time.Hour).Unix(), ExpiresAt: time.Now().Add(-time.Minute).Unix(), UpdatedAt: 1,
	}).Error)

	addresses, err := GetActivePendingDirectUSDTPaymentAddresses(time.Now().Unix())
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"TnewAddress", "TexpiredAddress", "TcancelledAddress"}, addresses)
	pending, err := GetPendingDirectUSDTPayments(10_000_001, "ToldAddress")
	require.NoError(t, err)
	assert.Empty(t, pending)
	cancelled, err := GetCancelledDirectUSDTPayments(13_000_001, "TcancelledAddress")
	require.NoError(t, err)
	assert.Len(t, cancelled, 1)
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

	tamperedEvent := event
	tamperedEvent.To = "TWrongDestination"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(tamperedEvent), ErrDirectPaymentInvalid)
	tamperedEvent = event
	tamperedEvent.AmountUnits++
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(tamperedEvent), ErrDirectPaymentAmountMismatch)
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

	missingEventIndex := baseEvent
	missingEventIndex.Confirmed = true
	missingEventIndex.Confirmations = uint64(setting.USDTTRC20MinConfirmations)
	missingEventIndex.EventIndex = ""
	missingEventIndex.EventID = "tx-invalid:"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(missingEventIndex), ErrDirectPaymentInvalid)

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
	createdAt := time.Now().Add(-time.Hour).Unix()
	topUp.CreateTime = createdAt
	payment.CreatedAt = createdAt
	payment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-post-expiry", EventIndex: "0", EventID: "tx-post-expiry:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		// The deadline is exclusive: a block at ExpiresAt is already late.
		BlockTimestamp: payment.ExpiresAt * 1000,
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
	metadata, metadataErr := GetPaymentMetadataByExternalPaymentIDWithError(DirectCryptoProvider, event.EventID)
	require.NoError(t, metadataErr)
	assert.Contains(t, metadata.Metadata, `"credited":false`)
}

func TestSettleDirectUSDTTRC20EventRejectsFutureBlockTimestamp(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1015)
	topUp, payment := newDirectCryptoPaymentTestOrder(1015, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))

	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, TxHash: "tx-future", EventIndex: "0", EventID: "tx-future:0",
		Contract: setting.USDTTRC20Contract, To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: (time.Now().Unix() + 60) * 1000,
	}
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(event), ErrDirectPaymentInvalid)

	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusPending, stored.Status)
}

func TestRecordDirectUSDTReconciliationEventIsIdempotent(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	address := setting.USDTTRC20ReceivingAddress
	event := DirectUSDTTransferEvent{
		Network: "TRON", TxHash: "tx-orphan", EventIndex: "0", EventID: "tx-orphan:0",
		Contract: setting.USDTTRC20Contract, Source: "TFrom", To: address,
		AmountUnits: 10_000_123, Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: time.Now().Add(-time.Minute).UnixMilli(),
	}
	require.NoError(t, RecordDirectUSDTReconciliationEvent(event, DirectCryptoReconciliationOrphan, "", 0))
	require.NoError(t, RecordDirectUSDTReconciliationEvent(event, DirectCryptoReconciliationOrphan, "", 0))
	var records []DirectCryptoReconciliation
	require.NoError(t, DB.Find(&records).Error)
	require.Len(t, records, 1)
	assert.Equal(t, "TFrom", records[0].Source)
	assert.Equal(t, int64(event.BlockTimestamp/1000), records[0].BlockTimestamp)
	claimed := event
	claimed.TradeNo = "later-invoice"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(claimed), ErrDirectPaymentEventAlreadyUsed)
}

func TestDirectUSDTEventCannotBeBothSettledAndReconciled(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1018)
	topUp, payment := newDirectCryptoPaymentTestOrder(1018, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	// Exercise the persistent decision lock used by multi-instance databases.
	require.NoError(t, DB.AutoMigrate(&Option{}))
	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, Network: "TRON", TxHash: "tx-settled-reconciliation", EventIndex: "0",
		EventID: "tx-settled-reconciliation:0", Contract: setting.USDTTRC20Contract,
		To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: payment.CreatedAt * 1000,
	}
	require.NoError(t, SettleDirectUSDTTRC20Event(event))
	require.NoError(t, RecordDirectUSDTReconciliationEvent(event, DirectCryptoReconciliationOrphan, "", 0))
	var count int64
	require.NoError(t, DB.Model(&DirectCryptoReconciliation{}).Where("event_id = ?", event.EventID).Count(&count).Error)
	assert.Zero(t, count)
}

func TestDirectUSDTReconciledEventCannotSettle(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1019)
	topUp, payment := newDirectCryptoPaymentTestOrder(1019, 10_000_000)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, DB.AutoMigrate(&Option{}))
	event := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, Network: "TRON", TxHash: "tx-reconciled-settlement", EventIndex: "0",
		EventID: "tx-reconciled-settlement:0", Contract: setting.USDTTRC20Contract,
		To: payment.Address, AmountUnits: payment.ExpectedUnits,
		Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: payment.CreatedAt * 1000,
	}
	require.NoError(t, RecordDirectUSDTReconciliationEvent(event, DirectCryptoReconciliationOrphan, "", 0))
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(event), ErrDirectPaymentEventAlreadyUsed)
	var user User
	require.NoError(t, DB.First(&user, 1019).Error)
	assert.Zero(t, user.Quota)
}

func TestRecordDirectUSDTReconciliationEventRejectsFutureTimestamp(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	event := DirectUSDTTransferEvent{
		Network: "TRON", TxHash: "tx-future-reconciliation", EventIndex: "0", EventID: "tx-future-reconciliation:0",
		Contract: setting.USDTTRC20Contract, To: setting.USDTTRC20ReceivingAddress,
		AmountUnits: 10_000_123, Confirmations: uint64(setting.USDTTRC20MinConfirmations), Confirmed: true,
		BlockTimestamp: time.Now().Add(time.Minute).UnixMilli(),
	}
	assert.ErrorIs(t, RecordDirectUSDTReconciliationEvent(event, DirectCryptoReconciliationOrphan, "", 0), ErrDirectPaymentInvalid)
}

func TestSettleDirectUSDTTRC20EventKeepsMultipleLateEvents(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1013)
	topUp, payment := newDirectCryptoPaymentTestOrder(1013, 10_000_000)
	createdAt := time.Now().Add(-2 * time.Hour).Unix()
	topUp.CreateTime = createdAt
	payment.CreatedAt = createdAt
	payment.ExpiresAt = createdAt + int64(30*time.Minute/time.Second)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.NoError(t, CancelDirectCryptoPayment(payment.TradeNo, payment.UserId))

	baseEvent := DirectUSDTTransferEvent{
		TradeNo: payment.TradeNo, Contract: setting.USDTTRC20Contract, To: payment.Address,
		AmountUnits: payment.ExpectedUnits, Confirmations: uint64(setting.USDTTRC20MinConfirmations),
		Confirmed: true, BlockTimestamp: payment.CreatedAt * 1000,
	}
	wrongAmount := baseEvent
	wrongAmount.TxHash, wrongAmount.EventIndex, wrongAmount.EventID = "tx-cancelled-wrong-amount", "0", "tx-cancelled-wrong-amount:0"
	wrongAmount.AmountUnits++
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(wrongAmount), ErrDirectPaymentAmountMismatch)
	_, err := GetPaymentMetadataByExternalPaymentIDWithError(DirectCryptoProvider, wrongAmount.EventID)
	assert.ErrorIs(t, err, gorm.ErrRecordNotFound)

	first := baseEvent
	first.TxHash, first.EventIndex, first.EventID = "tx-cancelled-1", "0", "tx-cancelled-1:0"
	second := baseEvent
	second.TxHash, second.EventIndex, second.EventID = "tx-cancelled-2", "0", "tx-cancelled-2:0"
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(first), ErrDirectPaymentExpired)
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(second), ErrDirectPaymentExpired)
	postExpiry := baseEvent
	postExpiry.TxHash, postExpiry.EventIndex, postExpiry.EventID = "tx-cancelled-post-expiry", "0", "tx-cancelled-post-expiry:0"
	postExpiry.BlockTimestamp = (payment.ExpiresAt + 60) * 1000
	assert.ErrorIs(t, SettleDirectUSDTTRC20Event(postExpiry), ErrDirectPaymentExpired)

	metadata, err := GetPaymentMetadataByExternalPaymentIDWithError(DirectCryptoProvider, first.EventID)
	require.NoError(t, err)
	var payload map[string]interface{}
	require.NoError(t, common.Unmarshal([]byte(metadata.Metadata), &payload))
	lateEvents, ok := payload["late_events"].([]interface{})
	require.True(t, ok)
	assert.Len(t, lateEvents, 3)
	var stored TopUp
	require.NoError(t, DB.Where("trade_no = ?", payment.TradeNo).First(&stored).Error)
	assert.Equal(t, common.TopUpStatusCancelled, stored.Status)
	var user User
	require.NoError(t, DB.First(&user, 1013).Error)
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

package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestRandomDirectUSDTTRC20SuffixUsesConfiguredInclusiveRange(t *testing.T) {
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousRead := directUSDTTRC20RandRead
	setting.USDTTRC20AmountTailLimitUnits = 10
	reads := 0
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		reads++
		buffer[0], buffer[1] = 0, 8
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		directUSDTTRC20RandRead = previousRead
	})

	suffix, err := randomDirectUSDTTRC20Suffix()
	require.NoError(t, err)
	require.Equal(t, uint32(9), suffix)
	require.Equal(t, 1, reads)
}

func TestRandomDirectUSDTTRC20SuffixRejectsInvalidRuntimeLimit(t *testing.T) {
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	setting.USDTTRC20AmountTailLimitUnits = 1
	t.Cleanup(func() {
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
	})

	_, err := randomDirectUSDTTRC20Suffix()
	require.Error(t, err)
}

func TestRandomDirectUSDTTRC20SuffixIgnoresLegacyBounds(t *testing.T) {
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousMin := setting.USDTTRC20AmountSuffixMinUnits
	previousMax := setting.USDTTRC20AmountSuffixMaxUnits
	previousRead := directUSDTTRC20RandRead
	setting.USDTTRC20AmountTailLimitUnits = 100
	setting.USDTTRC20AmountSuffixMinUnits = 9999
	setting.USDTTRC20AmountSuffixMaxUnits = 9999
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		buffer[0], buffer[1] = 0, 0
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		setting.USDTTRC20AmountSuffixMinUnits = previousMin
		setting.USDTTRC20AmountSuffixMaxUnits = previousMax
		directUSDTTRC20RandRead = previousRead
	})

	suffix, err := randomDirectUSDTTRC20Suffix()
	require.NoError(t, err)
	require.Equal(t, uint32(1), suffix)
}

func TestCreateDirectUSDTOrderIgnoresHistoricalSuffixesOutsideConfiguredRange(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1030)
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousRead := directUSDTTRC20RandRead
	setting.USDTTRC20AmountTailLimitUnits = 10
	directUSDTTRC20RandRead = func(buffer []byte) (int, error) {
		buffer[0], buffer[1] = 0, 0
		return len(buffer), nil
	}
	t.Cleanup(func() {
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		directUSDTTRC20RandRead = previousRead
	})

	baseUnits := uint64(10_000_000)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "historical-outside-range", UserId: 1030, Network: "TRON", Token: "USDT",
		Contract: setting.USDTTRC20Contract, Address: setting.USDTTRC20ReceivingAddress,
		ExpectedUnits: baseUnits + 10, BaseUnits: baseUnits, SuffixUnits: 10,
		Status: DirectCryptoPending, CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}).Error)

	topUp, payment := newDirectCryptoPaymentTestOrder(1030, baseUnits)
	require.NoError(t, CreateDirectUSDTOrder(topUp, payment))
	require.Equal(t, uint32(1), payment.SuffixUnits)
}

func TestCreateDirectUSDTOrderFailsClosedWhenConfiguredSuffixIsOccupied(t *testing.T) {
	setupDirectCryptoPaymentTest(t)
	createDirectCryptoPaymentTestUser(t, 1031)
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

	baseUnits := uint64(10_000_000)
	require.NoError(t, DB.Create(&DirectCryptoPayment{
		TradeNo: "historical-in-range", UserId: 1031, Network: "TRON", Token: "USDT",
		Contract: setting.USDTTRC20Contract, Address: setting.USDTTRC20ReceivingAddress,
		ExpectedUnits: baseUnits + 1, BaseUnits: baseUnits, SuffixUnits: 1,
		Status: DirectCryptoPending, CreatedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix(),
	}).Error)

	topUp, payment := newDirectCryptoPaymentTestOrder(1031, baseUnits)
	require.ErrorIs(t, CreateDirectUSDTOrder(topUp, payment), ErrDirectPaymentAmountExhausted)
}

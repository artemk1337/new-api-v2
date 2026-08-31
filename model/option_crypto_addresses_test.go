package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCryptoReceivingAddressesPersistAndLoadPerNetwork(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	previousDB := DB
	previousMap := common.OptionMap
	previousTron := setting.USDTTRC20ReceivingAddress
	previousTon := setting.USDTTONReceivingAddress
	previousSolana := setting.USDTSolanaReceivingAddress
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousPrecision := setting.USDTTRC20AmountPrecision
	previousSuffixMin := setting.USDTTRC20AmountSuffixMinUnits
	previousSuffixMax := setting.USDTTRC20AmountSuffixMaxUnits
	DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20ReceivingAddress = previousTron
		setting.USDTTONReceivingAddress = previousTon
		setting.USDTSolanaReceivingAddress = previousSolana
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		setting.USDTTRC20AmountPrecision = previousPrecision
		setting.USDTTRC20AmountSuffixMinUnits = previousSuffixMin
		setting.USDTTRC20AmountSuffixMaxUnits = previousSuffixMax
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})

	values := map[string]string{
		"USDTTRC20ReceivingAddress":     " TJRabPrwbZy45sbavfcjinPJC18kjpRTv8 ",
		"USDTTONReceivingAddress":       " 0:" + strings.Repeat("1", 64) + " ",
		"USDTSolanaReceivingAddress":    " 11111111111111111111111111111111 ",
		"USDTTRC20AmountTailLimitUnits": " 100 ",
	}
	require.NoError(t, UpdateOptionsBulk(values))

	require.Equal(t, "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8", setting.USDTTRC20ReceivingAddress)
	require.Equal(t, "0:"+strings.Repeat("1", 64), setting.USDTTONReceivingAddress)
	require.Equal(t, "11111111111111111111111111111111", setting.USDTSolanaReceivingAddress)
	require.Equal(t, 100, setting.USDTTRC20AmountTailLimitUnits)
	for key, expected := range map[string]string{
		"USDTTRC20ReceivingAddress":     "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8",
		"USDTTONReceivingAddress":       "0:" + strings.Repeat("1", 64),
		"USDTSolanaReceivingAddress":    "11111111111111111111111111111111",
		"USDTTRC20AmountTailLimitUnits": "100",
	} {
		var option Option
		require.NoError(t, db.First(&option, "key = ?", key).Error)
		require.Equal(t, expected, option.Value)
	}
}

func TestUpdateOptionValidatesPayMethodTopupGroupAgainstPersistedCatalog(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	previousRatios := common.TopupGroupRatio2JSONString()
	previousMethods := operation_setting.PayMethods
	DB = db
	require.NoError(t, common.UpdateTopupGroupRatioByJSONString(`{"default":1,"premium":1.2}`))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"alipay"}]`}).Error)
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		require.NoError(t, common.UpdateTopupGroupRatioByJSONString(previousRatios))
	})

	// Existing legacy method without the field remains accepted.
	require.NoError(t, UpdateOption("PayMethods", `[{"type":"alipay"}]`))
	// A newly introduced method cannot inherit the legacy omission.
	require.Error(t, UpdateOption("PayMethods", `[{"type":"alipay"},{"type":"stripe"}]`))
	// Once an explicit group is set it cannot be silently removed.
	require.NoError(t, UpdateOption("PayMethods", `[{"type":"alipay","topup_group":"default"}]`))
	require.Error(t, UpdateOption("PayMethods", `[{"type":"alipay"}]`))
	// Explicit but unknown group is never accepted.
	require.Error(t, UpdateOption("PayMethods", `[{"type":"alipay","topup_group":"missing"}]`))
}

func TestCryptoAmountSuffixOptionsRejectInvalidRanges(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	require.Error(t, validateDirectUSDTAmountSuffixOptionValuesFromDB(db, map[string]string{
		"USDTTRC20AmountSuffixMinUnits": "500",
		"USDTTRC20AmountSuffixMaxUnits": "499",
	}))
	require.Error(t, validateDirectUSDTAmountSuffixOptionValuesFromDB(db, map[string]string{
		"USDTTRC20AmountSuffixMinUnits": "0",
		"USDTTRC20AmountSuffixMaxUnits": "10000",
	}))
	require.Error(t, validateDirectUSDTAmountSuffixOptionValuesFromDB(db, map[string]string{
		"USDTTRC20AmountSuffixMinUnits": "0",
		"USDTTRC20AmountSuffixMaxUnits": "1",
	}))
}

func TestCryptoAmountTailLimitOptionValidationAndPersistence(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	previousLimit := setting.USDTTRC20AmountTailLimitUnits
	previousMap := common.OptionMap
	DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20AmountTailLimitUnits = previousLimit
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})

	require.NoError(t, UpdateOption("USDTTRC20AmountTailLimitUnits", " 100 "))
	require.Equal(t, 100, setting.USDTTRC20AmountTailLimitUnits)
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "USDTTRC20AmountTailLimitUnits").Error)
	require.Equal(t, "100", option.Value)
	require.Error(t, UpdateOption("USDTTRC20AmountTailLimitUnits", "1"))
	require.Error(t, UpdateOption("USDTTRC20AmountTailLimitUnits", "10001"))
}

func TestCryptoAmountSuffixOptionsRejectInvalidBulkUpdate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	DB = db
	t.Cleanup(func() { DB = previousDB })

	err = UpdateOptionsBulk(map[string]string{
		"USDTTRC20AmountSuffixMinUnits": "9000",
		"USDTTRC20AmountSuffixMaxUnits": "8999",
	})
	require.Error(t, err)
	var option Option
	require.ErrorIs(t, db.First(&option, "key = ?", "USDTTRC20AmountSuffixMinUnits").Error, gorm.ErrRecordNotFound)
}

func TestCryptoAmountSuffixBootstrapMaterializesPersistedCounterpart(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	previousMap := common.OptionMap
	previousMin := setting.USDTTRC20AmountSuffixMinUnits
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20AmountSuffixMinUnits = previousMin
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousMap
		common.OptionMapRWMutex.Unlock()
	})
	DB = db
	common.OptionMapRWMutex.Lock()
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()

	require.NoError(t, persistOptionsAndRuntimeWithTxGuard(map[string]string{
		"USDTTRC20AmountSuffixMinUnits": "7",
	}, nil, nil))
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "USDTTRC20AmountSuffixMaxUnits").Error)
	require.Equal(t, "9999", option.Value)
}

func TestCryptoAmountSuffixSingleUpdateUsesPersistedCounterpart(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "USDTTRC20AmountSuffixMinUnits", Value: "1"}).Error)
	require.NoError(t, db.Create(&Option{Key: "USDTTRC20AmountSuffixMaxUnits", Value: "100"}).Error)
	previousDB := DB
	previousMin := setting.USDTTRC20AmountSuffixMinUnits
	previousMax := setting.USDTTRC20AmountSuffixMaxUnits
	DB = db
	// Simulate a stale process snapshot: it still believes max=9999 while the
	// database has already been narrowed to 100 by another instance.
	setting.USDTTRC20AmountSuffixMinUnits = 1
	setting.USDTTRC20AmountSuffixMaxUnits = 9999
	t.Cleanup(func() {
		DB = previousDB
		setting.USDTTRC20AmountSuffixMinUnits = previousMin
		setting.USDTTRC20AmountSuffixMaxUnits = previousMax
	})

	require.Error(t, UpdateOption("USDTTRC20AmountSuffixMinUnits", "200"))
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "USDTTRC20AmountSuffixMinUnits").Error)
	require.Equal(t, "1", option.Value)
}

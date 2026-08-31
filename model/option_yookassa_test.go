package model

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetPayMethodsFromDBTreatsMissingOptionsTableAsBootstrap(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	methods, err := GetPayMethodsFromDB(db)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, methods)
}

func TestGetPayMethodsFromDBPreservesUnavailableDatabaseError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	methods, err := GetPayMethodsFromDB(db)
	require.Error(t, err)
	require.NotErrorIs(t, err, gorm.ErrRecordNotFound)
	require.Nil(t, methods)
}

func TestLegacyDirectUSDTConfigMigratesToCanonicalPayMethod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "custom1"}}
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "read-only-key"
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, ensureYooKassaPayMethodPersisted())
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "PayMethods").Error)
	var methods []map[string]string
	require.NoError(t, common.Unmarshal([]byte(option.Value), &methods))
	require.True(t, HasDirectUSDTMethod(methods))
	var marker Option
	require.NoError(t, db.First(&marker, "key = ?", operation_setting.DirectUSDTTRC20PayMethodsMigratedOption).Error)
}

func TestLegacyDirectUSDTConfigDoesNotMigrateWhenInvalid(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "custom1"}}
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "invalid"
	setting.USDTTRC20APIKey = "read-only-key"
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, ensureYooKassaPayMethodPersisted())
	var option Option
	require.ErrorIs(t, db.First(&option, "key = ?", "PayMethods").Error, gorm.ErrRecordNotFound)
}

func TestEnsureYooKassaPayMethodPersistsMigration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousYooKassaConfig := setting.GetYooKassaConfig()
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.PublishYooKassaConfig(previousYooKassaConfig)
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"name": "Custom", "type": "custom1"}}
	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, ensureYooKassaPayMethodPersisted())
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "PayMethods").Error)
	var methods []map[string]string
	require.NoError(t, common.Unmarshal([]byte(option.Value), &methods))
	require.Len(t, methods, 2)
	require.Equal(t, operation_setting.YooKassaSBPPaymentMethod, methods[1]["type"])
	_, hasCurrency := methods[1]["currency"]
	require.False(t, hasCurrency)
	require.Equal(t, "default", methods[1]["topup_group"])

	require.NoError(t, ensureYooKassaPayMethodPersisted())
	var count int64
	require.NoError(t, db.Model(&Option{}).Where("key = ?", "PayMethods").Count(&count).Error)
	require.EqualValues(t, 1, count)
	require.NoError(t, db.First(&option, "key = ?", "PayMethods").Error)
	require.NoError(t, common.Unmarshal([]byte(option.Value), &methods))
	require.Len(t, methods, 2)
}

func TestUpdateOptionsBulkKeepsExplicitPayMethodsWithoutYooKassa(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"custom"},{"type":"yookassa_sbp"}]`}).Error)

	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousYooKassaConfig := setting.GetYooKassaConfig()
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	common.OptionMapRWMutex.Lock()
	previousOptionMap := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.PublishYooKassaConfig(previousYooKassaConfig)
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
		common.OptionMapRWMutex.Lock()
		common.OptionMap = previousOptionMap
		common.OptionMapRWMutex.Unlock()
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "custom"}, {"type": operation_setting.YooKassaSBPPaymentMethod}}
	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, UpdateOptionsBulk(map[string]string{
		"PayMethods": `[{"type":"custom"}]`,
	}))

	var option Option
	require.NoError(t, db.First(&option, "key = ?", "PayMethods").Error)
	var methods []map[string]string
	require.NoError(t, common.Unmarshal([]byte(option.Value), &methods))
	require.Len(t, methods, 1)
	require.Equal(t, "custom", methods[0]["type"])
	require.Len(t, operation_setting.PayMethods, 1)
	require.Equal(t, "custom", operation_setting.PayMethods[0]["type"])
}

func TestUpdateOptionsBulkRollsBackWhenPayMethodMigrationFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"custom"}]`}).Error)

	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousYooKassaConfig := setting.GetYooKassaConfig()
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.PublishYooKassaConfig(previousYooKassaConfig)
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "custom"}}
	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("test:fail-paymethods", func(tx *gorm.DB) {
		option, ok := tx.Statement.Dest.(*Option)
		if ok && option.Key == "PayMethods" {
			tx.AddError(errors.New("pay method persistence failure"))
		}
	}))

	require.Error(t, UpdateOptionsBulk(map[string]string{"MinTopUp": "12"}))
	var option Option
	require.ErrorIs(t, db.First(&option, "key = ?", "MinTopUp").Error, gorm.ErrRecordNotFound)
}

func TestUpdateOptionsBulkMergesYooKassaMethodWithLatestPersistedPayMethods(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "PayMethods", Value: `[{"type":"custom","topup_group":"vip"}]`}).Error)

	previousDB := DB
	previousMethods := operation_setting.PayMethods
	previousYooKassaConfig := setting.GetYooKassaConfig()
	paymentSetting := operation_setting.GetPaymentSetting()
	previousCompliance := paymentSetting.ComplianceConfirmed
	previousTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		DB = previousDB
		operation_setting.PayMethods = previousMethods
		setting.PublishYooKassaConfig(previousYooKassaConfig)
		paymentSetting.ComplianceConfirmed = previousCompliance
		paymentSetting.ComplianceTermsVersion = previousTermsVersion
	})

	DB = db
	operation_setting.PayMethods = []map[string]string{{"type": "stale-local"}}
	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion

	require.NoError(t, UpdateOptionsBulk(map[string]string{"MinTopUp": "12"}))
	var option Option
	require.NoError(t, db.First(&option, "key = ?", "PayMethods").Error)
	var methods []map[string]string
	require.NoError(t, common.Unmarshal([]byte(option.Value), &methods))
	require.Len(t, methods, 2)
	require.Equal(t, "custom", methods[0]["type"])
	require.Equal(t, "vip", methods[0]["topup_group"])
	require.Equal(t, operation_setting.YooKassaSBPPaymentMethod, methods[1]["type"])
}

func TestUpdateOptionsBulkPublishesAtomicYooKassaConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&Option{}))

	previousDB := DB
	previousConfig := setting.GetYooKassaConfig()
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	t.Cleanup(func() {
		DB = previousDB
		setting.PublishYooKassaConfig(previousConfig)
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	DB = db
	operation_setting.GetPaymentSetting().ComplianceConfirmed = false
	configA := setting.YooKassaConfig{Enabled: true, ShopID: "shop-a", SecretKey: "secret-a", ReturnURL: "https://a.example/return", PaymentMethods: "sbp"}
	configB := setting.YooKassaConfig{Enabled: true, ShopID: "shop-b", SecretKey: "secret-b", ReturnURL: "https://b.example/return", PaymentMethods: "sbp,bank_card"}
	setting.PublishYooKassaConfig(configA)

	done := make(chan struct{})
	errs := make(chan error, 1)
	var readers sync.WaitGroup
	for range 4 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				config := setting.GetYooKassaConfig()
				if config != configA && config != configB {
					select {
					case errs <- errors.New("observed mixed YooKassa configuration"):
					default:
					}
					return
				}
			}
		}()
	}

	for i := range 40 {
		config := configA
		if i%2 == 1 {
			config = configB
		}
		require.NoError(t, UpdateOptionsBulk(map[string]string{
			"YooKassaEnabled":        "true",
			"YooKassaShopID":         config.ShopID,
			"YooKassaSecretKey":      config.SecretKey,
			"YooKassaReturnURL":      config.ReturnURL,
			"YooKassaPaymentMethods": config.PaymentMethods,
		}))
	}
	close(done)
	readers.Wait()
	select {
	case err := <-errs:
		require.NoError(t, err)
	default:
	}
}

package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestDirectUSDTAmountUnitsPreservesSixDecimalPlaces(t *testing.T) {
	tests := []struct {
		name   string
		amount string
		units  uint64
	}{
		{name: "minimum", amount: "10", units: 10_000_000},
		{name: "micro units", amount: "10.000001", units: 10_000_001},
		{name: "six decimals", amount: "123.456789", units: 123_456_789},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			amount, err := decimal.NewFromString(tt.amount)
			require.NoError(t, err)
			units, err := directUSDTAmountUnits(amount)
			require.NoError(t, err)
			assert.Equal(t, tt.units, units)
		})
	}
}

func TestDirectUSDTAmountUnitsRejectsSubMicroAndNonPositiveAmounts(t *testing.T) {
	for _, amount := range []string{"0", "-1", "10.0000001"} {
		value, err := decimal.NewFromString(amount)
		require.NoError(t, err)
		assert.Error(t, func() error {
			_, err := directUSDTAmountUnits(value)
			return err
		}(), amount)
	}
}

func TestPaymentMethodAdminOnlyGateAcceptsTypedJSONAndRejectsRegularUser(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "admin_only_methods.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"stripe","admin_only":true}]`}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	gin.SetMode(gin.TestMode)
	regular, _ := gin.CreateTestContext(httptest.NewRecorder())
	regular.Set("role", common.RoleCommonUser)
	require.False(t, paymentMethodAllowedForUser(regular, model.PaymentMethodStripe))

	admin, _ := gin.CreateTestContext(httptest.NewRecorder())
	admin.Set("role", common.RoleAdminUser)
	require.True(t, paymentMethodAllowedForUser(admin, model.PaymentMethodStripe))
}

func TestDirectCryptoGenericAndLegacyTRONRoutesCreateParentSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "crypto_parent_routes.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.User{}, &model.TopUp{}, &model.DirectCryptoPayment{}, &model.PaymentMetadata{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"crypto_direct","min_topup":"10","pending_ttl_minutes":"31"}]`}).Error)
	require.NoError(t, db.Create(&model.User{Id: 991, Username: "crypto-parent", Password: "password123", AffCode: "crypto-parent"}).Error)
	previousDB := model.DB
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	generic := gin.New()
	generic.POST("/crypto/:network/pay", func(c *gin.Context) {
		c.Set("id", 991)
		c.Set("role", common.RoleCommonUser)
		RequestDirectUSDTNetworkPay(c)
	})
	response := httptest.NewRecorder()
	generic.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/crypto/tron/pay", bytes.NewBufferString(`{"amount":"10","payment_method":"crypto_direct"}`)))
	require.Equal(t, http.StatusOK, response.Code)

	legacy := gin.New()
	legacy.POST("/usdt-trc20/pay", func(c *gin.Context) {
		c.Set("id", 991)
		c.Set("role", common.RoleCommonUser)
		RequestDirectUSDTTRC20Pay(c)
	})
	response = httptest.NewRecorder()
	legacy.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/usdt-trc20/pay", bytes.NewBufferString(`{"amount":"11","payment_method":"usdt_trc20_direct"}`)))
	require.Equal(t, http.StatusOK, response.Code)

	var topUps []model.TopUp
	require.NoError(t, db.Order("id").Find(&topUps).Error)
	require.Len(t, topUps, 2)
	for _, topUp := range topUps {
		assert.Equal(t, model.DirectCryptoProvider, topUp.PaymentMethod)
		assert.Equal(t, model.DirectCryptoProvider, topUp.PaymentProvider)
		assert.Equal(t, int64(31*60), topUp.PaymentPendingTTLSeconds)
	}
	var payments []model.DirectCryptoPayment
	require.NoError(t, db.Order("id").Find(&payments).Error)
	require.Len(t, payments, 2)
	for _, payment := range payments {
		assert.Equal(t, "TRON", payment.Network)
		assert.Equal(t, payment.CreatedAt+int64(31*60), payment.ExpiresAt)
	}
}

func TestDirectUSDTBaseAmountConvertsTokenDisplayToUSD(t *testing.T) {
	originalGeneralSetting := *operation_setting.GetGeneralSetting()
	originalQuotaPerUnit := common.GetQuotaPerUnit()
	t.Cleanup(func() {
		*operation_setting.GetGeneralSetting() = originalGeneralSetting
		common.SetQuotaPerUnit(originalQuotaPerUnit)
	})
	common.SetQuotaPerUnit(500_000)

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeUSD
	usdUnits, err := directUSDTBaseAmountUnits(decimal.NewFromInt(10))
	require.NoError(t, err)
	assert.Equal(t, uint64(10_000_000), usdUnits)

	operation_setting.GetGeneralSetting().QuotaDisplayType = operation_setting.QuotaDisplayTypeTokens
	tokenUnits, err := directUSDTBaseAmountUnits(decimal.NewFromInt(5_000_000))
	require.NoError(t, err)
	assert.Equal(t, usdUnits, tokenUnits)
}

func TestGetTopUpInfoPublishesConfiguredDirectUSDTMethod(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_topup_info.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":" USDT_TRC20_DIRECT ","name":"bad","currency":"BTC","min_topup":"0"},{"type":"usdt_trc20_direct","name":"duplicate"}]`}).Error)

	previousDB := model.DB
	previousDatabaseType := common.MainDatabaseType()
	previousLogDatabaseType := common.LogDatabaseType()
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		common.SetDatabaseTypes(previousDatabaseType, previousLogDatabaseType)
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) {
		c.Set("id", 1)
		GetTopUpInfo(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topup/info", nil))
	require.Equal(t, http.StatusOK, recorder.Code)

	var response struct {
		Data struct {
			PayMethods     []map[string]string `json:"pay_methods"`
			CryptoNetworks []string            `json:"crypto_networks"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	var direct map[string]string
	directCount := 0
	for _, method := range response.Data.PayMethods {
		if method["type"] == model.DirectCryptoProvider {
			directCount++
			direct = method
		}
	}
	require.NotNil(t, direct)
	assert.Equal(t, 1, directCount)
	assert.Equal(t, "Crypto", direct["name"])
	assert.Equal(t, "USDT", direct["currency"])
	assert.Equal(t, "1", direct["rate_to_usd"])
	assert.Equal(t, "6", direct["rounding_decimals"])
	assert.Equal(t, "10", direct["min_topup"])
	assert.Equal(t, []string{"TRON"}, response.Data.CryptoNetworks)
}

func TestGetTopUpQuoteRejectsDirectUSDTWithoutComplianceOrValidConfig(t *testing.T) {
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
	})
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "invalid"
	setting.USDTTRC20APIKey = "key"

	for _, tc := range []struct {
		name       string
		compliance bool
	}{
		{name: "compliance missing", compliance: false},
		{name: "direct config invalid", compliance: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			operation_setting.GetPaymentSetting().ComplianceConfirmed = tc.compliance
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", 1)
			c.Request = httptest.NewRequest(http.MethodPost, "/topup/quote", bytes.NewBufferString(`{"amount":10,"payment_method":"usdt_trc20_direct"}`))
			GetTopUpQuote(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"success":false`)
		})
	}
}

func TestGetTopUpQuoteLegacyCryptoIDUsesParentAdminOnlyPolicy(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "crypto_quote_admin_only.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"crypto_direct","admin_only":true}]`}).Error)
	previousDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = previousDB })

	gin.SetMode(gin.TestMode)
	for _, paymentMethod := range []string{
		operation_setting.DirectUSDTTONPaymentMethod,
		operation_setting.DirectUSDTSolanaPaymentMethod,
	} {
		t.Run(paymentMethod, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("id", 1)
			c.Set("role", common.RoleCommonUser)
			c.Request = httptest.NewRequest(http.MethodPost, "/topup/quote", bytes.NewBufferString(`{"amount":10,"payment_method":"`+paymentMethod+`"}`))
			GetTopUpQuote(c)
			require.Equal(t, http.StatusOK, recorder.Code)
			assert.Contains(t, recorder.Body.String(), `"success":false`)
			assert.Contains(t, recorder.Body.String(), "Payment method is not available")
		})
	}
}

func TestGetDirectUSDTNetworkStatusUsesInvoiceSnapshotWithoutLiveSolanaReadiness(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "crypto_status_snapshot.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.TopUp{}, &model.DirectCryptoPayment{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"crypto_direct"}]`}).Error)
	now := time.Now().Unix()
	destination := "11111111111111111111111111111111"
	require.NoError(t, db.Create(&model.DirectCryptoPayment{
		TradeNo: "solana-status-snapshot", UserId: 991, Network: "SOLANA", Token: "USDT",
		Contract: setting.USDTSolanaMint, Address: destination, ReceivingOwner: destination,
		Destination: destination, ExpectedUnits: 10_000_001, BaseUnits: 10_000_000,
		SuffixUnits: 1, Status: model.DirectCryptoPending, CreatedAt: now, UpdatedAt: now,
		ExpiresAt: now + 60,
	}).Error)
	previousDB := model.DB
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/crypto/:network/:trade_no", func(c *gin.Context) {
		c.Set("id", 991)
		c.Set("role", common.RoleCommonUser)
		GetDirectUSDTNetworkStatus(c)
	})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/crypto/solana/solana-status-snapshot", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":true`)
	assert.Contains(t, recorder.Body.String(), `"network":"SOLANA"`)
}

func TestGetTopUpInfoHidesDirectUSDTWhenCatalogMethodIsAbsent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_topup_info_absent.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"alipay"}]`}).Error)
	previousDB := model.DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = "test-read-only-key"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) { c.Set("id", 1); GetTopUpInfo(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topup/info", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			PayMethods []map[string]string `json:"pay_methods"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	for _, method := range response.Data.PayMethods {
		require.NotEqual(t, model.DirectCryptoProvider, method["type"])
	}
}

func TestGetTopUpInfoHidesDirectUSDTWhenConfigIsInvalid(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_topup_info_invalid.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"usdt_trc20_direct"}]`}).Error)
	previousDB := model.DB
	previousEnabled := setting.USDTTRC20Enabled
	previousAddress := setting.USDTTRC20ReceivingAddress
	previousAPIKey := setting.USDTTRC20APIKey
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	setting.USDTTRC20Enabled = true
	setting.USDTTRC20ReceivingAddress = "invalid"
	setting.USDTTRC20APIKey = "test-read-only-key"
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20Enabled = previousEnabled
		setting.USDTTRC20ReceivingAddress = previousAddress
		setting.USDTTRC20APIKey = previousAPIKey
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) { c.Set("id", 1); GetTopUpInfo(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topup/info", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			PayMethods []map[string]string `json:"pay_methods"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	for _, method := range response.Data.PayMethods {
		require.NotEqual(t, model.DirectCryptoProvider, method["type"])
	}
}

func TestGetTopUpInfoHidesCryptoWhenNoNetworkHasReadOnlyCredential(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_topup_info_no_credential.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"crypto_direct"}]`}).Error)

	previousDB := model.DB
	previousTronAddress, previousTronKey := setting.USDTTRC20ReceivingAddress, setting.USDTTRC20APIKey
	previousTonAddress, previousTonKey := setting.USDTTONReceivingAddress, setting.USDTTONAPIKey
	previousSolanaAddress, previousSolanaKey := setting.USDTSolanaReceivingAddress, setting.USDTSolanaAPIKey
	previousTokenAccount := setting.USDTSolanaReceivingTokenAccount
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	setting.USDTTRC20ReceivingAddress = "TJRabPrwbZy45sbavfcjinPJC18kjpRTv8"
	setting.USDTTRC20APIKey = ""
	setting.USDTTONReceivingAddress, setting.USDTTONAPIKey = "", ""
	setting.USDTSolanaReceivingAddress, setting.USDTSolanaAPIKey, setting.USDTSolanaReceivingTokenAccount = "", "", ""
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		setting.USDTTRC20ReceivingAddress, setting.USDTTRC20APIKey = previousTronAddress, previousTronKey
		setting.USDTTONReceivingAddress, setting.USDTTONAPIKey = previousTonAddress, previousTonKey
		setting.USDTSolanaReceivingAddress, setting.USDTSolanaAPIKey, setting.USDTSolanaReceivingTokenAccount = previousSolanaAddress, previousSolanaKey, previousTokenAccount
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) { c.Set("id", 1); GetTopUpInfo(c) })
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topup/info", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data struct {
			PayMethods     []map[string]string `json:"pay_methods"`
			CryptoNetworks []string            `json:"crypto_networks"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Empty(t, response.Data.CryptoNetworks)
	for _, method := range response.Data.PayMethods {
		require.NotEqual(t, model.DirectCryptoProvider, method["type"])
	}
}

func TestRequestDirectUSDTRejectsWhenCatalogMethodIsAbsent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "direct_pay_absent.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))
	require.NoError(t, db.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"alipay"}]`}).Error)
	previousDB := model.DB
	previousCompliance := operation_setting.GetPaymentSetting().ComplianceConfirmed
	model.DB = db
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	t.Cleanup(func() {
		model.DB = previousDB
		operation_setting.GetPaymentSetting().ComplianceConfirmed = previousCompliance
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/usdt-trc20/pay", func(c *gin.Context) { c.Set("id", 1); RequestDirectUSDTTRC20Pay(c) })
	recorder := httptest.NewRecorder()
	body := bytes.NewBufferString(`{"amount":"10","payment_method":"usdt_trc20_direct"}`)
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/usdt-trc20/pay", body))
	require.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "not available")
}

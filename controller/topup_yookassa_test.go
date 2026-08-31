package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupYooKassaWebhookTest(t *testing.T, paymentResponse string) *gin.Engine {
	t.Helper()

	originalMainDatabaseType := common.MainDatabaseType()
	originalLogDatabaseType := common.LogDatabaseType()
	originalRedisEnabled := common.RedisEnabled
	originalYooKassaConfig := setting.GetYooKassaConfig()
	originalPaymentSetting := *operation_setting.GetPaymentSetting()
	originalYooKassaAPIBaseURL := service.YooKassaAPIBaseURL
	originalYooKassaHTTPClient := service.YooKassaHTTPClient
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.TopUp{}, &model.PaymentMetadata{}, &model.Log{}))

	setting.PublishYooKassaConfig(setting.YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret", PaymentMethods: "sbp"})
	operation_setting.GetPaymentSetting().ComplianceConfirmed = true
	operation_setting.GetPaymentSetting().ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
	t.Cleanup(func() {
		common.RedisEnabled = originalRedisEnabled
		setting.PublishYooKassaConfig(originalYooKassaConfig)
		*operation_setting.GetPaymentSetting() = originalPaymentSetting
		service.YooKassaAPIBaseURL = originalYooKassaAPIBaseURL
		service.YooKassaHTTPClient = originalYooKassaHTTPClient
		common.SetDatabaseTypes(originalMainDatabaseType, originalLogDatabaseType)
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v3/payments/pay_1", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(paymentResponse))
	}))
	t.Cleanup(server.Close)

	service.YooKassaAPIBaseURL = server.URL + "/v3"
	service.YooKassaHTTPClient = server.Client()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/api/user/yookassa/notify", YooKassaNotify)
	return router
}

func insertYooKassaOrderForWebhookTest(t *testing.T, metadata string, quotaToAdd int) {
	t.Helper()
	if metadata == "" {
		metadata = `{"quota_to_add":"500000"}`
	}
	if quotaToAdd <= 0 {
		quotaToAdd = 500000
	}
	require.NoError(t, model.DB.Create(&model.User{
		Id:       1,
		Username: "yk_user",
		Password: "password",
		Status:   common.UserStatusEnabled,
		Group:    "default",
		Quota:    0,
	}).Error)
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          1,
		Amount:          10,
		Money:           100,
		TradeNo:         "trade-1",
		PaymentMethod:   model.PaymentMethodYooKassaSBP,
		PaymentProvider: model.PaymentProviderYooKassa,
		QuotaToAdd:      quotaToAdd,
		CreateTime:      time.Now().Unix(),
		Status:          common.TopUpStatusPending,
	}).Error)
	require.NoError(t, model.DB.Create(&model.PaymentMetadata{
		TradeNo:           "trade-1",
		PaymentProvider:   model.PaymentProviderYooKassa,
		ExternalPaymentID: "pay_1",
		Metadata:          metadata,
		CreateTime:        time.Now().Unix(),
		UpdateTime:        time.Now().Unix(),
	}).Error)
}

func postYooKassaWebhook(t *testing.T, router *gin.Engine) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/user/yookassa/notify", strings.NewReader(`{
		"type":"notification",
		"event":"payment.succeeded",
		"object":{"id":"pay_1"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func postYooKassaSync(t *testing.T, userID int, role int) *httptest.ResponseRecorder {
	t.Helper()
	router := gin.New()
	router.POST("/api/user/yookassa/sync", func(c *gin.Context) {
		c.Set("id", userID)
		c.Set("role", role)
		SyncYooKassaTopUp(c)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/user/yookassa/sync", strings.NewReader(`{"trade_no":"trade-1"}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func yookassaPaymentResponse(status string, paid bool, amount string) string {
	return `{
		"id":"pay_1",
		"status":"` + status + `",
		"paid":` + map[bool]string{true: "true", false: "false"}[paid] + `,
		"amount":{"value":"` + amount + `","currency":"RUB"},
		"metadata":{"trade_no":"trade-1","user_id":"1","topup_id":"1"}
	}`
}

func TestYooKassaWebhookPaymentSucceeded(t *testing.T) {
	router := setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 500000, user.Quota)
}

func TestYooKassaWebhookIsIdempotent(t *testing.T) {
	router := setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	assert.Equal(t, http.StatusOK, postYooKassaWebhook(t, router).Code)
	assert.Equal(t, http.StatusOK, postYooKassaWebhook(t, router).Code)

	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 500000, user.Quota)
}

func TestTopUpInfoFiltersYooKassaSBPWhenSBPIsDisabled(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	originalPayMethods := operation_setting.PayMethods2JsonString()
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(originalPayMethods))
	})
	require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[
		{"type":"alipay","name":"Alipay"},
		{"type":"yookassa_sbp","name":"СБП"}
	]`))

	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) {
		c.Set("id", 1)
		GetTopUpInfo(c)
	})

	for _, tt := range []struct {
		name         string
		methods      string
		wantYooKassa bool
	}{
		{name: "no methods", methods: "", wantYooKassa: false},
		{name: "SBP disabled", methods: "bank_card", wantYooKassa: false},
		{name: "SBP enabled", methods: "sbp,bank_card", wantYooKassa: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			config := setting.GetYooKassaConfig()
			config.PaymentMethods = tt.methods
			setting.PublishYooKassaConfig(config)
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/topup/info", nil))
			require.Equal(t, http.StatusOK, recorder.Code)

			var response struct {
				Data struct {
					EnableYooKassa bool                `json:"enable_yookassa_topup"`
					PayMethods     []map[string]string `json:"pay_methods"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			hasYooKassa, hasAlipay := false, false
			for _, method := range response.Data.PayMethods {
				switch method["type"] {
				case model.PaymentMethodYooKassaSBP:
					hasYooKassa = true
				case "alipay":
					hasAlipay = true
				}
			}
			assert.Equal(t, tt.wantYooKassa, hasYooKassa)
			assert.Equal(t, tt.wantYooKassa, response.Data.EnableYooKassa)
			assert.True(t, hasAlipay)
		})
	}
}

func TestTopUpInfoDoesNotReaddExplicitlyRemovedYooKassaSBP(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	previousPayMethods := operation_setting.PayMethods2JsonString()
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
	})
	require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[{"type":"alipay","name":"Alipay"}]`))

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
			EnableYooKassa bool                `json:"enable_yookassa_topup"`
			PayMethods     []map[string]string `json:"pay_methods"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.False(t, response.Data.EnableYooKassa)
	for _, method := range response.Data.PayMethods {
		assert.NotEqual(t, model.PaymentMethodYooKassaSBP, method["type"])
	}
}

func TestTopUpInfoExposesConfiguredManualLargePayment(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	original := *operation_setting.GetPaymentSetting()
	t.Cleanup(func() { *operation_setting.GetPaymentSetting() = original })

	settings := operation_setting.GetPaymentSetting()
	settings.ManualTopupEnabled = true
	settings.ManualTopupMinAmount = 5000
	settings.ManualTopupContactURL = "https://t.me/vibecode_support"

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
			Enabled bool    `json:"manual_topup_enabled"`
			Minimum float64 `json:"manual_topup_min_amount"`
			Contact string  `json:"manual_topup_contact_url"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Data.Enabled)
	assert.Equal(t, 5000.0, response.Data.Minimum)
	assert.Equal(t, "https://t.me/vibecode_support", response.Data.Contact)
}

func TestYooKassaDirectRoutesRejectDisabledPersistedSBP(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	// Keep the process-local snapshot stale on purpose; direct routes must use
	// the authoritative persisted PayMethods row.
	previousPayMethods := operation_setting.PayMethods2JsonString()
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
	})
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodYooKassaSBP}}
	require.NoError(t, model.DB.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"alipay"}]`}).Error)

	router := gin.New()
	for _, path := range []string{"/yookassa/amount", "/yookassa/pay", "/topup/quote"} {
		router.POST(path, func(c *gin.Context) {
			c.Set("id", 1)
			switch c.Request.URL.Path {
			case "/yookassa/amount":
				RequestYooKassaAmount(c)
			case "/yookassa/pay":
				RequestYooKassaPay(c)
			default:
				GetTopUpQuote(c)
			}
		})
	}
	for _, path := range []string{"/yookassa/amount", "/yookassa/pay", "/topup/quote"} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"amount":10,"payment_method":"yookassa_sbp"}`)))
		require.Equal(t, http.StatusOK, recorder.Code, path)
		var response struct {
			Message string `json:"message"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		assert.NotEqual(t, "success", response.Message, path)
	}
}

func TestYooKassaCurrentPayMethodsAllowsEnabledSBP(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	previousPayMethods := operation_setting.PayMethods2JsonString()
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
	})
	require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(`[{"type":"yookassa_sbp"}]`))
	require.NoError(t, model.DB.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"yookassa_sbp"}]`}).Error)
	assert.True(t, isCurrentYooKassaSBPEnabled())
	assert.True(t, isYooKassaPaymentMethodEnabled(model.PaymentMethodYooKassaSBP))
}

func TestTopUpInfoUsesPersistedPayMethodsOverStaleSnapshot(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	previousPayMethods := operation_setting.PayMethods2JsonString()
	t.Cleanup(func() {
		require.NoError(t, operation_setting.UpdatePayMethodsByJsonString(previousPayMethods))
	})
	operation_setting.PayMethods = []map[string]string{{"type": model.PaymentMethodYooKassaSBP, "name": "СБП"}}
	require.NoError(t, model.DB.Create(&model.Option{Key: "PayMethods", Value: `[{"type":"alipay","name":"Alipay"}]`}).Error)

	router := gin.New()
	router.GET("/topup/info", func(c *gin.Context) {
		c.Set("id", 1)
		GetTopUpInfo(c)
	})
	containsSBP := func() bool {
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
			if method["type"] == model.PaymentMethodYooKassaSBP {
				return true
			}
		}
		return false
	}

	assert.False(t, containsSBP(), "deleted persisted SBP must not be shown from a stale process snapshot")
	require.NoError(t, model.DB.Model(&model.Option{}).Where("key = ?", "PayMethods").Update("value", `[{"type":"yookassa_sbp","name":"СБП"}]`).Error)
	assert.True(t, containsSBP(), "currently persisted SBP must remain visible")
}

func TestYooKassaWebhookAcknowledgesExpiredOrder(t *testing.T) {
	router := setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "trade-1").Update("status", common.TopUpStatusExpired).Error)

	assert.Equal(t, http.StatusOK, postYooKassaWebhook(t, router).Code)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Zero(t, user.Quota)
}

func TestYooKassaWebhookAcknowledgesInvalidAmount(t *testing.T) {
	router := setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "99.99"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestYooKassaWebhookRejectsInvalidStatus(t *testing.T) {
	router := setupYooKassaWebhookTest(t, yookassaPaymentResponse("pending", false, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
}

func TestYooKassaWebhookUsesPaymentMetadataWhenProviderMetadataMissingTradeNo(t *testing.T) {
	paymentResponse := `{
		"id":"pay_1",
		"status":"succeeded",
		"paid":true,
		"amount":{"value":"100.00","currency":"RUB"},
		"metadata":{"user_id":"1","topup_id":"1"}
	}`
	router := setupYooKassaWebhookTest(t, paymentResponse)
	insertYooKassaOrderForWebhookTest(t, `{"quota_to_add":"123456"}`, 123456)

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusOK, recorder.Code)

	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 123456, user.Quota)
}

func TestYooKassaWebhookKeepsMissingMetadataAsBadRequest(t *testing.T) {
	paymentResponse := `{
		"id":"pay_1",
		"status":"succeeded",
		"paid":true,
		"amount":{"value":"100.00","currency":"RUB"},
		"metadata":{"user_id":"1","topup_id":"1"}
	}`
	router := setupYooKassaWebhookTest(t, paymentResponse)
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Where("trade_no = ?", "trade-1").Delete(&model.PaymentMetadata{}).Error)

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestYooKassaSyncPaymentSucceeded(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	recorder := postYooKassaSync(t, 1, common.RoleCommonUser)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 500000, user.Quota)
}

func TestYooKassaAdminSyncCompletesExpiredPayment(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "trade-1").Update("status", common.TopUpStatusExpired).Error)

	recorder := postYooKassaSync(t, 2, common.RoleAdminUser)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 500000, user.Quota)
}

func TestYooKassaAdminSyncCompletesPendingPaymentAfterExpiry(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "trade-1").Updates(map[string]interface{}{
		"create_time":                 time.Now().Add(-2 * time.Second).Unix(),
		"payment_pending_ttl_seconds": 1,
	}).Error)

	recorder := postYooKassaSync(t, 2, common.RoleAdminUser)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 500000, user.Quota)
}

func TestYooKassaUserSyncDoesNotCompleteExpiredPayment(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "trade-1").Update("status", common.TopUpStatusExpired).Error)

	recorder := postYooKassaSync(t, 1, common.RoleCommonUser)
	assert.Equal(t, http.StatusOK, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusExpired, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Zero(t, user.Quota)
}

func TestYooKassaAdminSyncDoesNotCreditExpiredPaymentWithWrongAmount(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "99.99"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Model(&model.TopUp{}).Where("trade_no = ?", "trade-1").Update("status", common.TopUpStatusExpired).Error)

	recorder := postYooKassaSync(t, 2, common.RoleAdminUser)
	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message string `json:"message"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.NotEqual(t, "success", response.Message)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusExpired, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Zero(t, user.Quota)
}

func TestYooKassaSyncReturnsServerErrorOnPaymentMetadataDatabaseError(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)
	require.NoError(t, model.DB.Migrator().DropTable(&model.PaymentMetadata{}))

	recorder := postYooKassaSync(t, 1, common.RoleCommonUser)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestYooKassaWebhookReturnsServerErrorOnPaymentMetadataDatabaseError(t *testing.T) {
	paymentResponse := `{
		"id":"pay_1",
		"status":"succeeded",
		"paid":true,
		"amount":{"value":"100.00","currency":"RUB"},
		"metadata":{"user_id":"1","topup_id":"1"}
	}`
	router := setupYooKassaWebhookTest(t, paymentResponse)
	insertYooKassaOrderForWebhookTest(t, `{"quota_to_add":"123456"}`, 123456)
	require.NoError(t, model.DB.Migrator().DropTable(&model.PaymentMetadata{}))

	recorder := postYooKassaWebhook(t, router)
	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func TestYooKassaSyncRejectsOtherUserOrder(t *testing.T) {
	setupYooKassaWebhookTest(t, yookassaPaymentResponse("succeeded", true, "100.00"))
	insertYooKassaOrderForWebhookTest(t, "", 500000)

	recorder := postYooKassaSync(t, 2, common.RoleCommonUser)
	assert.Equal(t, http.StatusForbidden, recorder.Code)

	topUp := model.GetTopUpByTradeNo("trade-1")
	require.NotNil(t, topUp)
	assert.Equal(t, common.TopUpStatusPending, topUp.Status)
	var user model.User
	require.NoError(t, model.DB.First(&user, 1).Error)
	assert.Equal(t, 0, user.Quota)
}

func TestYooKassaPaymentMethodIsSBPOnly(t *testing.T) {
	originalYooKassaConfig := setting.GetYooKassaConfig()
	t.Cleanup(func() {
		setting.PublishYooKassaConfig(originalYooKassaConfig)
	})

	config := setting.GetYooKassaConfig()
	config.PaymentMethods = "sbp,bank_card"
	setting.PublishYooKassaConfig(config)

	assert.True(t, isYooKassaPaymentMethodEnabled(model.PaymentMethodYooKassaSBP))
	assert.True(t, isYooKassaPaymentMethodEnabled("sbp"))
	assert.False(t, isYooKassaPaymentMethodEnabled("bank_card"))
}

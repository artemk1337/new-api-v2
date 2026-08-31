package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateOptionPayMethodsPersistsAndNormalizesProviderCurrency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.PlatformCurrency{}))
	require.NoError(t, db.Create(&model.PlatformCurrency{Code: "USD", Name: "US Dollar", Symbol: "$", Enabled: true, RateToUSD: 1, ManualRateToUSD: 1}).Error)
	require.NoError(t, db.Create(&model.PlatformCurrency{Code: "EUR", Name: "Euro", Symbol: "€", Enabled: true, RateToUSD: 0.92, ManualRateToUSD: 0.92}).Error)

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalPayMethods := operation_setting.PayMethods
	originalWaffoCurrency := setting.WaffoCurrency
	originalNOWPaymentsPriceCurrency := setting.NOWPaymentsPriceCurrency
	originalNOWPaymentsPayCurrency := setting.NOWPaymentsPayCurrency
	originalRedisEnabled := common.RedisEnabled
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = make(map[string]string)
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		operation_setting.PayMethods = originalPayMethods
		setting.WaffoCurrency = originalWaffoCurrency
		setting.NOWPaymentsPriceCurrency = originalNOWPaymentsPriceCurrency
		setting.NOWPaymentsPayCurrency = originalNOWPaymentsPayCurrency
		common.RedisEnabled = originalRedisEnabled
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	update := func(key, value string) bool {
		body, marshalErr := common.Marshal(OptionUpdateRequest{Key: key, Value: value})
		require.NoError(t, marshalErr)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
		UpdateOption(ctx)
		var response struct {
			Success bool `json:"success"`
		}
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
		return response.Success
	}

	require.False(t, update("PayMethods", `[{"name":"Alipay","type":"alipay","currency":"USD"}]`), "a new method requires a configured topup_group")
	require.True(t, update("PayMethods", `[{"name":"Alipay","type":"alipay","currency":"USD","topup_group":"default"}]`))
	var stored model.Option
	require.NoError(t, db.First(&stored, "key = ?", "PayMethods").Error)
	assert.JSONEq(t, `[{"name":"Alipay","type":"alipay","topup_group":"default"}]`, stored.Value)
	require.Len(t, operation_setting.PayMethods, 1)
	_, hasCurrency := operation_setting.PayMethods[0]["currency"]
	assert.False(t, hasCurrency)

	setting.WaffoCurrency = "USD"
	require.True(t, update("PayMethods", `[{"name":"Waffo","type":" WAFFO ","currency":"RUB","topup_group":"default"}]`))
	_, hasCurrency = operation_setting.PayMethods[0]["currency"]
	assert.False(t, hasCurrency)

	require.True(t, update("PayMethods", `[{"name":"Alipay","type":"alipay","currency":"RUB","topup_group":"default"}]`))
	stored = model.Option{}
	require.NoError(t, db.First(&stored, "key = ?", "PayMethods").Error)
	assert.JSONEq(t, `[{"name":"Alipay","type":"alipay","topup_group":"default"}]`, stored.Value)
	_, hasCurrency = operation_setting.PayMethods[0]["currency"]
	assert.False(t, hasCurrency)

	require.True(t, update("NOWPaymentsPriceCurrency", "usd"))
	stored = model.Option{}
	require.NoError(t, db.First(&stored, "key = ?", "NOWPaymentsPriceCurrency").Error)
	assert.Equal(t, "usdt", stored.Value)
	assert.Equal(t, "usdt", setting.NOWPaymentsPriceCurrency)
	require.True(t, update("NOWPaymentsPayCurrency", "usdttrc20"))
	stored = model.Option{}
	require.NoError(t, db.First(&stored, "key = ?", "NOWPaymentsPayCurrency").Error)
	assert.Equal(t, "usdt", stored.Value)
	assert.Equal(t, "usdt", setting.NOWPaymentsPayCurrency)
}

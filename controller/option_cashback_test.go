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
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestUpdateAndGetOptionsExposeCanonicalCashbackArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalRedisEnabled := common.RedisEnabled
	var originalCashbacks operation_setting.AmountCashbackConfig
	if current := operation_setting.GetPaymentSetting().AmountCashback; current != nil {
		originalCashbacks = append(operation_setting.AmountCashbackConfig{}, current...)
	}
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
		common.RedisEnabled = originalRedisEnabled
		operation_setting.GetPaymentSetting().AmountCashback = originalCashbacks
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})

	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "payment_setting.amount_cashback",
		Value: "null",
	})
	require.NoError(t, err)
	updateRecorder := httptest.NewRecorder()
	updateContext, _ := gin.CreateTestContext(updateRecorder)
	updateContext.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	UpdateOption(updateContext)

	var updatePayload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(updateRecorder.Body.Bytes(), &updatePayload))
	require.True(t, updatePayload.Success)

	var stored model.Option
	require.NoError(t, db.First(&stored, "key = ?", "payment_setting.amount_cashback").Error)
	require.Equal(t, `[]`, stored.Value)

	getRecorder := httptest.NewRecorder()
	getContext, _ := gin.CreateTestContext(getRecorder)
	getContext.Request = httptest.NewRequest(http.MethodGet, "/api/option", nil)
	GetOptions(getContext)

	var getPayload struct {
		Success bool            `json:"success"`
		Data    []*model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(getRecorder.Body.Bytes(), &getPayload))
	require.True(t, getPayload.Success)
	values := make(map[string]string, len(getPayload.Data))
	for _, option := range getPayload.Data {
		values[option.Key] = option.Value
	}
	require.Equal(t, `[]`, values["payment_setting.amount_cashback"])
}

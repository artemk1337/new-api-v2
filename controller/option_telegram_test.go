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
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGetOptionsMasksTelegramBotToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = map[string]string{
		"TelegramBotToken": "telegram-token",
		"SystemName":       "new-api",
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
	})

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/option", nil)
	GetOptions(ctx)

	var payload struct {
		Success bool            `json:"success"`
		Data    []*model.Option `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	values := make(map[string]string, len(payload.Data))
	for _, option := range payload.Data {
		values[option.Key] = option.Value
	}
	require.Equal(t, maskedOptionValue, values["TelegramBotToken"])
	require.NotContains(t, recorder.Body.String(), "telegram-token")
}

func TestUpdateOptionTelegramBotTokenMaskKeepsStoredValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalToken := common.TelegramBotToken
	originalRedisEnabled := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.TelegramBotToken = "telegram-token"
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.TelegramBotToken = originalToken
		common.RedisEnabled = originalRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	require.NoError(t, db.Create(&model.Option{
		Key:   "TelegramBotToken",
		Value: "telegram-token",
	}).Error)

	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "TelegramBotToken",
		Value: maskedOptionValue,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	UpdateOption(ctx)

	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "TelegramBotToken").Error)
	require.Equal(t, "telegram-token", option.Value)
	require.Equal(t, "telegram-token", common.TelegramBotToken)
}

func TestUpdateOptionTelegramBotTokenReplacesStoredValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}))

	originalDB := model.DB
	originalLogDB := model.LOG_DB
	originalToken := common.TelegramBotToken
	originalRedisEnabled := common.RedisEnabled
	model.DB = db
	model.LOG_DB = db
	common.TelegramBotToken = "old-token"
	common.RedisEnabled = false
	t.Cleanup(func() {
		model.DB = originalDB
		model.LOG_DB = originalLogDB
		common.TelegramBotToken = originalToken
		common.RedisEnabled = originalRedisEnabled
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "TelegramBotToken",
		Value: "new-token",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	ctx.Set("id", 1)
	UpdateOption(ctx)

	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.True(t, payload.Success)

	var option model.Option
	require.NoError(t, db.First(&option, "key = ?", "TelegramBotToken").Error)
	require.Equal(t, "new-token", option.Value)
	require.Equal(t, "new-token", common.TelegramBotToken)
}

func TestUpdateOptionTelegramOAuthRequiresBotName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	originalToken := common.TelegramBotToken
	originalBotName := common.TelegramBotName
	common.TelegramBotToken = "telegram-token"
	common.TelegramBotName = ""
	t.Cleanup(func() {
		common.TelegramBotToken = originalToken
		common.TelegramBotName = originalBotName
	})

	body, err := common.Marshal(OptionUpdateRequest{
		Key:   "TelegramOAuthEnabled",
		Value: true,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/option", bytes.NewReader(body))
	UpdateOption(ctx)

	var payload struct {
		Success bool `json:"success"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.False(t, payload.Success)
}

package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupUserNotificationSettingTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.User{}))
	model.DB = db
	model.LOG_DB = db
	common.RedisEnabled = false

	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

func updateUserTelegramSetting(t *testing.T, chatID string) *httptest.ResponseRecorder {
	t.Helper()

	body, err := common.Marshal(UpdateUserSettingRequest{
		QuotaWarningType:      dto.NotifyTypeTelegram,
		QuotaWarningThreshold: 100000,
		TelegramChatId:        chatID,
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPut, "/api/user/setting", bytes.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", 1)
	UpdateUserSetting(ctx)
	return recorder
}

func TestUpdateUserSettingTelegramValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	setupUserNotificationSettingTestDB(t)

	oldToken := common.TelegramBotToken
	t.Cleanup(func() { common.TelegramBotToken = oldToken })

	common.TelegramBotToken = ""
	require.False(t, apiResponseSuccess(t, updateUserTelegramSetting(t, "123")))

	common.TelegramBotToken = "bot-token"
	for _, chatID := range []string{"", "0", "+123", "chat-id"} {
		require.False(t, apiResponseSuccess(t, updateUserTelegramSetting(t, chatID)))
	}
}

func TestUpdateUserSettingStoresTelegramChatID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, i18n.Init())
	db := setupUserNotificationSettingTestDB(t)
	require.NoError(t, db.Create(&model.User{Id: 1, Username: "telegram-user", Password: "password", Status: common.UserStatusEnabled}).Error)

	oldToken := common.TelegramBotToken
	common.TelegramBotToken = "bot-token"
	t.Cleanup(func() { common.TelegramBotToken = oldToken })

	require.True(t, apiResponseSuccess(t, updateUserTelegramSetting(t, "-100123")))

	user, err := model.GetUserById(1, true)
	require.NoError(t, err)
	settings := user.GetSetting()
	require.Equal(t, dto.NotifyTypeTelegram, settings.NotifyType)
	require.Equal(t, "-100123", settings.TelegramChatId)
}

func apiResponseSuccess(t *testing.T, recorder *httptest.ResponseRecorder) bool {
	t.Helper()

	response := struct {
		Success bool `json:"success"`
	}{}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	return response.Success
}

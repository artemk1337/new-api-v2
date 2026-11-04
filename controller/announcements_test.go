package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnnouncementController(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.Option{}, &model.AnnouncementTelegramDelivery{}, &model.SystemTask{}))
	originalDB := model.DB
	originalConsole := *console_setting.GetConsoleSetting()
	common.OptionMapRWMutex.Lock()
	originalOptions := common.OptionMap
	common.OptionMap = map[string]string{}
	common.OptionMapRWMutex.Unlock()
	model.DB = db
	t.Cleanup(func() {
		model.DB = originalDB
		*console_setting.GetConsoleSetting() = originalConsole
		common.OptionMapRWMutex.Lock()
		common.OptionMap = originalOptions
		common.OptionMapRWMutex.Unlock()
		sqlDB, err := db.DB()
		require.NoError(t, err)
		require.NoError(t, sqlDB.Close())
	})
}

func TestAnnouncementAPIRejectsStaleRevision(t *testing.T) {
	setupAnnouncementController(t)
	settings, err := model.GetAnnouncementSettings()
	require.NoError(t, err)
	settings.Channel = "@news_channel"
	body, err := common.Marshal(settings)
	require.NoError(t, err)
	call := func() *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/announcements/", bytes.NewReader(body))
		SaveAnnouncements(ctx)
		return recorder
	}
	require.Equal(t, http.StatusOK, call().Code)
	require.Equal(t, http.StatusConflict, call().Code)
}

func TestPublicAnnouncementsHideTelegramDraftAndState(t *testing.T) {
	setupAnnouncementController(t)
	settings, err := model.GetAnnouncementSettings()
	require.NoError(t, err)
	settings.Channel = "@news_channel"
	settings.Announcements = []model.AnnouncementAdmin{{
		Announcement: model.Announcement{Content: "Short announcement", PublishDate: "2026-11-04T12:00:00Z", Type: "default"},
		TelegramMode: "bot", TelegramText: "Private full draft",
	}}
	_, err = model.SaveAnnouncementSettings(settings)
	require.NoError(t, err)
	public, err := model.PublicAnnouncements()
	require.NoError(t, err)
	body, err := common.Marshal(public)
	require.NoError(t, err)
	require.Contains(t, string(body), "Short announcement")
	for _, private := range []string{"Private full draft", "telegramText", "telegramStatus", "telegramError", "chatId", "messageId"} {
		require.NotContains(t, string(body), private)
	}
}

func TestAnnouncementAPIMalformedBody(t *testing.T) {
	for _, handler := range []gin.HandlerFunc{SaveAnnouncements, RetryAnnouncementTelegram} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/announcements/", bytes.NewBufferString("{"))
		handler(ctx)
		require.Equal(t, http.StatusBadRequest, recorder.Code)
	}
}

package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/console_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupAnnouncementTelegramClient(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	oldBase, oldToken, oldClient := telegramAPIBaseURL, common.TelegramBotToken, httpClient
	oldWorker, oldKey := system_setting.WorkerUrl, system_setting.WorkerValidKey
	oldAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	t.Cleanup(func() {
		telegramAPIBaseURL, common.TelegramBotToken, httpClient = oldBase, oldToken, oldClient
		system_setting.WorkerUrl, system_setting.WorkerValidKey = oldWorker, oldKey
		system_setting.WorkerAllowHttpImageRequestEnabled = oldAllowHTTP
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	telegramAPIBaseURL, common.TelegramBotToken, httpClient = server.URL, "secret-bot-token", server.Client()
	system_setting.WorkerUrl, system_setting.WorkerValidKey = "", ""
}

func TestAnnouncementTelegramPublishesSeparatePlainText(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/botsecret-bot-token/sendMessage", r.URL.Path)
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.Equal(t, "@channel_name", payload["chat_id"])
		require.Equal(t, "Full news\n😀 <plain text>", payload["text"])
		require.NotContains(t, payload, "parse_mode")
		require.NotContains(t, payload, "message_id")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42,"chat":{"id":-100123}}}`))
	})
	result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{
		Channel: "@channel_name", AttemptText: "Full news\n😀 <plain text>",
	})
	require.Equal(t, "sent", result.Status)
	require.Equal(t, int64(-100123), result.ChatID)
	require.Equal(t, 42, result.MessageID)
	require.Equal(t, "https://t.me/channel_name/42", result.TelegramURL)
}

func TestAnnouncementTelegramEditsExistingMessage(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/botsecret-bot-token/editMessageText", r.URL.Path)
		var payload map[string]any
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.Equal(t, float64(-100123), payload["chat_id"])
		require.Equal(t, float64(42), payload["message_id"])
		require.Equal(t, "Edited", payload["text"])
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42,"chat":{"id":-100123}}}`))
	})
	result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{
		Channel: "@original_channel", ChatID: -100123, MessageID: 42, AttemptText: "Edited",
	})
	require.Equal(t, "sent", result.Status)
	require.Equal(t, "https://t.me/original_channel/42", result.TelegramURL)
}

func TestAnnouncementTelegramNotModifiedIsSuccess(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":400,"description":"Bad Request: message is not modified"}`))
	})
	result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{
		ChatID: -100123, MessageID: 42, TelegramURL: "https://t.me/channel_name/42", AttemptText: "Same",
	})
	require.Equal(t, "sent", result.Status)
	require.Equal(t, 42, result.MessageID)
	require.Equal(t, "https://t.me/channel_name/42", result.TelegramURL)
}

func TestAnnouncementTelegramRejectsAndRedactsErrors(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"ok":false,"error_code":403,"description":"secret-bot-token cannot post"}`))
	})
	result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{Channel: "@channel_name", AttemptText: "News"})
	require.Equal(t, "failed", result.Status)
	require.Contains(t, result.Error, "administrator permissions")
	require.NotContains(t, result.Error, "secret-bot-token")
}

func TestAnnouncementTelegramAmbiguousResultsDoNotPretendSuccess(t *testing.T) {
	for _, response := range []string{`gateway failed`, `{"ok":true}`, `{"ok":false}`} {
		t.Run(response, func(t *testing.T) {
			setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) })
			result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{Channel: "@channel_name", AttemptText: "News"})
			require.Equal(t, "unknown", result.Status)
			require.Empty(t, result.TelegramURL)
		})
	}
}

func TestAnnouncementTelegramTransportFailureIsUnknownOnlyForSend(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		_, bounded := request.Context().Deadline()
		require.True(t, bounded)
		return nil, errors.New("transport failed: secret-bot-token")
	})
	for _, tt := range []struct {
		messageID int
		status    string
	}{{0, "unknown"}, {42, "failed"}} {
		result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{MessageID: tt.messageID, ChatID: -100123, Channel: "@channel_name", AttemptText: "News"})
		require.Equal(t, tt.status, result.Status)
		require.NotContains(t, result.Error, "secret-bot-token")
	}
}

func TestAnnouncementTelegramWorkerPreservesPayloadAndDeadline(t *testing.T) {
	setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
		var forwarded WorkerRequest
		require.NoError(t, common.DecodeJson(r.Body, &forwarded))
		require.Equal(t, "https://api.telegram.org/botsecret-bot-token/sendMessage", forwarded.URL)
		require.Equal(t, "worker-secret", forwarded.Key)
		require.Equal(t, http.MethodPost, forwarded.Method)
		var payload map[string]any
		require.NoError(t, common.Unmarshal(forwarded.Body, &payload))
		require.Equal(t, "@channel_name", payload["chat_id"])
		require.Equal(t, "News", payload["text"])
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":9,"chat":{"id":-100123}}}`))
	})
	system_setting.WorkerUrl = telegramAPIBaseURL
	system_setting.WorkerValidKey = "worker-secret"
	telegramAPIBaseURL = "https://api.telegram.org"
	transport := httpClient.Transport
	httpClient.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		_, bounded := request.Context().Deadline()
		require.True(t, bounded)
		return transport.RoundTrip(request)
	})
	result := publishAnnouncementTelegram(context.Background(), model.AnnouncementTelegramDelivery{Channel: "@channel_name", AttemptText: "News"})
	require.Equal(t, "sent", result.Status)
}

func TestAnnouncementTelegramWorkerRecoveryAndRepeat(t *testing.T) {
	for _, tt := range []struct {
		name, initialStatus, expectedStatus string
		messageID, calls                    int
	}{
		{"new publication", "pending", "sent", 0, 1},
		{"interrupted publication", "sending", "unknown", 0, 0},
		{"interrupted edit", "sending", "sent", 42, 1},
		{"explicit failure waits for manual retry", "failed", "failed", 0, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			require.NoError(t, db.AutoMigrate(&model.Option{}, &model.SystemTask{}, &model.SystemTaskLock{}, &model.AnnouncementTelegramDelivery{}))
			oldDB := model.DB
			oldConsole := *console_setting.GetConsoleSetting()
			common.OptionMapRWMutex.Lock()
			oldOptions := common.OptionMap
			common.OptionMap = make(map[string]string)
			common.OptionMapRWMutex.Unlock()
			model.DB = db
			t.Cleanup(func() {
				model.DB = oldDB
				*console_setting.GetConsoleSetting() = oldConsole
				common.OptionMapRWMutex.Lock()
				common.OptionMap = oldOptions
				common.OptionMapRWMutex.Unlock()
				_ = sqlDB.Close()
			})
			calls := 0
			setupAnnouncementTelegramClient(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				expectedMethod := "sendMessage"
				if tt.messageID != 0 {
					expectedMethod = "editMessageText"
				}
				require.Equal(t, "/botsecret-bot-token/"+expectedMethod, r.URL.Path)
				_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42,"chat":{"id":-100123}}}`))
			})
			delivery := model.AnnouncementTelegramDelivery{AnnouncementID: "news", Mode: "bot", Channel: "@channel_name", DesiredText: "News", DesiredVersion: 1, AttemptText: "News", AttemptVersion: 1, Status: tt.initialStatus, AttemptOwner: "previous-task", MessageID: tt.messageID, ChatID: -100123}
			require.NoError(t, db.Create(&delivery).Error)
			for range 2 {
				task, err := model.CreateSystemTask(announcementTelegramTaskType, nil, nil)
				require.NoError(t, err)
				task, claimed, err := model.ClaimSystemTask(task.ID, announcementTelegramTaskType, "runner", time.Now().Unix()+60)
				require.NoError(t, err)
				require.True(t, claimed)
				announcementTelegramHandler{}.Run(context.Background(), task, "runner")
				finished, err := model.GetSystemTaskByTaskID(task.TaskID)
				require.NoError(t, err)
				require.Equal(t, model.SystemTaskStatusSucceeded, finished.Status)
			}
			require.Equal(t, tt.calls, calls)
			require.NoError(t, db.First(&delivery, "announcement_id = ?", "news").Error)
			require.Equal(t, tt.expectedStatus, delivery.Status)
			work, err := model.HasAnnouncementTelegramWork()
			require.NoError(t, err)
			require.False(t, work)
		})
	}
}

package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/require"
)

func TestSendTelegramNotify(t *testing.T) {
	oldBaseURL := telegramAPIBaseURL
	oldToken := common.TelegramBotToken
	oldClient := httpClient
	t.Cleanup(func() {
		telegramAPIBaseURL = oldBaseURL
		common.TelegramBotToken = oldToken
		httpClient = oldClient
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/botbot-token/sendMessage", r.URL.Path)
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))

		payload := struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}{}
		require.NoError(t, common.DecodeJson(r.Body, &payload))
		require.Equal(t, "-100123", payload.ChatID)
		require.Equal(t, "Quota warning\n\nRemaining: 42", payload.Text)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(server.Close)

	telegramAPIBaseURL = server.URL
	common.TelegramBotToken = "bot-token"
	httpClient = server.Client()

	require.NoError(t, sendTelegramNotify("-100123", dto.NewNotify("quota_exceed", "Quota warning", "Remaining: {{value}}", []interface{}{42})))
}

func TestSendTelegramNotifyUsesWorker(t *testing.T) {
	oldBaseURL := telegramAPIBaseURL
	oldToken := common.TelegramBotToken
	oldClient := httpClient
	oldWorkerURL := system_setting.WorkerUrl
	oldWorkerKey := system_setting.WorkerValidKey
	oldAllowHTTP := system_setting.WorkerAllowHttpImageRequestEnabled
	t.Cleanup(func() {
		telegramAPIBaseURL = oldBaseURL
		common.TelegramBotToken = oldToken
		httpClient = oldClient
		system_setting.WorkerUrl = oldWorkerURL
		system_setting.WorkerValidKey = oldWorkerKey
		system_setting.WorkerAllowHttpImageRequestEnabled = oldAllowHTTP
	})

	worker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		workerRequest := WorkerRequest{}
		require.NoError(t, common.DecodeJson(r.Body, &workerRequest))
		require.Equal(t, "worker-key", workerRequest.Key)
		require.Equal(t, http.MethodPost, workerRequest.Method)
		require.Equal(t, "application/json", workerRequest.Headers["Content-Type"])
		require.Equal(t, "https://example.com/botbot-token/sendMessage", workerRequest.URL)

		payload := struct {
			ChatID string `json:"chat_id"`
			Text   string `json:"text"`
		}{}
		require.NoError(t, common.Unmarshal(workerRequest.Body, &payload))
		require.Equal(t, "123", payload.ChatID)
		require.Equal(t, "Quota warning\n\nRemaining quota", payload.Text)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(worker.Close)

	telegramAPIBaseURL = "https://example.com"
	common.TelegramBotToken = "bot-token"
	httpClient = worker.Client()
	system_setting.WorkerUrl = worker.URL
	system_setting.WorkerValidKey = "worker-key"
	system_setting.WorkerAllowHttpImageRequestEnabled = false

	require.NoError(t, sendTelegramNotify("123", dto.NewNotify("quota_exceed", "Quota warning", "Remaining quota", nil)))
}

func TestSendTelegramNotifyReturnsTelegramErrors(t *testing.T) {
	oldBaseURL := telegramAPIBaseURL
	oldToken := common.TelegramBotToken
	oldClient := httpClient
	t.Cleanup(func() {
		telegramAPIBaseURL = oldBaseURL
		common.TelegramBotToken = oldToken
		httpClient = oldClient
	})

	tests := []struct {
		name       string
		statusCode int
		response   string
		expected   string
	}{
		{name: "telegram API error", statusCode: http.StatusOK, response: `{"ok":false,"description":"chat not found"}`, expected: "chat not found"},
		{name: "HTTP error", statusCode: http.StatusBadGateway, response: `{"ok":false}`, expected: "status code: 502"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.response))
			}))
			t.Cleanup(server.Close)

			telegramAPIBaseURL = server.URL
			common.TelegramBotToken = "bot-token"
			httpClient = server.Client()

			err := sendTelegramNotify("123", dto.NewNotify("quota_exceed", "Quota warning", "Remaining quota", nil))
			require.ErrorContains(t, err, tt.expected)
		})
	}
}

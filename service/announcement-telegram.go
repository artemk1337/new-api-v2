package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

const announcementTelegramTaskType = "announcement_telegram"

type announcementTelegramHandler struct{}

func (announcementTelegramHandler) Type() string            { return announcementTelegramTaskType }
func (announcementTelegramHandler) Interval() time.Duration { return 15 * time.Second }
func (announcementTelegramHandler) NewPayload() any         { return nil }

func (announcementTelegramHandler) Enabled() bool {
	work, err := model.HasAnnouncementTelegramWork()
	return err == nil && work
}

func init() {
	RegisterSystemTaskHandler(announcementTelegramHandler{})
}

// EnqueueAnnouncementTelegramDelivery wakes the durable publication worker.
// The scheduled handler also picks up intentions if enqueue fails after commit.
func EnqueueAnnouncementTelegramDelivery() error {
	_, _, err := EnqueueSystemTask(announcementTelegramTaskType, nil)
	return err
}

func (announcementTelegramHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	if err := model.RecoverAnnouncementTelegramDeliveries(task.TaskID); err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	for ctx.Err() == nil {
		delivery, found, err := model.ClaimAnnouncementTelegramDelivery(task.TaskID)
		if err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
		if !found {
			if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, nil, ""); err != nil {
				logSystemTaskLockError(ctx, task, err)
			}
			return
		}
		result := publishAnnouncementTelegram(ctx, delivery)
		if err := model.FinishAnnouncementTelegramDelivery(delivery, result); err != nil {
			failSystemTask(task, runnerID, err)
			return
		}
	}
	failSystemTask(task, runnerID, ctx.Err())
}

type announcementTelegramResponse struct {
	OK          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		MessageID int `json:"message_id"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
	} `json:"result"`
}

func publishAnnouncementTelegram(ctx context.Context, delivery model.AnnouncementTelegramDelivery) model.AnnouncementTelegramResult {
	result := model.AnnouncementTelegramResult{Status: "failed"}
	if strings.TrimSpace(common.TelegramBotToken) == "" {
		result.Error = "Telegram bot token is not configured"
		return result
	}
	method := "sendMessage"
	var chatID any = delivery.Channel
	if delivery.MessageID != 0 {
		method = "editMessageText"
		if delivery.ChatID != 0 {
			chatID = delivery.ChatID
		}
	}
	payload := struct {
		ChatID    any    `json:"chat_id"`
		MessageID int    `json:"message_id,omitempty"`
		Text      string `json:"text"`
	}{chatID, delivery.MessageID, delivery.AttemptText}
	body, err := common.Marshal(payload)
	if err != nil {
		result.Error = "Unable to encode the Telegram publication"
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	endpoint := telegramAPIBaseURL + "/bot" + common.TelegramBotToken + "/" + method
	var response *http.Response
	if system_setting.EnableWorker() {
		response, err = DoWorkerRequestWithContext(ctx, &WorkerRequest{
			URL: endpoint, Key: system_setting.WorkerValidKey, Method: http.MethodPost,
			Headers: map[string]string{"Content-Type": "application/json"}, Body: body,
		})
	} else {
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if requestErr != nil {
			result.Error = "Unable to prepare the Telegram publication"
			return result
		}
		request.Header.Set("Content-Type", "application/json")
		response, err = GetHttpClient().Do(request)
	}
	if err != nil {
		return announcementTelegramUnconfirmed(delivery, "Telegram request did not complete; check the channel before retrying")
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return announcementTelegramUnconfirmed(delivery, "Unable to read the Telegram result; check the channel before retrying")
	}
	var parsed announcementTelegramResponse
	if err := common.Unmarshal(responseBody, &parsed); err != nil {
		return announcementTelegramUnconfirmed(delivery, "Invalid Telegram result; check the channel before retrying")
	}
	if !parsed.OK {
		if delivery.MessageID != 0 && strings.Contains(parsed.Description, "message is not modified") {
			return model.AnnouncementTelegramResult{Status: "sent", ChatID: delivery.ChatID, MessageID: delivery.MessageID, TelegramURL: delivery.TelegramURL}
		}
		if parsed.ErrorCode == 0 {
			return announcementTelegramUnconfirmed(delivery, "Unconfirmed Telegram result; check the channel before retrying")
		}
		result.Error = "Telegram rejected the publication. Check the channel, bot permissions and text."
		switch parsed.ErrorCode {
		case http.StatusUnauthorized:
			result.Error = "Telegram bot authentication failed. Check the bot token."
		case http.StatusForbidden:
			result.Error = "Telegram bot cannot publish to this channel. Check its administrator permissions."
		case http.StatusTooManyRequests:
			result.Error = "Telegram rate limit reached. Try again later."
		}
		if delivery.MessageID != 0 && (strings.Contains(parsed.Description, "message to edit not found") || strings.Contains(parsed.Description, "message can't be edited")) {
			result.Error = "Telegram post is unavailable. Check the original message before retrying."
		}
		return result
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || parsed.Result.MessageID <= 0 || parsed.Result.Chat.ID == 0 {
		return announcementTelegramUnconfirmed(delivery, "Incomplete Telegram result; check the channel before retrying")
	}
	return model.AnnouncementTelegramResult{
		Status: "sent", ChatID: parsed.Result.Chat.ID, MessageID: parsed.Result.MessageID,
		TelegramURL: fmt.Sprintf("https://t.me/%s/%d", strings.TrimPrefix(delivery.Channel, "@"), parsed.Result.MessageID),
	}
}

func announcementTelegramUnconfirmed(delivery model.AnnouncementTelegramDelivery, message string) model.AnnouncementTelegramResult {
	status := "unknown"
	if delivery.MessageID != 0 {
		status = "failed"
	}
	return model.AnnouncementTelegramResult{Status: status, Error: message}
}

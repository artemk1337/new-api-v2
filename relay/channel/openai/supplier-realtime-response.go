package openai

import (
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type supplierRealtimeSession struct {
	dto.RealtimeSession
	ID                      string          `json:"id,omitempty"`
	Object                  string          `json:"object,omitempty"`
	Model                   string          `json:"model"`
	ExpiresAt               *int64          `json:"expires_at,omitempty"`
	OutputModalities        []string        `json:"output_modalities,omitempty"`
	Audio                   json.RawMessage `json:"audio,omitempty"`
	MaxResponseOutputTokens json.RawMessage `json:"max_response_output_tokens,omitempty"`
}

type supplierRealtimeResponse struct {
	ID                string                    `json:"id,omitempty"`
	Object            string                    `json:"object,omitempty"`
	Model             string                    `json:"model,omitempty"`
	Status            string                    `json:"status,omitempty"`
	StatusDetails     json.RawMessage           `json:"status_details,omitempty"`
	Output            []supplierResponsesOutput `json:"output,omitempty"`
	Usage             *dto.RealtimeUsage        `json:"usage,omitempty"`
	ConversationID    string                    `json:"conversation_id,omitempty"`
	Modalities        []string                  `json:"modalities,omitempty"`
	Voice             string                    `json:"voice,omitempty"`
	OutputAudioFormat string                    `json:"output_audio_format,omitempty"`
	MaxOutputTokens   json.RawMessage           `json:"max_output_tokens,omitempty"`
}

type supplierRealtimeEvent struct {
	Type           string                    `json:"type"`
	EventID        string                    `json:"event_id,omitempty"`
	Session        *supplierRealtimeSession  `json:"session,omitempty"`
	Response       *supplierRealtimeResponse `json:"response,omitempty"`
	Item           *supplierResponsesOutput  `json:"item,omitempty"`
	ItemID         string                    `json:"item_id,omitempty"`
	PreviousItemID string                    `json:"previous_item_id,omitempty"`
	ResponseID     string                    `json:"response_id,omitempty"`
	OutputIndex    *int                      `json:"output_index,omitempty"`
	ContentIndex   *int                      `json:"content_index,omitempty"`
	AudioStartMS   *int                      `json:"audio_start_ms,omitempty"`
	AudioEndMS     *int                      `json:"audio_end_ms,omitempty"`
	Delta          json.RawMessage           `json:"delta,omitempty"`
	Part           json.RawMessage           `json:"part,omitempty"`
	Audio          *string                   `json:"audio,omitempty"`
	Text           *string                   `json:"text,omitempty"`
	Transcript     *string                   `json:"transcript,omitempty"`
	Arguments      *string                   `json:"arguments,omitempty"`
	Name           string                    `json:"name,omitempty"`
	CallID         string                    `json:"call_id,omitempty"`
	Error          *types.OpenAIError        `json:"error,omitempty"`
}

func projectSupplierRealtimeEvent(c *gin.Context, info *relaycommon.RelayInfo, data []byte) ([]byte, error) {
	var event supplierRealtimeEvent
	if err := common.Unmarshal(data, &event); err != nil {
		return nil, errSupplierResponse
	}
	if event.Error != nil || event.Type == "error" || event.Response != nil && event.Response.Status == "failed" {
		event = supplierRealtimeEvent{
			Type: "error", EventID: helper.GetLocalRealtimeID(c),
			Error: &types.OpenAIError{Message: "Request could not be completed", Type: "upstream_error", Code: "request_failed"},
		}
	}
	if event.Session != nil {
		event.Session.Model = supplierPublicModel(c, info)
	}
	if event.Response != nil {
		event.Response.Model = supplierPublicModel(c, info)
		// Status details are service diagnostics; generated output remains in
		// the output array and usage retains only typed token counters.
		event.Response.StatusDetails = nil
	}
	return common.Marshal(event)
}

package openai

import (
	"bytes"
	"encoding/json"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/gin-gonic/gin"
)

// Output payloads and encrypted compaction state are user data. Keep these
// lanes opaque while projecting the service envelope and each output item.
type supplierResponsesOutput struct {
	Type                string          `json:"type"`
	ID                  string          `json:"id,omitempty"`
	Status              string          `json:"status,omitempty"`
	Role                string          `json:"role,omitempty"`
	Content             json.RawMessage `json:"content,omitempty"`
	Summary             json.RawMessage `json:"summary,omitempty"`
	EncryptedContent    string          `json:"encrypted_content,omitempty"`
	CallID              string          `json:"call_id,omitempty"`
	Name                string          `json:"name,omitempty"`
	Arguments           json.RawMessage `json:"arguments,omitempty"`
	Result              json.RawMessage `json:"result,omitempty"`
	Action              json.RawMessage `json:"action,omitempty"`
	Queries             []string        `json:"queries,omitempty"`
	Results             json.RawMessage `json:"results,omitempty"`
	Code                string          `json:"code,omitempty"`
	ContainerID         string          `json:"container_id,omitempty"`
	Outputs             json.RawMessage `json:"outputs,omitempty"`
	Error               json.RawMessage `json:"error,omitempty"`
	ServerLabel         string          `json:"server_label,omitempty"`
	ApprovalRequestID   string          `json:"approval_request_id,omitempty"`
	PendingSafetyChecks json.RawMessage `json:"pending_safety_checks,omitempty"`
	Quality             string          `json:"quality,omitempty"`
	Size                string          `json:"size,omitempty"`
	Background          string          `json:"background,omitempty"`
	OutputFormat        string          `json:"output_format,omitempty"`
	RevisedPrompt       string          `json:"revised_prompt,omitempty"`
}

type supplierResponsesEnvelope struct {
	ID                 string                    `json:"id"`
	Object             string                    `json:"object"`
	CreatedAt          int64                     `json:"created_at"`
	CompletedAt        *int64                    `json:"completed_at,omitempty"`
	Status             string                    `json:"status,omitempty"`
	Model              string                    `json:"model,omitempty"`
	Output             []supplierResponsesOutput `json:"output"`
	Usage              *dto.Usage                `json:"usage,omitempty"`
	Error              json.RawMessage           `json:"error,omitempty"`
	IncompleteDetails  *dto.IncompleteDetails    `json:"incomplete_details,omitempty"`
	MaxOutputTokens    *int                      `json:"max_output_tokens,omitempty"`
	ParallelToolCalls  *bool                     `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID json.RawMessage           `json:"previous_response_id,omitempty"`
	Reasoning          *dto.Reasoning            `json:"reasoning,omitempty"`
	Store              *bool                     `json:"store,omitempty"`
	Temperature        *float64                  `json:"temperature,omitempty"`
	ToolChoice         json.RawMessage           `json:"tool_choice,omitempty"`
	TopP               *float64                  `json:"top_p,omitempty"`
	Truncation         json.RawMessage           `json:"truncation,omitempty"`
	// Only original caller data may be echoed in these fields.
	Instructions json.RawMessage `json:"instructions,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	Tools        json.RawMessage `json:"tools,omitempty"`
	User         json.RawMessage `json:"user,omitempty"`
}

type supplierResponsesEvent struct {
	Type           string                   `json:"type"`
	SequenceNumber *int                     `json:"sequence_number,omitempty"`
	Response       json.RawMessage          `json:"response,omitempty"`
	ResponseID     string                   `json:"response_id,omitempty"`
	OutputIndex    *int                     `json:"output_index,omitempty"`
	ContentIndex   *int                     `json:"content_index,omitempty"`
	SummaryIndex   *int                     `json:"summary_index,omitempty"`
	ItemID         string                   `json:"item_id,omitempty"`
	Item           *supplierResponsesOutput `json:"item,omitempty"`
	Part           json.RawMessage          `json:"part,omitempty"`
	Delta          json.RawMessage          `json:"delta,omitempty"`
	Text           *string                  `json:"text,omitempty"`
	Arguments      json.RawMessage          `json:"arguments,omitempty"`
	Name           string                   `json:"name,omitempty"`
	CallID         string                   `json:"call_id,omitempty"`
	Logprobs       json.RawMessage          `json:"logprobs,omitempty"`
	Error          json.RawMessage          `json:"error,omitempty"`
}

func supplierHasError(raw json.RawMessage) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func projectSupplierResponses(c *gin.Context, info *relaycommon.RelayInfo, data []byte, compact bool) ([]byte, error) {
	var response supplierResponsesEnvelope
	if err := common.Unmarshal(data, &response); err != nil || supplierHasError(response.Error) || response.Status == "failed" {
		return nil, errSupplierResponse
	}
	// Preserve opaque response/item IDs: previous_response_id and encrypted
	// output items must remain usable in the next request.
	response.Model = supplierPublicModel(c, info)
	response.Object = "response"
	if compact {
		response.Object = "response.compaction"
	}
	response.Instructions, response.Metadata, response.Tools, response.User = nil, nil, nil, nil
	if request, ok := info.Request.(*dto.OpenAIResponsesRequest); ok {
		response.Instructions = request.Instructions
		response.Metadata = request.Metadata
		response.Tools = request.Tools
		response.User = request.User
	}
	if response.Usage != nil {
		usage := supplierPublicUsage(*response.Usage)
		response.Usage = &usage
	}
	return common.Marshal(response)
}

func projectSupplierResponsesEvent(c *gin.Context, info *relaycommon.RelayInfo, data []byte) ([]byte, error) {
	var event supplierResponsesEvent
	if err := common.Unmarshal(data, &event); err != nil || supplierHasError(event.Error) {
		return nil, errSupplierResponse
	}
	switch event.Type {
	case "error", "response.failed", "response.error":
		return nil, errSupplierResponse
	}
	if supplierHasError(event.Response) {
		response, err := projectSupplierResponses(c, info, event.Response, false)
		if err != nil {
			return nil, err
		}
		event.Response = response
	}
	return common.Marshal(event)
}

func writeSupplierResponsesError(c *gin.Context) {
	data := `{"type":"error","code":"request_failed","message":"Request could not be completed","param":null}`
	helper.ResponseChunkData(c, dto.ResponsesStreamResponse{Type: "error"}, data)
}

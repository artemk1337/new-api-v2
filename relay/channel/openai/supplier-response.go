package openai

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var errSupplierResponse = errors.New("upstream request could not be completed")

// Content and tool arguments are generated output. Keep them opaque rather
// than recursively removing words that also occur in legitimate user data.
type supplierChatMessage struct {
	Role             string                 `json:"role,omitempty"`
	Content          json.RawMessage        `json:"content,omitempty"`
	Reasoning        *string                `json:"reasoning,omitempty"`
	ReasoningContent *string                `json:"reasoning_content,omitempty"`
	Refusal          *string                `json:"refusal,omitempty"`
	Name             *string                `json:"name,omitempty"`
	ToolCalls        []supplierToolCall     `json:"tool_calls,omitempty"`
	FunctionCall     *supplierFunctionCall  `json:"function_call,omitempty"`
	Audio            *supplierAudioResponse `json:"audio,omitempty"`
	Annotations      json.RawMessage        `json:"annotations,omitempty"`
}

type supplierToolCall struct {
	Index    *int                 `json:"index,omitempty"`
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function supplierFunctionCall `json:"function"`
}

type supplierFunctionCall struct {
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments"`
}

type supplierAudioResponse struct {
	ID         string `json:"id,omitempty"`
	Data       string `json:"data,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`
	Transcript string `json:"transcript,omitempty"`
}

type supplierChatChoice struct {
	Index        int                  `json:"index"`
	Message      *supplierChatMessage `json:"message,omitempty"`
	Delta        *supplierChatMessage `json:"delta,omitempty"`
	Text         *string              `json:"text,omitempty"`
	FinishReason *string              `json:"finish_reason"`
	Logprobs     *supplierLogprobs    `json:"logprobs,omitempty"`
}

type supplierLogprobs struct {
	Content       []supplierTokenLogprob `json:"content,omitempty"`
	Refusal       []supplierTokenLogprob `json:"refusal,omitempty"`
	Tokens        []string               `json:"tokens,omitempty"`
	TokenLogprobs []*float64             `json:"token_logprobs,omitempty"`
	TopLogprobs   []map[string]float64   `json:"top_logprobs,omitempty"`
	TextOffset    []int                  `json:"text_offset,omitempty"`
}

type supplierTokenLogprob struct {
	Token       string                 `json:"token"`
	Logprob     float64                `json:"logprob"`
	Bytes       []int                  `json:"bytes"`
	TopLogprobs []supplierTokenLogprob `json:"top_logprobs,omitempty"`
}

type supplierChatResponse struct {
	ID      string               `json:"id"`
	Object  string               `json:"object"`
	Created int64                `json:"created"`
	Model   string               `json:"model"`
	Choices []supplierChatChoice `json:"choices"`
	Usage   *dto.Usage           `json:"usage,omitempty"`
	Error   json.RawMessage      `json:"error,omitempty"`
}

func supplierResponseBound(c *gin.Context, info *relaycommon.RelayInfo) bool {
	return c.GetBool("supplier_pricing") || info != nil && info.PriceData.SupplierTariff != nil
}

func supplierPublicModel(c *gin.Context, info *relaycommon.RelayInfo) string {
	if info.RequestedModelName != "" {
		return info.RequestedModelName
	}
	if name := common.GetContextKeyString(c, constant.ContextKeyRequestedModel); name != "" {
		return name
	}
	return info.OriginModelName
}

func supplierPublicUsage(usage dto.Usage) dto.Usage {
	usage.Cost = nil
	usage.UsageSource = ""
	usage.UsageSemantic = ""
	return usage
}

// Project the protocol envelope through an allowlist. Unrecognized upstream
// service fields cannot become public when a provider adds new metadata.
func projectSupplierChatResponse(c *gin.Context, info *relaycommon.RelayInfo, data []byte, stream bool, usage *dto.Usage) ([]byte, error) {
	var response supplierChatResponse
	if err := common.Unmarshal(data, &response); err != nil {
		return nil, errSupplierResponse
	}
	if len(response.Error) > 0 && !bytes.Equal(bytes.TrimSpace(response.Error), []byte("null")) {
		return nil, errSupplierResponse
	}
	response.ID = helper.GetResponseID(c)
	response.Model = supplierPublicModel(c, info)
	response.Object = "chat.completion"
	if stream {
		response.Object = "chat.completion.chunk"
	}
	if info.RelayMode == relayconstant.RelayModeCompletions {
		response.Object = "text_completion"
	}
	if usage != nil {
		response.Usage = usage
	}
	if response.Usage != nil {
		publicUsage := supplierPublicUsage(*response.Usage)
		response.Usage = &publicUsage
	}
	return common.Marshal(response)
}

func supplierStreamErrorData(c *gin.Context) error {
	return helper.ObjectData(c, struct {
		Error types.OpenAIError `json:"error"`
	}{Error: types.OpenAIError{Message: "Request could not be completed", Type: "upstream_error", Code: "request_failed"}})
}

func writeChatStreamResponse(c *gin.Context, info *relaycommon.RelayInfo, response *dto.ChatCompletionsStreamResponse) error {
	if !supplierResponseBound(c, info) {
		return helper.ObjectData(c, response)
	}
	data, err := common.Marshal(response)
	if err != nil {
		return err
	}
	data, err = projectSupplierChatResponse(c, info, data, true, nil)
	if err != nil {
		return err
	}
	return helper.StringData(c, string(data))
}

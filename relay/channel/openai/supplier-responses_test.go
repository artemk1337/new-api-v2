package openai

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const supplierResponsesJSON = `{"id":"resp_continuation","object":"response","created_at":1710000000,"status":"completed","model":"private-model","provider":"private-source","metadata":{"supplier_id":7},"tools":[{"type":"private-service","url":"private-source"}],"output":[{"type":"message","id":"msg_1","role":"assistant","metadata":{"provider":"private-source"},"content":[{"type":"output_text","text":"Preserve generated metadata text","metadata":{"user":"keep"}}]},{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup","arguments":"{\"provider\":\"keep\"}","cost":0.006},{"type":"reasoning","id":"rs_1","encrypted_content":"opaque-continuation-state","metadata":{"source":"private-source"}}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006,"usage_source":"private-source"}}`

func TestSupplierResponsesJSONProjectsNestedOutputAndKeepsContinuation(t *testing.T) {
	c, recorder, resp, info := newResponsesChatTestContext(t, supplierResponsesJSON, false)
	info.RequestedModelName = "public-alias"
	info.RelayMode = relayconstant.RelayModeResponses
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
	info.Request = &dto.OpenAIResponsesRequest{Metadata: []byte(`{"user":"original"}`)}
	resp.Header.Set("Server", "private-source")

	usage, apiErr := OaiResponsesHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	var wire map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &wire))
	require.Equal(t, "public-alias", wire["model"])
	require.Equal(t, "resp_continuation", wire["id"])
	require.Equal(t, map[string]any{"user": "original"}, wire["metadata"])
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
	output := wire["output"].([]any)
	require.Equal(t, "opaque-continuation-state", output[2].(map[string]any)["encrypted_content"])
	require.Equal(t, `{"provider":"keep"}`, output[1].(map[string]any)["arguments"])
	content := output[0].(map[string]any)["content"].([]any)[0].(map[string]any)
	require.Equal(t, map[string]any{"user": "keep"}, content["metadata"])
	require.Empty(t, recorder.Header().Get("Server"))
}

func TestSupplierResponsesSSEProjectsNestedResponseAndOutputEvents(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := strings.Join([]string{
		`data: {"type":"response.created","sequence_number":0,"provider":"private-source","response":{"id":"resp_continuation","model":"private-model","metadata":{"supplier_id":7},"output":[]}}`,
		`data: {"type":"response.output_item.added","output_index":0,"metadata":{"provider":"private-source"},"item":{"type":"function_call","id":"fc_1","name":"lookup","arguments":"{\"metadata\":\"keep\"}","provider":"private-source"}}`,
		`data: {"type":"response.output_text.delta","delta":"Preserve generated provider metadata text","metadata":{"provider":"private-source"}}`,
		`data: {"type":"response.completed","response":` + supplierResponsesJSON + `}`, `data: [DONE]`, ``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RequestedModelName = "public-alias"
	info.RelayMode = relayconstant.RelayModeResponses
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	wire := recorder.Body.String()
	require.Contains(t, wire, `"model":"public-alias"`)
	require.Contains(t, wire, `"sequence_number":0`)
	require.Contains(t, wire, "opaque-continuation-state")
	require.Contains(t, wire, "Preserve generated provider metadata text")
	require.NotContains(t, wire, "private-")
	require.NotContains(t, wire, `"cost"`)
	require.NotContains(t, wire, `"supplier_id"`)
}

func TestSupplierResponsesSSEErrorsAreGeneric(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	for _, event := range []string{
		`{"type":"error","message":"private-source credit exhausted","error":{"metadata":{"cost":0.006}}}`,
		`{"type":"response.failed","response":{"model":"private-model","error":{"message":"private-source credit exhausted","type":"private-source"}}}`,
	} {
		c, recorder, resp, info := newResponsesChatTestContext(t, "data: "+event+"\n\ndata: [DONE]\n", true)
		info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
		_, apiErr := OaiResponsesStreamHandler(c, info, resp)
		require.NotNil(t, apiErr)
		require.Contains(t, recorder.Body.String(), `"code":"request_failed"`)
		require.NotContains(t, recorder.Body.String(), "private-")
		require.NotContains(t, recorder.Body.String(), "0.006")
	}
}

func TestSupplierCompactionKeepsEncryptedOutputWithoutProcurementMetadata(t *testing.T) {
	body := `{"id":"cmp_1","object":"response.compaction","created_at":1710000000,"provider":"private-source","metadata":{"supplier_id":7},"output":[{"type":"compaction","id":"cmp_item_1","encrypted_content":"opaque-compacted-state","metadata":{"source":"private-source"}}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5,"cost":0.006}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.RequestedModelName = "public-alias"
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage, apiErr := OaiResponsesCompactionHandler(c, resp, info)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "opaque-compacted-state")
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), `"cost"`)
}

func TestUnboundResponsesKeepExistingWireContract(t *testing.T) {
	c, recorder, resp, info := newResponsesChatTestContext(t, supplierResponsesJSON, false)
	_, apiErr := OaiResponsesHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.JSONEq(t, supplierResponsesJSON, recorder.Body.String())
}

func TestSupplierChatResponsesConversionsKeepPublicModelAlias(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	chatJSON := `{"id":"chatcmpl_1","object":"chat.completion","created":1710000000,"model":"private-model","choices":[{"index":0,"message":{"role":"assistant","content":"generated text"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.006}}`
	responsesSSE := "data: " + `{"type":"response.output_text.delta","delta":"generated text"}` + "\n\ndata: " + `{"type":"response.completed","response":` + supplierResponsesJSON + `}` + "\n\ndata: [DONE]\n"
	chatSSE := "data: " + `{"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"private-model","choices":[{"index":0,"delta":{"content":"generated text"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.006}}` + "\n\ndata: [DONE]\n"
	tests := []struct {
		name    string
		body    string
		stream  bool
		handler func(*gin.Context, *relaycommon.RelayInfo, *http.Response) (*dto.Usage, *types.NewAPIError)
	}{
		{"responses-to-chat", supplierResponsesJSON, false, OaiResponsesToChatHandler},
		{"responses-to-chat-buffered", responsesSSE, false, OaiResponsesToChatBufferedStreamHandler},
		{"responses-to-chat-stream", responsesSSE, true, OaiResponsesToChatStreamHandler},
		{"chat-to-responses", chatJSON, false, OaiChatToResponsesHandler},
		{"chat-to-responses-stream", chatSSE, true, OaiChatToResponsesStreamHandler},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, recorder, resp, info := newResponsesChatTestContext(t, test.body, test.stream)
			info.RequestedModelName = "public-alias"
			info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
			_, apiErr := test.handler(c, info, resp)
			require.Nil(t, apiErr)
			require.Contains(t, recorder.Body.String(), `"model":"public-alias"`)
			require.NotContains(t, recorder.Body.String(), "private-")
			require.NotContains(t, recorder.Body.String(), `"cost"`)
		})
	}
}

func TestSupplierEmbeddingAndModerationProjectionPreservesResults(t *testing.T) {
	for _, test := range []struct {
		mode int
		body string
		want string
	}{
		{relayconstant.RelayModeEmbeddings, `{"object":"list","model":"private-model","metadata":{"source":"private-source"},"data":[{"object":"embedding","index":0,"embedding":[0.1,0.2],"provider":"private-source"}],"usage":{"prompt_tokens":2,"total_tokens":2,"cost":0.006}}`, `"embedding":[0.1,0.2]`},
		{relayconstant.RelayModeModerations, `{"id":"modr_1","model":"private-model","metadata":{"source":"private-source"},"results":[{"flagged":false,"categories":{"violence":false},"category_scores":{"violence":0.01},"provider":"private-source"}],"usage":{"prompt_tokens":2,"total_tokens":2}}`, `"violence":false`},
	} {
		c, recorder, resp, info := newResponsesChatTestContext(t, test.body, false)
		info.RelayMode = test.mode
		info.RelayFormat = types.RelayFormatEmbedding
		info.RequestedModelName = "public-alias"
		info.PriceData.SupplierTariff = &types.SupplierModelTariff{}
		_, apiErr := OpenaiHandler(c, info, resp)
		require.Nil(t, apiErr)
		require.Contains(t, recorder.Body.String(), test.want)
		require.Contains(t, recorder.Body.String(), `"model":"public-alias"`)
		require.NotContains(t, recorder.Body.String(), "private-")
		require.NotContains(t, recorder.Body.String(), `"cost"`)
	}
}

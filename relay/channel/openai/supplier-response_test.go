package openai

import (
	"net/http"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestSupplierChatJSONProjectsEnvelopeAndPreservesGeneratedContent(t *testing.T) {
	body := `{"id":"private-provider-id","object":"chat.completion","created":1710000000,"model":"private-upstream-model","provider":"private-source","metadata":{"supplier_id":7},"system_fingerprint":"private-fingerprint","choices":[{"index":0,"message":{"role":"assistant","content":[{"type":"text","text":"Keep metadata and provider in generated content","metadata":{"user_value":"keep"}}],"metadata":{"source":"private-source"},"tool_calls":[{"id":"call_1","type":"function","metadata":{"source":"private-source"},"function":{"name":"lookup","arguments":"{\"provider\":\"keep\",\"cost\":4}","metadata":{"source":"private-source"}}}]},"finish_reason":"tool_calls","metadata":{"source":"private-source"}}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.006,"metadata":{"source":"private-source"},"usage_source":"private-source"}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.RequestedModelName = "public-alias"
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{SupplierID: 7, ProfileID: 8}
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Server", "private-source")
	resp.Header.Set("X-Provider", "private-source")
	resp.Header.Set("Set-Cookie", "provider_session=private-source")

	usage, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 0.006, usage.Cost, "billing still receives the unmodified upstream usage")
	require.Equal(t, "private-upstream-model", info.ProviderReturnedModelName)
	var wire map[string]any
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &wire))
	require.Equal(t, "public-alias", wire["model"])
	require.Equal(t, "chatcmpl-responses-test", wire["id"])
	require.NotContains(t, wire, "provider")
	require.NotContains(t, wire, "metadata")
	require.NotContains(t, wire, "system_fingerprint")
	require.NotContains(t, wire["usage"], "cost")
	require.NotContains(t, wire["usage"], "usage_source")
	require.NotContains(t, recorder.Body.String(), "private-source")
	message := wire["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	require.NotContains(t, message, "metadata")
	content := message["content"].([]any)[0].(map[string]any)
	require.Equal(t, map[string]any{"user_value": "keep"}, content["metadata"])
	call := message["tool_calls"].([]any)[0].(map[string]any)
	function := call["function"].(map[string]any)
	require.Equal(t, `{"provider":"keep","cost":4}`, function["arguments"])
	require.Empty(t, recorder.Header().Get("Server"))
	require.Empty(t, recorder.Header().Get("X-Provider"))
	require.Empty(t, recorder.Header().Get("Set-Cookie"))
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
}

func TestSupplierChatStreamSanitizesEveryFrameAndFinalUsage(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := strings.Join([]string{
		`data: {"id":"private-provider-id","object":"chat.completion.chunk","created":1710000000,"model":"private-upstream-model","provider":"private-source","metadata":{"supplier_id":7},"system_fingerprint":"private-fingerprint","choices":[{"index":0,"delta":{"role":"assistant","content":"Keep provider metadata in generated text","tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"metadata\":\"keep\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"private-provider-id","object":"chat.completion.chunk","created":1710000000,"model":"private-upstream-model","provider":"private-source","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.006,"usage_source":"private-source"}}`,
		`data: [DONE]`, ``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RequestedModelName = "public-alias"
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	usage, apiErr := OaiStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.Equal(t, 5, usage.TotalTokens)
	require.Equal(t, 0.006, usage.Cost)
	output := recorder.Body.String()
	require.Contains(t, output, `"model":"public-alias"`)
	require.Contains(t, output, "Keep provider metadata in generated text")
	require.Contains(t, output, `"arguments":"{\"metadata\":\"keep\"}"`)
	require.NotContains(t, output, "private-")
	require.NotContains(t, output, `"cost"`)
	require.NotContains(t, output, "system_fingerprint")
	require.Contains(t, output, "data: [DONE]")
}

func TestSupplierStreamUpstreamErrorIsGeneric(t *testing.T) {
	previousTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = previousTimeout })
	body := "data: " + `{"error":{"message":"private-supplier.example has insufficient credit","type":"private-source","code":"private-error","metadata":{"cost":0.006}},"provider":"private-source"}` + "\n\ndata: [DONE]\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Set("supplier_pricing", true)

	_, apiErr := OaiStreamHandler(c, info, resp)

	require.NotNil(t, apiErr)
	require.Contains(t, recorder.Body.String(), `"message":"Request could not be completed"`)
	require.Contains(t, recorder.Body.String(), `"code":"request_failed"`)
	require.NotContains(t, recorder.Body.String(), "private-")
	require.NotContains(t, recorder.Body.String(), "0.006")
	require.NotContains(t, recorder.Body.String(), "metadata")
}

func TestSupplierSyntheticStreamUsageDoesNotExposeSource(t *testing.T) {
	c, recorder, _, info := newResponsesChatTestContext(t, "", true)
	info.RequestedModelName = "public-alias"
	c.Set("supplier_pricing", true)
	usage := dto.Usage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3, Cost: 0.006, UsageSource: "private-source"}

	HandleFinalResponse(c, info, "", "private-source", 1710000000, "private-model", "private-fingerprint", &usage, false)

	output := recorder.Body.String()
	require.Contains(t, output, `"model":"public-alias"`)
	require.NotContains(t, output, "private-")
	require.NotContains(t, output, "system_fingerprint")
	require.NotContains(t, output, `"cost"`)
	require.Equal(t, 0.006, usage.Cost)
}

func TestUnboundChatRetainsExistingPassthrough(t *testing.T) {
	body := `{"id":"upstream-id","object":"chat.completion","created":1710000000,"model":"upstream-model","metadata":{"keep":true},"system_fingerprint":"keep","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5,"cost":0.006}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	resp.Header.Set("X-Provider", "existing-header")

	_, apiErr := OpenaiHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.JSONEq(t, body, recorder.Body.String())
	require.Equal(t, "existing-header", recorder.Header().Get("X-Provider"))
	c, recorder, _, info = newResponsesChatTestContext(t, "", true)
	require.NoError(t, sendStreamData(c, info, body, false, false))
	require.Contains(t, recorder.Body.String(), body)
}

func TestSupplierChatJSONErrorIsNotWrittenToDownstream(t *testing.T) {
	body := `{"error":{"message":"private-source exhausted credits","type":"private-source","metadata":{"cost":0.006}}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)
	info.PriceData.SupplierTariff = &types.SupplierModelTariff{}

	_, apiErr := OpenaiHandler(c, info, resp)

	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Empty(t, recorder.Body.String(), "the controller owns the safe public error envelope")
}

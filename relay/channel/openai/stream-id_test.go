package openai

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatStreamKeepsIDAndBillingUsage(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	for _, includeUsage := range []bool{true, false} {
		t.Run(map[bool]string{true: "with usage", false: "without usage"}[includeUsage], func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"id":"chatcmpl-prelude-1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":""}}]}`,
				`data: {"id":"resp_1","object":"chat.completion.chunk","extension":{"keep":true},"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"q\":"}}]}}]}`,
				`data: {"id":"resp_1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"x\"}"}}]},"finish_reason":"tool_calls"}]}`,
				`data: {"id":"resp_1","object":"chat.completion.chunk","model":"gpt-test","choices":[],"usage":{"prompt_tokens":18491,"completion_tokens":801,"total_tokens":19292,"prompt_tokens_details":{"cached_tokens":100},"completion_tokens_details":{"reasoning_tokens":130}},"extension":{"billing":"preserved"}}`,
				`data: [DONE]`,
				``,
			}, "\n\n")
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			info.RelayMode = relayconstant.RelayModeChatCompletions
			info.ShouldIncludeUsage = includeUsage

			usage, apiErr := OaiStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.Equal(t, 18491, usage.PromptTokens)
			require.Equal(t, 801, usage.CompletionTokens)
			require.Equal(t, 130, usage.CompletionTokenDetails.ReasoningTokens)
			require.Equal(t, 100, usage.PromptTokensDetails.CachedTokens)
			require.Equal(t, "gpt-test", info.ProviderReturnedModelName)

			var chunks []string
			for _, line := range strings.Split(recorder.Body.String(), "\n") {
				if strings.HasPrefix(line, "data: {") {
					chunk := strings.TrimPrefix(line, "data: ")
					require.True(t, gjson.Valid(chunk))
					require.Equal(t, "chatcmpl-prelude-1", gjson.Get(chunk, "id").String())
					chunks = append(chunks, chunk)
				}
			}
			if includeUsage {
				require.Len(t, chunks, 4)
				require.Equal(t, int64(801), gjson.Get(chunks[3], "usage.completion_tokens").Int())
				require.Equal(t, "preserved", gjson.Get(chunks[3], "extension.billing").String())
			} else {
				require.Len(t, chunks, 3)
			}
			require.Equal(t, "call_1", gjson.Get(chunks[1], "choices.0.delta.tool_calls.0.id").String())
			require.True(t, gjson.Get(chunks[1], "extension.keep").Bool())
			require.Equal(t, "{\"q\":", gjson.Get(chunks[1], "choices.0.delta.tool_calls.0.function.arguments").String())
			require.Equal(t, "\"x\"}", gjson.Get(chunks[2], "choices.0.delta.tool_calls.0.function.arguments").String())
			require.Equal(t, 1, strings.Count(recorder.Body.String(), "data: [DONE]"))
		})
	}
}

func TestChatStreamSyntheticUsageKeepsFirstID(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	body := "data: {\"id\":\"first\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\ndata: {\"id\":\"last\",\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayMode = relayconstant.RelayModeChatCompletions
	info.SetEstimatePromptTokens(50)

	usage, apiErr := OaiStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.Equal(t, 50, usage.PromptTokens)
	for _, line := range strings.Split(recorder.Body.String(), "\n") {
		if strings.HasPrefix(line, "data: {") {
			require.Equal(t, "first", gjson.Get(strings.TrimPrefix(line, "data: "), "id").String())
		}
	}
	require.Contains(t, recorder.Body.String(), `"usage":`)
}

func TestNormalizeChatStreamIDPreservesStableChunk(t *testing.T) {
	data := `{ "id": "stable", "extension": {"number":1234567890123456789} }`
	var id string
	normalized, err := normalizeChatStreamID(data, &id, "fallback")
	require.NoError(t, err)
	require.Equal(t, data, normalized)
	normalized, err = normalizeChatStreamID(`{"id":"changed","extension":{"number":1234567890123456789}}`, &id, "fallback")
	require.NoError(t, err)
	require.Equal(t, "stable", gjson.Get(normalized, "id").String())
	require.Equal(t, "1234567890123456789", gjson.Get(normalized, "extension.number").Raw)

	normalized, err = normalizeChatStreamID(`{"choices":[]}`, &id, "fallback")
	require.NoError(t, err)
	require.Equal(t, "stable", gjson.Get(normalized, "id").String())

	id = ""
	normalized, err = normalizeChatStreamID(`{"choices":[]}`, &id, "fallback")
	require.NoError(t, err)
	require.Equal(t, "fallback", gjson.Get(normalized, "id").String())
}

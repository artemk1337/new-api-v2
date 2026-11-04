package relayconvert

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/require"
)

func TestCacheWriteLifetimesSurviveUsageConversion(t *testing.T) {
	var src dto.Usage
	require.NoError(t, common.Unmarshal([]byte(`{"input_tokens":12000,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":3000,"ttl_12h_tokens":4000}}}`), &src))
	chat := UsageFromResponsesUsage(&src)
	require.Equal(t, 7000, chat.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 4000, chat.PromptTokensDetails.CacheWriteTokensDetails.TTL12hTokens)
	response := UsageFromChatUsage(chat)
	require.Equal(t, *src.InputTokensDetails, *response.InputTokensDetails)
}

func TestCacheOptionsSurviveProtocolConversion(t *testing.T) {
	var request dto.OpenAIResponsesRequest
	require.NoError(t, common.Unmarshal([]byte(`{"model":"gpt-test","input":"hello","prompt_cache_options":{"ttl":"12h","disabled":false}}`), &request))
	chat, err := ResponsesRequestToChatCompletionsRequest(&request)
	require.NoError(t, err)
	require.JSONEq(t, string(request.PromptCacheOptions), string(chat.PromptCacheOptions))
	response, err := ChatCompletionsRequestToResponsesRequest(chat)
	require.NoError(t, err)
	require.JSONEq(t, string(request.PromptCacheOptions), string(response.PromptCacheOptions))
}

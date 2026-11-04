package openai

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

const cacheLifetimeResponse = `{"id":"resp_cache","status":"completed","usage":{"input_tokens":12000,"output_tokens":1,"total_tokens":12001,"input_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":3000,"ttl_12h_tokens":4000}}}}`

func TestResponsesCacheWriteLifetimesPreserved(t *testing.T) {
	for _, compact := range []bool{false, true} {
		c, _, resp, info := newResponsesChatTestContext(t, cacheLifetimeResponse, false)
		usage, err := OaiResponsesHandler(c, info, resp)
		if compact {
			c, _, resp, _ = newResponsesChatTestContext(t, cacheLifetimeResponse, false)
			usage, err = OaiResponsesCompactionHandler(c, resp)
		}
		require.Nil(t, err)
		require.Equal(t, 7000, usage.PromptTokensDetails.CacheWriteTokens)
		require.NotNil(t, usage.PromptTokensDetails.CacheWriteTokensDetails)
		require.Equal(t, 4000, usage.PromptTokensDetails.CacheWriteTokensDetails.TTL12hTokens)
	}
}

func TestResponsesStreamCacheWriteLifetimesPreserved(t *testing.T) {
	old := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = old })
	body := "data: {\"type\":\"response.completed\",\"response\":" + cacheLifetimeResponse + "}\n\ndata: [DONE]\n\n"
	c, _, resp, info := newResponsesChatTestContext(t, body, true)
	usage, err := OaiResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.Equal(t, 7000, usage.PromptTokensDetails.CacheWriteTokens)
	require.Equal(t, 3000, usage.PromptTokensDetails.CacheWriteTokensDetails.TTL30mTokens)
	require.Equal(t, 4000, usage.PromptTokensDetails.CacheWriteTokensDetails.TTL12hTokens)
}

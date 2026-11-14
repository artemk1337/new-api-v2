package openai

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/stretchr/testify/require"
)

func TestChatStreamRetainsLateUsageAfterClientDisconnect(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, recorder, resp, info := newResponsesChatTestContext(t, "", true)
	info.RelayMode = relayconstant.RelayModeChatCompletions
	clientContext, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(clientContext)
	reader, writer := io.Pipe()
	resp.Body = reader
	t.Cleanup(func() { _ = writer.Close() })

	finished := make(chan *dto.Usage, 1)
	go func() {
		usage, _ := OaiStreamHandler(c, info, resp)
		finished <- usage
	}()

	_, err := io.WriteString(writer, "data: {\"id\":\"prelude\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
	require.NoError(t, err)
	cancel()
	_, err = io.WriteString(writer, "data: {\"id\":\"upstream\",\"model\":\"gpt-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":18491,\"completion_tokens\":801,\"total_tokens\":19292,\"prompt_tokens_details\":{\"cached_tokens\":100},\"completion_tokens_details\":{\"reasoning_tokens\":130}}}\n\ndata: [DONE]\n\n")
	require.NoError(t, err)
	_ = writer.Close()

	select {
	case usage := <-finished:
		require.Equal(t, 18491, usage.PromptTokens)
		require.Equal(t, 801, usage.CompletionTokens)
		require.Equal(t, 19292, usage.TotalTokens)
		require.Equal(t, 130, usage.CompletionTokenDetails.ReasoningTokens)
		require.Equal(t, 100, usage.PromptTokensDetails.CachedTokens)
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish after receiving final usage")
	}
	require.NotContains(t, recorder.Body.String(), `"usage"`)
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
}

func TestResponsesStreamRetainsLateUsageAfterClientDisconnect(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	c, recorder, resp, info := newResponsesChatTestContext(t, "", true)
	clientContext, cancel := context.WithCancel(c.Request.Context())
	t.Cleanup(cancel)
	c.Request = c.Request.WithContext(clientContext)
	reader, writer := io.Pipe()
	resp.Body = reader
	t.Cleanup(func() { _ = writer.Close() })

	finished := make(chan *dto.Usage, 1)
	go func() {
		usage, _ := OaiResponsesStreamHandler(c, info, resp)
		finished <- usage
	}()

	_, err := io.WriteString(writer, "data: {\"type\":\"response.created\"}\n\n")
	require.NoError(t, err)
	cancel()
	_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":327416,\"output_tokens\":95,\"total_tokens\":327511,\"input_tokens_details\":{\"cached_tokens\":259984}}}}\n\ndata: [DONE]\n\n")
	_ = writer.Close()

	select {
	case usage := <-finished:
		require.NotNil(t, usage)
		require.Equal(t, 327416, usage.PromptTokens)
		require.Equal(t, 95, usage.CompletionTokens)
		require.Equal(t, 259984, usage.PromptTokensDetails.CachedTokens)
		require.Equal(t, 327511, usage.TotalTokens)
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not finish after receiving final usage")
	}
	require.NotContains(t, recorder.Body.String(), "response.completed")
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
}

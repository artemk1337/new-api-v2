package channel

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type streamRequestAdaptor struct {
	Adaptor
	url string
}

func (a streamRequestAdaptor) GetRequestURL(*relaycommon.RelayInfo) (string, error) {
	return a.url, nil
}

func (streamRequestAdaptor) SetupRequestHeader(_ *gin.Context, h *http.Header, _ *relaycommon.RelayInfo) error {
	h.Set("Content-Type", "application/json")
	return nil
}

func setupUpstreamStreamTest(t *testing.T) (*gin.Context, *relaycommon.RelayInfo, context.CancelFunc, *httptest.ResponseRecorder) {
	t.Helper()
	service.InitHttpClient()
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	clientContext, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	c.Request = httptest.NewRequestWithContext(clientContext, http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	info := &relaycommon.RelayInfo{
		IsStream:    true,
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	return c, info, cancel, recorder
}

func TestDoApiRequestStreamRetainsLateUsageAfterDisconnect(t *testing.T) {
	c, info, cancelClient, recorder := setupUpstreamStreamTest(t)
	releaseUsage := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-releaseUsage:
		case <-r.Context().Done():
			return
		}
		_, _ = io.WriteString(w, "data: {\"usage\":{\"prompt_tokens\":18491,\"completion_tokens\":801,\"total_tokens\":19292,\"prompt_tokens_details\":{\"cached_tokens\":42},\"completion_tokens_details\":{\"reasoning_tokens\":130}}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	resp, err := DoApiRequest(streamRequestAdaptor{url: server.URL}, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	firstChunk := make(chan struct{})
	finished := make(chan struct{})
	var usage *dto.Usage
	go func() {
		defer close(finished)
		helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
			var chunk struct {
				Usage *dto.Usage `json:"usage"`
			}
			if err := common.UnmarshalJsonStr(data, &chunk); err != nil {
				sr.Stop(err)
				return
			}
			if chunk.Usage != nil {
				usage = chunk.Usage
			} else {
				close(firstChunk)
			}
			if err := helper.StringData(c, data); err != nil {
				sr.Stop(err)
			}
		})
	}()
	select {
	case <-firstChunk:
	case <-time.After(time.Second):
		t.Fatal("initial content did not arrive")
	}
	cancelClient()
	close(releaseUsage)
	select {
	case <-finished:
	case <-time.After(4 * time.Second):
		t.Fatal("usage drain did not finish")
	}
	require.NotNil(t, usage)
	require.Equal(t, 18491, usage.PromptTokens)
	require.Equal(t, 801, usage.CompletionTokens)
	require.Equal(t, 130, usage.CompletionTokenDetails.ReasoningTokens)
	require.Equal(t, 42, usage.PromptTokensDetails.CachedTokens)
	require.NotContains(t, recorder.Body.String(), "usage")
	require.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
}

func TestDoApiRequestStreamDisconnectCancelsTransportAfterDrain(t *testing.T) {
	c, info, cancelClient, _ := setupUpstreamStreamTest(t)
	upstreamCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	resp, err := DoApiRequest(streamRequestAdaptor{url: server.URL}, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	finished := make(chan struct{})
	go func() {
		helper.StreamScannerHandler(c, resp, info, func(string, *helper.StreamResult) {})
		close(finished)
	}()
	start := time.Now()
	cancelClient()
	select {
	case <-upstreamCanceled:
	case <-time.After(4 * time.Second):
		t.Fatal("upstream transport was not canceled")
	}
	require.GreaterOrEqual(t, time.Since(start), 2*time.Second)
	require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("scanner did not finish after transport cancellation")
	}
}

func TestDoApiRequestDisconnectBeforeHeadersPreservesResponse(t *testing.T) {
	c, info, cancelClient, _ := setupUpstreamStreamTest(t)
	started := make(chan struct{})
	releaseHeaders := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-releaseHeaders:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"usage\":{\"prompt_tokens\":50,\"completion_tokens\":8}}\n\ndata: [DONE]\n\n")
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	type result struct {
		resp *http.Response
		err  error
	}
	finished := make(chan result, 1)
	go func() {
		resp, err := DoApiRequest(streamRequestAdaptor{url: server.URL}, c, info, strings.NewReader(`{}`))
		finished <- result{resp: resp, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream request did not start")
	}
	cancelClient()
	select {
	case <-finished:
		t.Fatal("client disconnect canceled an upstream request before headers")
	case <-time.After(2100 * time.Millisecond):
	}
	close(releaseHeaders)
	var response result
	select {
	case response = <-finished:
	case <-time.After(time.Second):
		t.Fatal("upstream response did not arrive")
	}
	require.NoError(t, response.err)
	t.Cleanup(func() { _ = response.resp.Body.Close() })
	body, err := io.ReadAll(response.resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"completion_tokens":8`)
	require.True(t, info.AttemptDispatched)
	require.NoError(t, response.resp.Request.Context().Err())
	require.NoError(t, response.resp.Body.Close())
	require.ErrorIs(t, response.resp.Request.Context().Err(), context.Canceled)
}

func TestDoApiRequestStreamErrorPreservesBillingEvidence(t *testing.T) {
	c, info, cancelClient, _ := setupUpstreamStreamTest(t)
	cancelClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"message":"failed"},"usage":{"prompt_tokens":50,"completion_tokens":8}}`)
	}))
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	resp, err := DoApiRequest(streamRequestAdaptor{url: server.URL}, c, info, strings.NewReader(`{}`))
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusBadGateway, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(body), `"completion_tokens":8`)
	require.NoError(t, resp.Request.Context().Err())
	require.NoError(t, resp.Body.Close())
	require.ErrorIs(t, resp.Request.Context().Err(), context.Canceled)
}

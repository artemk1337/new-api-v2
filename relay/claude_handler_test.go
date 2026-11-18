package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClaudeHelperSendsOpus55EffortToUpstream(t *testing.T) {
	tests := []struct {
		name        string
		channelType int
		input       string
		wantPath    string
		wantFields  map[string]string
	}{
		{
			name:        "Anthropic preserves structured output",
			channelType: constant.ChannelTypeAnthropic,
			input:       `"reasoning_effort":"xhigh","output_config":{"format":{"type":"json_schema"}},"thinking":{"type":"enabled","budget_tokens":1280}`,
			wantPath:    "/v1/messages",
			wantFields: map[string]string{
				"output_config.effort":      "xhigh",
				"output_config.format.type": "json_schema",
				"thinking.type":             "adaptive",
			},
		},
		{
			name:        "OpenAI prefers native effort",
			channelType: constant.ChannelTypeOpenAI,
			input:       `"reasoning_effort":"low","output_config":{"effort":"max"}`,
			wantPath:    "/v1/chat/completions",
			wantFields:  map[string]string{"reasoning_effort": "max"},
		},
	}

	service.InitHttpClient()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
				}
				if r.URL.Path != tt.wantPath {
					t.Errorf("upstream path = %q, want %q", r.URL.Path, tt.wantPath)
				}
				for field, want := range tt.wantFields {
					if got := gjson.GetBytes(body, field).String(); got != want {
						t.Errorf("%s = %q, want %q", field, got, want)
					}
				}
				for _, field := range []string{"thinking.budget_tokens", "temperature"} {
					if gjson.GetBytes(body, field).Exists() {
						t.Errorf("unexpected upstream field %s", field)
					}
				}
				if tt.channelType == constant.ChannelTypeAnthropic && gjson.GetBytes(body, "reasoning_effort").Exists() {
					t.Error("unexpected reasoning_effort in Anthropic request")
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `{"error":{"message":"test upstream response"}}`)
			}))
			defer server.Close()

			const model = "claude-opus-5-5"
			body := `{"model":"` + model + `","temperature":0.7,"messages":[{"role":"user","content":"hello"}],` + tt.input + `}`
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			common.SetContextKey(ctx, constant.ContextKeyChannelType, tt.channelType)
			common.SetContextKey(ctx, constant.ContextKeyChannelBaseUrl, server.URL)
			common.SetContextKey(ctx, constant.ContextKeyOriginalModel, model)
			request, err := helper.GetAndValidateClaudeRequest(ctx)
			require.NoError(t, err)
			info := &relaycommon.RelayInfo{Request: request, OriginModelName: model, RelayFormat: types.RelayFormatClaude}
			apiErr := ClaudeHelper(ctx, info)
			require.NotNil(t, apiErr)
			require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
			require.Contains(t, apiErr.Error(), "test upstream response")
		})
	}
}

func TestNormalizeClaudeHelperUsageSemanticMarksOpenAITotals(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:     211120,
		CompletionTokens: 90,
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 210048,
		},
		ClaudeCacheCreation5mTokens: 700,
		ClaudeCacheCreation1hTokens: 370,
	}
	info := &relaycommon.RelayInfo{
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAI},
		FinalRequestRelayFormat: types.RelayFormatOpenAI,
	}

	result, ok := normalizeClaudeHelperUsageSemantic(info, usage).(*dto.Usage)
	require.True(t, ok)
	require.Same(t, usage, result)
	require.Equal(t, "openai", result.UsageSemantic)
	require.Equal(t, "openai-compatible", result.UsageSource)
	require.Equal(t, 1070, result.PromptTokensDetails.CachedCreationTokens)
	require.Equal(t, 700, result.ClaudeCacheCreation5mTokens)
	require.Equal(t, 370, result.ClaudeCacheCreation1hTokens)

	params := service.BuildTieredTokenParams(result, false, map[string]bool{"cr": true, "cc": true, "cc1h": true})
	require.Equal(t, float64(2), params.P)
}

func TestNormalizeClaudeHelperUsageSemanticPreservesNativeClaude(t *testing.T) {
	usage := &dto.Usage{UsageSemantic: "anthropic"}
	info := &relaycommon.RelayInfo{FinalRequestRelayFormat: types.RelayFormatClaude}

	result, ok := normalizeClaudeHelperUsageSemantic(info, usage).(*dto.Usage)
	require.True(t, ok)
	require.Same(t, usage, result)
	require.Equal(t, "anthropic", result.UsageSemantic)
	require.Empty(t, result.UsageSource)
}

func TestNormalizeClaudeHelperUsageSemanticForClaudeResponsesConversion(t *testing.T) {
	usage := &dto.Usage{
		PromptTokens:                100,
		ClaudeCacheCreation5mTokens: 10,
		ClaudeCacheCreation1hTokens: 10,
	}
	info := &relaycommon.RelayInfo{
		RequestConversionChain:  []types.RelayFormat{types.RelayFormatClaude, types.RelayFormatOpenAIResponses},
		FinalRequestRelayFormat: types.RelayFormatOpenAIResponses,
	}

	result := normalizeClaudeHelperUsageSemantic(info, usage).(*dto.Usage)
	params := service.BuildTieredTokenParams(result, false, map[string]bool{"cc": true, "cc1h": true})

	require.Equal(t, float64(80), params.P)
	require.Equal(t, float64(10), params.CC)
	require.Equal(t, float64(10), params.CC1h)
}

func TestNormalizeClaudeHelperUsageSemanticNoOpForNonConvertedRequests(t *testing.T) {
	tests := []struct {
		name  string
		chain []types.RelayFormat
		final types.RelayFormat
	}{
		{
			name:  "native Claude",
			chain: []types.RelayFormat{types.RelayFormatClaude},
			final: types.RelayFormatClaude,
		},
		{
			name:  "plain OpenAI",
			chain: []types.RelayFormat{types.RelayFormatOpenAI},
			final: types.RelayFormatOpenAI,
		},
		{
			name:  "OpenAI responses",
			chain: []types.RelayFormat{types.RelayFormatOpenAIResponses},
			final: types.RelayFormatOpenAIResponses,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{}
			info := &relaycommon.RelayInfo{
				RequestConversionChain:  tt.chain,
				FinalRequestRelayFormat: tt.final,
			}

			result, ok := normalizeClaudeHelperUsageSemantic(info, usage).(*dto.Usage)
			require.True(t, ok)
			require.Same(t, usage, result)
			require.Empty(t, result.UsageSemantic)
			require.Empty(t, result.UsageSource)
		})
	}
}

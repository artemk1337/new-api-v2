package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

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

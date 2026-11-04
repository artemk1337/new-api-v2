package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

const lifetimeCacheExpr = `tier("standard", p + cr * 0.1 + cc * 1.25 + cc12h * 2.5)`

func TestCacheWriteLifetimesSettleWithoutDoubleCounting(t *testing.T) {
	var usage dto.Usage
	require.NoError(t, common.Unmarshal([]byte(`{"prompt_tokens":12000,"prompt_tokens_details":{"cached_tokens":4000,"cache_write_tokens":7000,"cache_write_tokens_details":{"ttl_30m_tokens":3000,"ttl_12h_tokens":4000}}}`), &usage))
	params := BuildTieredTokenParams(&usage, false, billingexpr.UsedVars(lifetimeCacheExpr))
	require.Equal(t, float64(1000), params.P)
	require.Equal(t, float64(12000), params.Len)
	require.Equal(t, float64(3000), params.CC)
	require.Zero(t, params.CC1h)
	require.Equal(t, float64(4000), params.CC12h)
	require.False(t, params.CacheCreationTokensFallback)
	ok, quota, _ := TryTieredSettle(makeRelayInfo(lifetimeCacheExpr, 1, 0, 0), params)
	require.True(t, ok)
	// The documented input example costs $0.01515, not an additional write aggregate.
	require.Equal(t, 7575, quota)
}

func TestCacheWriteUnpricedLifetimeStaysInPrompt(t *testing.T) {
	usage := &dto.Usage{PromptTokens: 12000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4000, CacheWriteTokens: 7000, CacheWriteTokensDetails: &dto.CacheWriteTokenDetails{TTL30mTokens: 3000, TTL12hTokens: 4000}}}
	params := BuildTieredTokenParams(usage, false, map[string]bool{"cc": true, "cr": true})
	require.Equal(t, float64(5000), params.P)
	require.False(t, params.CacheCreationTokensFallback)
}

func TestCacheWriteMissingLifetimeUsesHigherApplicableRate(t *testing.T) {
	for _, tt := range []struct {
		name    string
		details *dto.CacheWriteTokenDetails
		want    int
	}{
		{"missing split", nil, 9450},
		{"explicit zero twelve hour", &dto.CacheWriteTokenDetails{TTL30mTokens: 7000, TTL12hTokens: 0}, 5075},
		{"residual unknown writes", &dto.CacheWriteTokenDetails{TTL30mTokens: 1000, TTL12hTokens: 2000}, 8825},
	} {
		t.Run(tt.name, func(t *testing.T) {
			usage := &dto.Usage{PromptTokens: 12000, PromptTokensDetails: dto.InputTokenDetails{CachedTokens: 4000, CacheWriteTokens: 7000, CacheWriteTokensDetails: tt.details}}
			params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(lifetimeCacheExpr))
			_, quota, _ := TryTieredSettle(makeRelayInfo(lifetimeCacheExpr, 1, 0, 0), params)
			require.Equal(t, tt.want, quota)
		})
	}
}

func TestCacheWriteFallbackComposesWithUnknownReasoning(t *testing.T) {
	expr := `tier("standard", p + cc * 5 + cc12h * 2 + c * 8 + rt * 3)`
	usage := &dto.Usage{PromptTokens: 1000, CompletionTokens: 100, PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 500}}
	params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expr))
	_, quota, _ := TryTieredSettle(makeRelayInfo(expr, 1, 0, 0), params)
	// 500 ordinary + 500 writes at 5 + 100 unknown output at 8.
	require.Equal(t, 1900, quota)
}

func TestCacheWriteFallbackWithoutGenericLanePreservesOrdinaryInput(t *testing.T) {
	expr := `tier("standard", p * 5 + cc12h * 2)`
	usage := &dto.Usage{PromptTokens: 1000, PromptTokensDetails: dto.InputTokenDetails{CacheWriteTokens: 500}}
	params := BuildTieredTokenParams(usage, false, billingexpr.UsedVars(expr))
	_, quota, _ := TryTieredSettle(makeRelayInfo(expr, 1, 0, 0), params)
	require.Equal(t, 2500, quota)
}

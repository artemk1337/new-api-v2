package billingexpr_test

import (
	"testing"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/stretchr/testify/require"
)

func TestCacheWriteLifetimeExpressions(t *testing.T) {
	expr := `len <= 272000 ? tier("short", p * 2 + cc * 2.5 + cc12h * 5) : tier("long", p * 4 + cc * 5 + cc12h * 10)`
	require.True(t, billingexpr.UsedVars(expr)["cc12h"])
	for _, tt := range []struct {
		length float64
		want   float64
		tier   string
	}{{272000, 800, "short"}, {272001, 1600, "long"}} {
		cost, trace, err := billingexpr.RunExpr(expr, billingexpr.TokenParams{P: 100, CC: 40, CC12h: 100, Len: tt.length})
		require.NoError(t, err)
		require.Equal(t, tt.want, cost)
		require.Equal(t, tt.tier, trace.MatchedTier)
	}
}

func TestCacheWriteLifetimeFallbackRequiresLinearAllocations(t *testing.T) {
	for _, expr := range []string{
		`tier("normal", p * 2 + cc * 2.5 + cc12h * 5)`,
		`len <= 272000 ? tier("short", p + cc12h * 2) : tier("long", p * 2 + cc12h * 4)`,
		`param("tier") == "fast" ? tier("fast", cc * 4 + cc12h * 8) : tier("normal", cc * 2 + cc12h * 4)`,
	} {
		require.True(t, billingexpr.CacheCreationSplitIsSafe(expr), expr)
	}
	for _, expr := range []string{
		`tier("unsafe", cc12h * (500 - cc12h))`,
		`cc12h < 200 ? tier("unsafe", cc12h * 10) : tier("unsafe", cc12h)`,
	} {
		require.False(t, billingexpr.CacheCreationSplitIsSafe(expr), expr)
	}
}

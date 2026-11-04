package billingexpr

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSupplierExpressionRejectsRequestAndClockDependencies(t *testing.T) {
	for _, expression := range []string{
		`param("service_tier") == "fast" ? p * 10 : p`,
		`header("anthropic-beta") == "fast" ? p * 10 : p`,
		`hour("UTC") < 12 ? p : p * 2`,
		`minute("UTC") + p`, `weekday("UTC") + p`, `month("UTC") + p`, `day("UTC") + p`,
		`false ? hour("UTC") * p : p`,
	} {
		t.Run(expression, func(t *testing.T) { require.Error(t, ValidateSupplierExpression(expression)) })
	}
}

func TestSupplierExpressionAllowsPureContextTiers(t *testing.T) {
	expression := `v1:len > 1000 ? tier("long", p * 2 + c * 10 + cr * 0.2) : tier("base", max(p, 0) + c * 5 + cr * 0.1)`
	require.NoError(t, ValidateSupplierExpression(expression))
	cost, trace, err := RunExpr(expression, TokenParams{P: 200, C: 100, CR: 1000, Len: 1200})
	require.NoError(t, err)
	require.Equal(t, 1600.0, cost)
	require.Equal(t, "long", trace.MatchedTier)
}

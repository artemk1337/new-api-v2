package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmokeTestExprRejectsNegativeOptionalTokenDimensions(t *testing.T) {
	for _, variable := range []string{"cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "rt"} {
		t.Run(variable, func(t *testing.T) {
			err := SmokeTestExpr(`tier("base", ` + variable + ` > 0 ? -` + variable + ` : p)`)
			require.Error(t, err)
		})
	}
}

func TestSmokeTestExprRejectsDirectCompletionTierWithReasoning(t *testing.T) {
	require.Error(t, SmokeTestExpr(`c <= 500 ? tier("short", c + rt) : tier("long", c + rt)`))
	require.Error(t, SmokeTestExpr(`200 < c ? tier("short", c + rt) : tier("long", c + rt)`))
	require.Error(t, SmokeTestExpr(`c + ao > 200 && c + ao < 400 ? tier("middle", (c + rt) * 100) : tier("edge", c + rt)`))
	require.Error(t, SmokeTestExpr(`let output = c; output > 200 && output < 400 ? tier("middle", (c + rt) * 100) : tier("edge", c + rt)`))
	require.Error(t, SmokeTestExpr(`tier("base", min(c, rt) * 100)`))
	require.NoError(t, SmokeTestExpr(`tier("base", max(p, cr) + c * 2 + rt * 8)`))
	require.NoError(t, SmokeTestExpr(`len <= 500 ? tier("short", c + rt) : tier("long", c + rt)`))
	require.NoError(t, SmokeTestExpr(`len <= 500 ? tier("short", c * 2 + rt * 8) : tier("long", c * 4 + rt * 12)`))
}

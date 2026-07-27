package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSmokeTestExprRejectsNegativeOptionalTokenDimensions(t *testing.T) {
	for _, variable := range []string{"cr", "cc", "cc1h", "img", "img_o", "ai", "ao"} {
		t.Run(variable, func(t *testing.T) {
			err := SmokeTestExpr(`tier("base", ` + variable + ` > 0 ? -` + variable + ` : p)`)
			require.Error(t, err)
		})
	}
}

package billing_setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTwelveHourCacheValidationRejectsUnsafeUnknownAllocation(t *testing.T) {
	require.NoError(t, SmokeTestExpr(`tier("standard", p * 2 + cc * 2.5 + cc12h * 5)`))
	require.ErrorContains(t, SmokeTestExpr(`tier("unsafe", cc12h * (500 - cc12h))`), "12-hour cache pricing requires linear")
}

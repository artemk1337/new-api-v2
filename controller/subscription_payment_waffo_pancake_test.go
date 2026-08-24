package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaffoPancakeSubscriptionProviderAmountRejectsFractionalCents(t *testing.T) {
	for _, amount := range []float64{10.004, 10.005} {
		_, err := waffoPancakeSubscriptionProviderAmount(amount)
		require.Error(t, err)
	}
}

func TestWaffoPancakeSubscriptionProviderAmountKeepsExactCents(t *testing.T) {
	amount, err := waffoPancakeSubscriptionProviderAmount(10.01)
	require.NoError(t, err)
	require.Equal(t, 10.01, amount)
}

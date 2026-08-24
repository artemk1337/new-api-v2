package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishYooKassaConfigPreservesExplicitEmptyPaymentMethods(t *testing.T) {
	previous := GetYooKassaConfig()
	t.Cleanup(func() { PublishYooKassaConfig(previous) })

	PublishYooKassaConfig(YooKassaConfig{Enabled: true, ShopID: "shop", SecretKey: "secret"})
	config := GetYooKassaConfig()
	require.Empty(t, config.PaymentMethods)
	require.Empty(t, YooKassaPaymentMethods)
}

package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestValidateCreemWebhookModeBindsConfiguredEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name      string
		testMode  bool
		mode      string
		wantError bool
	}{
		{name: "legacy payload without mode", testMode: true},
		{name: "test matches", testMode: true, mode: "test"},
		{name: "test rejects live", testMode: true, mode: "live", wantError: true},
		{name: "live matches", mode: "live"},
		{name: "live accepts production alias", mode: "production"},
		{name: "live rejects test", mode: "test", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &CreemWebhookEvent{}
			event.Object.Mode = tc.mode
			err := validateCreemWebhookMode(event, setting.CreemConfig{TestMode: tc.testMode})
			if tc.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestValidateCreemWebhookModeRejectsConflictingNestedModes(t *testing.T) {
	event := &CreemWebhookEvent{}
	event.Object.Mode = "test"
	event.Object.Order.Mode = "live"
	require.Error(t, validateCreemWebhookMode(event, setting.CreemConfig{TestMode: true}))
}

func TestCreemTopUpInfoForConfigKeepsReadinessAndProductsTogether(t *testing.T) {
	confirmPaymentComplianceForTest(t)

	ready, products := creemTopUpInfoForConfig(setting.CreemConfig{
		APIKey:        "api-key",
		Products:      `[{"productId":"old"}]`,
		WebhookSecret: "webhook-secret",
	})
	require.True(t, ready)
	require.Equal(t, `[{"productId":"old"}]`, products)

	ready, products = creemTopUpInfoForConfig(setting.CreemConfig{
		APIKey:        "api-key",
		Products:      `[{"productId":"new"}]`,
		WebhookSecret: "",
	})
	require.False(t, ready)
	require.Equal(t, `[{"productId":"new"}]`, products)
}

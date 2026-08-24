package setting

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishCreemConfigReplacesWholeSnapshot(t *testing.T) {
	original := GetCreemConfig()
	t.Cleanup(func() { PublishCreemConfig(original) })

	oldConfig := CreemConfig{
		APIKey:        "old-api",
		Products:      "old-products",
		TestMode:      false,
		WebhookSecret: "old-webhook",
	}
	PublishCreemConfig(oldConfig)
	require.Equal(t, oldConfig, GetCreemConfig())

	newConfig := CreemConfig{
		APIKey:        "new-api",
		Products:      "new-products",
		TestMode:      true,
		WebhookSecret: "new-webhook",
	}
	PublishCreemConfig(newConfig)
	require.Equal(t, newConfig, GetCreemConfig())
}

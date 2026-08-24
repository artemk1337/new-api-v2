package setting

import "sync/atomic"

var CreemApiKey = ""
var CreemProducts = "[]"
var CreemTestMode = false
var CreemWebhookSecret = ""

type CreemConfig struct {
	APIKey        string
	Products      string
	TestMode      bool
	WebhookSecret string
}

var creemConfig atomic.Pointer[CreemConfig]

func init() {
	PublishCreemConfig(CreemConfig{Products: CreemProducts})
}

func GetCreemConfig() CreemConfig {
	config := creemConfig.Load()
	if config == nil {
		return CreemConfig{Products: "[]"}
	}
	return *config
}

// PublishCreemConfig switches every runtime checkout dependency together.
// Legacy exported fields are kept for option serialization and migration code;
// payment paths must read GetCreemConfig instead.
func PublishCreemConfig(config CreemConfig) {
	if config.Products == "" {
		config.Products = "[]"
	}
	creemConfig.Store(&config)
	CreemApiKey = config.APIKey
	CreemProducts = config.Products
	CreemTestMode = config.TestMode
	CreemWebhookSecret = config.WebhookSecret
}

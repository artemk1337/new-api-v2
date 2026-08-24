package setting

import "sync/atomic"

var YooKassaEnabled = false
var YooKassaShopID = ""
var YooKassaSecretKey = ""
var YooKassaReturnURL = ""
var YooKassaPaymentMethods = "sbp"

type YooKassaConfig struct {
	Enabled        bool
	ShopID         string
	SecretKey      string
	ReturnURL      string
	PaymentMethods string
}

var yooKassaConfig atomic.Pointer[YooKassaConfig]

func init() {
	PublishYooKassaConfig(YooKassaConfig{PaymentMethods: YooKassaPaymentMethods})
}

// GetYooKassaConfig returns one immutable configuration version. Payment
// handlers must keep this value for the whole request, rather than combine
// independently mutable legacy fields.
func GetYooKassaConfig() YooKassaConfig {
	config := yooKassaConfig.Load()
	if config == nil {
		return YooKassaConfig{PaymentMethods: "sbp"}
	}
	return *config
}

// PublishYooKassaConfig switches all checkout settings together. The legacy
// fields remain for option serialization and migrations; runtime payment code
// reads GetYooKassaConfig instead.
func PublishYooKassaConfig(config YooKassaConfig) {
	yooKassaConfig.Store(&config)
	YooKassaEnabled = config.Enabled
	YooKassaShopID = config.ShopID
	YooKassaSecretKey = config.SecretKey
	YooKassaReturnURL = config.ReturnURL
	YooKassaPaymentMethods = config.PaymentMethods
}

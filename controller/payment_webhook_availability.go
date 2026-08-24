package controller

import (
	"strings"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
)

func isPaymentComplianceConfirmed() bool {
	return operation_setting.IsPaymentComplianceConfirmed()
}

func isStripeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isStripeAPISecretConfigured() &&
		strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

// isStripeAPISecretConfigured keeps the readiness check in sync with the
// Stripe client, which accepts only secret or restricted API keys.
func isStripeAPISecretConfigured() bool {
	secret := strings.TrimSpace(setting.StripeApiSecret)
	return strings.HasPrefix(secret, "sk_") || strings.HasPrefix(secret, "rk_")
}

func isStripeWebhookConfigured() bool {
	return strings.TrimSpace(setting.StripeWebhookSecret) != ""
}

func isStripeWebhookEnabled() bool {
	// Existing pending checkouts only need the verification secret. Keep this
	// independent from create-time API/price/compliance settings so disabling
	// new checkouts does not strand already-paid orders.
	return isStripeWebhookConfigured()
}

func isCreemTopUpEnabled() bool {
	return isCreemTopUpEnabledForConfig(setting.GetCreemConfig())
}

func isCreemTopUpEnabledForConfig(config setting.CreemConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	products := strings.TrimSpace(config.Products)
	return strings.TrimSpace(config.APIKey) != "" &&
		products != "" &&
		products != "[]" &&
		isCreemWebhookConfiguredForConfig(config)
}

func isCreemWebhookConfigured() bool {
	return isCreemWebhookConfiguredForConfig(setting.GetCreemConfig())
}

func isCreemWebhookConfiguredForConfig(config setting.CreemConfig) bool {
	// Test mode changes the provider endpoint, not the trust model. A public
	// webhook without a secret would let anyone forge a paid callback.
	return strings.TrimSpace(config.WebhookSecret) != ""
}

func isCreemWebhookEnabled() bool {
	return isCreemWebhookConfigured()
}

func isWaffoTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	if !setting.WaffoEnabled {
		return false
	}

	return isWaffoWebhookConfigured()
}

func isWaffoWebhookConfigured() bool {
	if setting.WaffoSandbox {
		return strings.TrimSpace(setting.WaffoSandboxApiKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPrivateKey) != "" &&
			strings.TrimSpace(setting.WaffoSandboxPublicCert) != ""
	}

	return strings.TrimSpace(setting.WaffoApiKey) != "" &&
		strings.TrimSpace(setting.WaffoPrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPublicCert) != ""
}

func isWaffoWebhookEnabled() bool {
	return isWaffoWebhookConfigured()
}

func isWaffoPancakeTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	// Presence-of-credentials = enabled. Webhook public keys ship inside
	// the SDK; mode (test/prod) is read from each event.
	return strings.TrimSpace(setting.WaffoPancakeMerchantID) != "" &&
		strings.TrimSpace(setting.WaffoPancakePrivateKey) != "" &&
		strings.TrimSpace(setting.WaffoPancakeProductID) != ""
}

func isWaffoPancakeWebhookConfigured() bool {
	// Pancake webhook signatures are verified with the provider's embedded
	// public keys (see service.VerifyConfiguredWaffoPancakeWebhook), so the
	// merchant/private credentials used to create new checkout sessions are not
	// required to process an existing pending payment.
	return true
}

func isWaffoPancakeWebhookEnabled() bool {
	return isWaffoPancakeWebhookConfigured()
}

func isYooKassaTopUpEnabled() bool {
	return isYooKassaTopUpEnabledForConfig(setting.GetYooKassaConfig())
}

func isYooKassaTopUpEnabledForConfig(config setting.YooKassaConfig) bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return config.Enabled &&
		strings.TrimSpace(config.ShopID) != "" &&
		strings.TrimSpace(config.SecretKey) != ""
}

func isYooKassaWebhookConfigured() bool {
	return isYooKassaWebhookConfiguredForConfig(setting.GetYooKassaConfig())
}

func isYooKassaWebhookConfiguredForConfig(config setting.YooKassaConfig) bool {
	return strings.TrimSpace(config.ShopID) != "" &&
		strings.TrimSpace(config.SecretKey) != ""
}

func isYooKassaWebhookEnabled() bool {
	return isYooKassaWebhookConfigured()
}

func isNOWPaymentsTopUpEnabled() bool {
	return isPaymentComplianceConfirmed() && setting.NOWPaymentsEnabled && isNOWPaymentsWebhookConfigured() &&
		strings.TrimSpace(setting.NOWPaymentsAPIKey) != "" &&
		strings.TrimSpace(setting.NOWPaymentsIPNCallbackURL) != ""
}

func isNOWPaymentsWebhookConfigured() bool {
	return strings.TrimSpace(setting.NOWPaymentsAPIKey) != "" && strings.TrimSpace(setting.NOWPaymentsIPNSecret) != ""
}

func isNOWPaymentsWebhookEnabled() bool {
	return isNOWPaymentsWebhookConfigured()
}

func isEpayTopUpEnabled() bool {
	if !isPaymentComplianceConfirmed() {
		return false
	}
	return isEpayWebhookConfigured() && len(operation_setting.PayMethodsSnapshot()) > 0
}

func isEpayWebhookConfigured() bool {
	return strings.TrimSpace(operation_setting.PayAddress) != "" &&
		strings.TrimSpace(operation_setting.EpayId) != "" &&
		strings.TrimSpace(operation_setting.EpayKey) != ""
}

func isEpayWebhookEnabled() bool {
	return isEpayWebhookConfigured()
}

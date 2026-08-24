package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/require"
)

func confirmPaymentComplianceForTest(t *testing.T) {
	t.Helper()
	paymentSetting := operation_setting.GetPaymentSetting()
	originalConfirmed := paymentSetting.ComplianceConfirmed
	originalTermsVersion := paymentSetting.ComplianceTermsVersion
	t.Cleanup(func() {
		paymentSetting.ComplianceConfirmed = originalConfirmed
		paymentSetting.ComplianceTermsVersion = originalTermsVersion
	})
	paymentSetting.ComplianceConfirmed = true
	paymentSetting.ComplianceTermsVersion = operation_setting.CurrentComplianceTermsVersion
}

func TestStripeWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalAPISecret := setting.StripeApiSecret
	originalWebhookSecret := setting.StripeWebhookSecret
	originalPriceID := setting.StripePriceId
	t.Cleanup(func() {
		setting.StripeApiSecret = originalAPISecret
		setting.StripeWebhookSecret = originalWebhookSecret
		setting.StripePriceId = originalPriceID
	})

	setting.StripeWebhookSecret = ""
	setting.StripeApiSecret = "sk_test_123"
	require.False(t, isStripeWebhookEnabled())
	require.False(t, isStripeTopUpEnabled())

	setting.StripeWebhookSecret = "whsec_test"
	require.True(t, isStripeWebhookEnabled())
	require.True(t, isStripeTopUpEnabled())

	setting.StripeApiSecret = "publishable_test_key"
	require.False(t, isStripeTopUpEnabled(), "readiness must reject non-secret Stripe keys")
	setting.StripeApiSecret = "rk_test_123"
	require.True(t, isStripeTopUpEnabled(), "restricted Stripe keys are accepted")

	setting.StripePriceId = ""
	require.True(t, isStripeWebhookEnabled())
	require.True(t, isStripeTopUpEnabled())

	setting.StripeApiSecret = ""
	require.False(t, isStripeTopUpEnabled())
	require.True(t, isStripeWebhookEnabled())
}

func TestCreemWebhookEnabledRequiresTopUpAndWebhookConfig(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalConfig := setting.GetCreemConfig()
	t.Cleanup(func() {
		setting.PublishCreemConfig(originalConfig)
	})

	setting.PublishCreemConfig(setting.CreemConfig{APIKey: "creem_api_key", Products: `[{"productId":"prod_123"}]`})
	require.False(t, isCreemWebhookEnabled())
	require.False(t, isCreemTopUpEnabled())

	config := setting.GetCreemConfig()
	config.WebhookSecret = "creem_secret"
	setting.PublishCreemConfig(config)
	require.True(t, isCreemWebhookEnabled())
	require.True(t, isCreemTopUpEnabled())

	config = setting.GetCreemConfig()
	config.Products = "[]"
	setting.PublishCreemConfig(config)
	require.False(t, isCreemTopUpEnabled())
	require.True(t, isCreemWebhookEnabled())
}

func TestCreemTestModeStillRequiresWebhookSignatureSecret(t *testing.T) {
	config := setting.CreemConfig{TestMode: true}
	require.False(t, isCreemWebhookConfiguredForConfig(config))
	require.False(t, verifyCreemSignatureWithConfig(`{"eventType":"checkout.completed"}`, "forged", config))

	config.WebhookSecret = "test-secret"
	body := `{"eventType":"checkout.completed"}`
	require.True(t, verifyCreemSignatureWithConfig(body, generateCreemSignature(body, config.WebhookSecret), config))
}

func TestWaffoWebhookReadinessIsIndependentFromCreateReadiness(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.WaffoEnabled
	originalSandbox := setting.WaffoSandbox
	originalAPIKey := setting.WaffoApiKey
	originalPrivateKey := setting.WaffoPrivateKey
	originalPublicCert := setting.WaffoPublicCert
	originalSandboxAPIKey := setting.WaffoSandboxApiKey
	originalSandboxPrivateKey := setting.WaffoSandboxPrivateKey
	originalSandboxPublicCert := setting.WaffoSandboxPublicCert
	t.Cleanup(func() {
		setting.WaffoEnabled = originalEnabled
		setting.WaffoSandbox = originalSandbox
		setting.WaffoApiKey = originalAPIKey
		setting.WaffoPrivateKey = originalPrivateKey
		setting.WaffoPublicCert = originalPublicCert
		setting.WaffoSandboxApiKey = originalSandboxAPIKey
		setting.WaffoSandboxPrivateKey = originalSandboxPrivateKey
		setting.WaffoSandboxPublicCert = originalSandboxPublicCert
	})

	setting.WaffoEnabled = true
	setting.WaffoSandbox = false
	setting.WaffoApiKey = ""
	setting.WaffoPrivateKey = "private"
	setting.WaffoPublicCert = "public"
	require.False(t, isWaffoTopUpEnabled())
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoApiKey = "api"
	require.True(t, isWaffoTopUpEnabled())
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = false
	require.False(t, isWaffoTopUpEnabled())
	require.True(t, isWaffoWebhookEnabled())

	setting.WaffoEnabled = true
	setting.WaffoSandbox = true
	setting.WaffoSandboxApiKey = ""
	setting.WaffoSandboxPrivateKey = "sandbox_private"
	setting.WaffoSandboxPublicCert = "sandbox_public"
	require.False(t, isWaffoWebhookEnabled())

	setting.WaffoSandboxApiKey = "sandbox_api"
	require.True(t, isWaffoTopUpEnabled())
	require.True(t, isWaffoWebhookEnabled())
}

func TestWaffoPancakeWebhookReadinessIsIndependentFromCreateReadiness(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalMerchantID := setting.WaffoPancakeMerchantID
	originalPrivateKey := setting.WaffoPancakePrivateKey
	originalProductID := setting.WaffoPancakeProductID
	t.Cleanup(func() {
		setting.WaffoPancakeMerchantID = originalMerchantID
		setting.WaffoPancakePrivateKey = originalPrivateKey
		setting.WaffoPancakeProductID = originalProductID
	})

	// Checkout creation needs merchant/private/product settings. Webhook
	// verification uses public keys bundled in the SDK and remains available
	// for existing pending orders when creation settings are cleared.
	setting.WaffoPancakeMerchantID = ""
	setting.WaffoPancakePrivateKey = "private"
	setting.WaffoPancakeProductID = "product"
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeMerchantID = "merchant"
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.True(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeProductID = ""
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())

	setting.WaffoPancakeProductID = "product"
	setting.WaffoPancakePrivateKey = ""
	require.True(t, isWaffoPancakeWebhookEnabled())
	require.False(t, isWaffoPancakeTopUpEnabled())
}

func TestNOWPaymentsDisableHidesTopUpButKeepsWebhookForPendingPayments(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalEnabled := setting.NOWPaymentsEnabled
	originalAPIKey := setting.NOWPaymentsAPIKey
	originalIPNSecret := setting.NOWPaymentsIPNSecret
	originalCallbackURL := setting.NOWPaymentsIPNCallbackURL
	t.Cleanup(func() {
		setting.NOWPaymentsEnabled = originalEnabled
		setting.NOWPaymentsAPIKey = originalAPIKey
		setting.NOWPaymentsIPNSecret = originalIPNSecret
		setting.NOWPaymentsIPNCallbackURL = originalCallbackURL
	})

	setting.NOWPaymentsEnabled = false
	setting.NOWPaymentsAPIKey = "api_key"
	setting.NOWPaymentsIPNSecret = "ipn_secret"
	setting.NOWPaymentsIPNCallbackURL = "https://example.com/api/user/nowpayments/notify"
	require.False(t, isNOWPaymentsTopUpEnabled())
	require.True(t, isNOWPaymentsWebhookEnabled())

	setting.NOWPaymentsEnabled = true
	require.True(t, isNOWPaymentsTopUpEnabled())

	setting.NOWPaymentsIPNSecret = ""
	require.False(t, isNOWPaymentsWebhookEnabled())
}

func TestEpayWebhookReadinessIsIndependentFromCreateReadiness(t *testing.T) {
	confirmPaymentComplianceForTest(t)
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey
	originalPayMethods := operation_setting.PayMethods
	t.Cleanup(func() {
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
		operation_setting.PayMethods = originalPayMethods
	})

	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = ""
	operation_setting.PayMethods = []map[string]string{{"type": "alipay"}}
	require.False(t, isEpayWebhookEnabled())

	operation_setting.EpayKey = "epay_key"
	require.True(t, isEpayWebhookEnabled())
	require.True(t, isEpayTopUpEnabled())

	operation_setting.PayMethods = nil
	require.True(t, isEpayWebhookEnabled())
	require.False(t, isEpayTopUpEnabled())
}

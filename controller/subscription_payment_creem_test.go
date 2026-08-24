package controller

import (
	"errors"
	"testing"

	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/require"
)

func TestCreemConfiguredProductCurrencyUsesProviderProduct(t *testing.T) {
	original := setting.GetCreemConfig()
	t.Cleanup(func() { setting.PublishCreemConfig(original) })
	config := original
	config.Products = `[{"productId":"prod_cny","currency":"CNY"}]`
	setting.PublishCreemConfig(config)
	require.Equal(t, "CNY", creemConfiguredProductCurrency("prod_cny"))
	require.Equal(t, "UNKNOWN", creemConfiguredProductCurrency("prod_missing"))
}

func TestCreemCreateErrorPolicy(t *testing.T) {
	require.True(t, isPermanentCreemCreateError(errors.New("Creem API http status 400")))
	require.False(t, isPermanentCreemCreateError(errors.New("发送HTTP请求失败: timeout")))
	require.False(t, isPermanentCreemCreateError(errors.New("Creem API http status 503")))
	require.False(t, isPermanentCreemCreateError(errors.New("Creem API resp no checkout url")), "a malformed 2xx response is ambiguous")
	require.False(t, isPermanentCreemCreateError(errors.New("Creem API 解析响应失败")), "a malformed response is ambiguous")
}

func TestValidateCreemProductContractRejectsProviderMismatch(t *testing.T) {
	local := &CreemProduct{ProductId: "prod_basic", Price: 10, Currency: "USD"}

	require.NoError(t, validateCreemProductContract(local, &creemProviderProduct{Id: "prod_basic", Price: 1000, Currency: "usd"}))
	require.Error(t, validateCreemProductContract(local, &creemProviderProduct{Id: "prod_basic", Price: 999, Currency: "USD"}))
	require.Error(t, validateCreemProductContract(local, &creemProviderProduct{Id: "prod_basic", Price: 1000, Currency: "EUR"}))
	require.Error(t, validateCreemProductContract(local, &creemProviderProduct{Id: "prod_other", Price: 1000, Currency: "USD"}))
	require.Error(t, validateCreemProductContract(
		&CreemProduct{ProductId: "prod_fractional", Price: 10.005, Currency: "USD"},
		&creemProviderProduct{Id: "prod_fractional", Price: 1001, Currency: "USD"},
	))
	require.NoError(t, validateCreemProductContract(
		&CreemProduct{ProductId: "prod_jpy", Price: 100, Currency: "JPY"},
		&creemProviderProduct{Id: "prod_jpy", Price: 100, Currency: "JPY"},
	))
	require.NoError(t, validateCreemProductContract(
		&CreemProduct{ProductId: "prod_kwd", Price: 12.345, Currency: "KWD"},
		&creemProviderProduct{Id: "prod_kwd", Price: 12345, Currency: "KWD"},
	))
	require.Error(t, validateCreemProductContract(
		&CreemProduct{ProductId: "prod_zero", Price: 0, Currency: "USD"},
		&creemProviderProduct{Id: "prod_zero", Price: 0, Currency: "USD"},
	))
}

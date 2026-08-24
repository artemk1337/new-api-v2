package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateCreemTopUpProductRequiresIDAndQuota(t *testing.T) {
	for name, product := range map[string]*CreemProduct{
		"missing product": {Price: 10, Currency: "USD", Quota: 100},
		"zero quota":      {ProductId: "prod_basic", Price: 10, Currency: "USD"},
		"negative quota":  {ProductId: "prod_basic", Price: 10, Currency: "USD", Quota: -1},
	} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, validateCreemTopUpProduct(product))
		})
	}

	require.NoError(t, validateCreemTopUpProduct(&CreemProduct{
		ProductId: "prod_basic",
		Price:     10,
		Currency:  "EUR",
		Quota:     5_000_000,
	}))
}

func TestCreemSnapshotKeepsConfiguredQuotaAndProviderCurrency(t *testing.T) {
	product := &CreemProduct{ProductId: "prod_eur", Price: 10, Currency: "EUR", Quota: 5_000_000}
	topUp := &model.TopUp{Amount: product.Quota}
	rate := 1.25 // 1 USD = 1.25 EUR.

	applyCreemPaymentSnapshot(topUp, product, rate)

	assert.Equal(t, product.Quota, int64(topUp.QuotaToAdd))
	assert.Equal(t, product.ProductId, topUp.CreemProductID)
	assert.InDelta(t, product.Price, topUp.RequestedAmount, 0.0000001)
	assert.Equal(t, "EUR", topUp.PaymentCurrency)
	assert.InDelta(t, 8, topUp.PaymentBaseAmount, 0.0000001)
	assert.InDelta(t, 10, topUp.PaymentChargedAmount, 0.0000001)
	assert.InDelta(t, 1.25, topUp.PaymentRateToUSD, 0.0000001)
	require.NoError(t, service.ValidatePaymentSnapshot(topUp, "eur", 10))
}

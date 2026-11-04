package model

import (
	"testing"

	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/require"
)

func TestPublicSupplierPriceUsesOneRetailMultiplierForEveryLane(t *testing.T) {
	input, output, cache, write, hour := 0.012, 0.06, 0.0012, 0.015, 0.024
	tariff := types.SupplierModelTariff{SupplierID: 7, ProfileID: 8, Revision: 9, ModelName: "internal-alias", BillingMode: types.SupplierBillingModeToken, PriceUnit: "million_tokens", InputUSD: &input, OutputUSD: &output, CacheReadUSD: &cache, CacheWriteUSD: &write, CacheWrite1hUSD: &hour, Tools: map[string]float64{"web_search": 0.003}}
	retail := publicGroupPrice(tariff, 2)
	require.Equal(t, input*2, *retail.InputUSD)
	require.Equal(t, output*2, *retail.OutputUSD)
	require.Equal(t, cache*2, *retail.CacheReadUSD)
	require.Equal(t, write*2, *retail.CacheWriteUSD)
	require.Equal(t, hour*2, *retail.CacheWrite1hUSD)
	require.Equal(t, 0.006, retail.Tools["web_search"])
	require.Nil(t, retail.AudioInputUSD)
}

func TestPublicSupplierTierPricesPreserveDifferentCacheRates(t *testing.T) {
	price := publicGroupPrice(types.SupplierModelTariff{BillingMode: types.SupplierBillingModeExpression, PriceUnit: "million_tokens", Expression: `(len <= 272000 ? tier("private-source",p*2+c*10+cr*0.1+cc*2.5+cc1h*4) : tier("private-long",p*4+c*15+cr*0.2+cc*5+cc1h*8))*0.006`}, 2)
	require.Len(t, price.Tiers, 2)
	require.InDelta(t, 0.024, *price.InputUSD, 1e-12)
	require.InDelta(t, 0.12, *price.OutputUSD, 1e-12)
	require.InDelta(t, 0.0012, *price.Tiers[0].CacheReadUSD, 1e-12)
	require.InDelta(t, 0.096, *price.Tiers[1].CacheWrite1hUSD, 1e-12)
	require.EqualValues(t, 272000, *price.Tiers[0].MaxInputTokens)
	require.EqualValues(t, 272001, *price.Tiers[1].MinInputTokens)
}

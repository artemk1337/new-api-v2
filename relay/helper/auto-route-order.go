package helper

import (
	"cmp"
	"math"
	"slices"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
)

// SortAutoRouteCandidates compares each token route's frozen retail input/output
// pair. A lower pair price means greater savings against a shared model baseline.
// Other billing units and opaque expressions keep their quote-cost policy.
func SortAutoRouteCandidates(candidates []relaycommon.AutoRouteCandidate) {
	slices.SortStableFunc(candidates, func(a, b relaycommon.AutoRouteCandidate) int {
		if order := cmp.Compare(a.EstimatedQuota, b.EstimatedQuota); order != 0 {
			return order
		}
		return cmp.Compare(a.Group, b.Group)
	})
	type pairRoute struct {
		candidate relaycommon.AutoRouteCandidate
		price     float64
	}
	var slots []int
	var routes []pairRoute
	for index, candidate := range candidates {
		if pair, ok := frozenRetailPairPrice(candidate); ok {
			slots = append(slots, index)
			routes = append(routes, pairRoute{candidate: candidate, price: pair})
		}
	}
	slices.SortStableFunc(routes, func(a, b pairRoute) int {
		if order := cmp.Compare(a.price, b.price); order != 0 {
			return order
		}
		return cmp.Compare(a.candidate.Group, b.candidate.Group)
	})
	for index, slot := range slots {
		candidates[slot] = routes[index].candidate
	}
}

func frozenRetailPairPrice(candidate relaycommon.AutoRouteCandidate) (float64, bool) {
	price := candidate.PriceData
	if tariff := price.SupplierTariff; tariff != nil && tariff.BillingMode == types.SupplierBillingModeToken && (tariff.InputUSD == nil || tariff.OutputUSD == nil) {
		return 0, false
	}
	if snapshot := candidate.TieredSnapshot; snapshot != nil {
		tiers, ok := billingexpr.PublicPricingTiers(snapshot.ExprString, snapshot.GroupRatio)
		if !ok {
			return 0, false
		}
		for _, tier := range tiers {
			inputTokens := int64(snapshot.EstimatedPromptTokens)
			if tier.MinInputTokens != nil && inputTokens < *tier.MinInputTokens ||
				tier.MaxInputTokens != nil && inputTokens > *tier.MaxInputTokens {
				continue
			}
			if tier.InputUSD != nil && tier.OutputUSD != nil {
				pair := *tier.InputUSD + *tier.OutputUSD
				return pair, !math.IsInf(pair, 0)
			}
		}
		return 0, false
	}
	if price.UsePrice || price.SupplierTariff != nil || price.QuotaPerUnit <= 0 || price.ModelRatio <= 0 && !price.FreeModel ||
		price.CompletionRatio < 0 || math.IsNaN(price.CompletionRatio) || math.IsInf(price.CompletionRatio, 0) {
		return 0, false
	}
	input := price.ModelRatio * price.GroupRatioInfo.GroupRatio * 1_000_000 / price.QuotaPerUnit
	pair := input + input*price.CompletionRatio
	return pair, input >= 0 && !math.IsNaN(pair) && !math.IsInf(pair, 0)
}

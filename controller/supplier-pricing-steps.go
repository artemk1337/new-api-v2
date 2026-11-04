package controller

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/types"
	"github.com/shopspring/decimal"
)

// Supplier steps preserve only explicitly priced lanes. The legacy converter
// supplies global-style defaults, which cannot establish a supplier's costs.
func supplierStepPricingExpression(item supplierCatalogueItem, tariff types.SupplierModelTariff) (string, error) {
	if item.QuotaType == nil || *item.QuotaType != 0 || len(item.StepRatios) == 0 || tariff.InputUSD == nil || tariff.OutputUSD == nil || item.ReasoningRatio != nil {
		return "", fmt.Errorf("unsupported supplier step pricing")
	}
	parts := make([]string, 0, len(item.StepRatios))
	reasoningSplit := false
	for _, step := range item.StepRatios {
		reasoningSplit = reasoningSplit || step.CompletionThinkingStepRatio != 0
	}
	for index, step := range item.StepRatios {
		if step.StepSize <= 0 || step.CompletionStepSize == 0 || step.CompletionStepSize < -1 || !nonNegativeFinite(step.PromptStepRatio) || !nonNegativeFinite(step.CompletionStepRatio) || !nonNegativeFinite(step.CacheStepRatio) || !nonNegativeFinite(step.PromptThinkingStepRatio) || !nonNegativeFinite(step.CompletionThinkingStepRatio) || (step.PromptThinkingStepRatio != 0 && step.PromptThinkingStepRatio != step.PromptStepRatio) {
			return "", fmt.Errorf("invalid supplier step pricing")
		}
		if index > 0 {
			previous := item.StepRatios[index-1]
			if step.StepSize < previous.StepSize || step.StepSize == previous.StepSize && !completionStepAfter(previous.CompletionStepSize, step.CompletionStepSize) {
				return "", fmt.Errorf("supplier steps must have increasing bounds")
			}
		}
		terms := make([]string, 0, 10)
		add := func(variable string, price *float64, multiplier float64) {
			if price == nil {
				return
			}
			rate := decimal.NewFromFloat(*price).Mul(decimal.NewFromFloat(multiplier))
			terms = append(terms, variable+" * "+rate.String())
		}
		add("p", tariff.InputUSD, step.PromptStepRatio)
		add("c", tariff.OutputUSD, step.CompletionStepRatio)
		add("cr", tariff.CacheReadUSD, step.CacheStepRatio)
		add("cc", tariff.CacheWriteUSD, step.CacheStepRatio)
		add("cc1h", tariff.CacheWrite1hUSD, step.CacheStepRatio)
		add("img", tariff.ImageUSD, step.PromptStepRatio)
		add("img_o", tariff.ImageOutputUSD, step.CompletionStepRatio)
		add("ai", tariff.AudioInputUSD, step.PromptStepRatio)
		add("ao", tariff.AudioOutputUSD, step.CompletionStepRatio)
		if reasoningSplit {
			multiplier := step.CompletionThinkingStepRatio
			if multiplier == 0 {
				multiplier = step.CompletionStepRatio
			}
			add("rt", tariff.OutputUSD, multiplier)
		}
		tier := "tier(\"step_" + strconv.Itoa(index+1) + "\", " + strings.Join(terms, " + ") + ")"
		if index == len(item.StepRatios)-1 {
			parts = append(parts, tier)
			continue
		}
		condition := "len <= " + strconv.Itoa(step.StepSize)
		if step.CompletionStepSize > 0 {
			length := "c"
			if tariff.AudioOutputUSD != nil {
				length += " + ao"
			}
			if tariff.ImageOutputUSD != nil {
				length += " + img_o"
			}
			if reasoningSplit {
				length += " + rt"
			}
			condition += " && " + length + " <= " + strconv.Itoa(step.CompletionStepSize)
		}
		parts = append(parts, condition+" ? "+tier+" : ")
	}
	return strings.Join(parts, ""), nil
}

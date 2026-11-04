package service

import (
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// TieredResultWrapper wraps billingexpr.TieredResult for use at the service layer.
type TieredResultWrapper = billingexpr.TieredResult

// BuildTieredTokenParams constructs billingexpr.TokenParams from a dto.Usage,
// normalizing P and C so they mean "tokens not separately priced by the
// expression". Sub-categories (cache, image, audio) are only subtracted
// when the expression references them via their own variable.
//
// GPT-format APIs report prompt_tokens / completion_tokens as totals that
// include all sub-categories (cache, image, audio). Claude-format APIs
// report them as text-only. This function normalizes to text-only when
// sub-categories are separately priced.
func BuildTieredTokenParams(usage *dto.Usage, isClaudeUsageSemantic bool, usedVars map[string]bool) billingexpr.TokenParams {
	p := float64(usage.PromptTokens)
	c := float64(usage.CompletionTokens)
	cr := float64(usage.PromptTokensDetails.CachedTokens)
	cc5m := float64(usage.PromptTokensDetails.CachedCreationTokens)
	cc1h := float64(0)

	hasSplitCacheCreation := usage.ClaudeCacheCreation5mTokens > 0 || usage.ClaudeCacheCreation1hTokens > 0
	if hasSplitCacheCreation &&
		(usage.UsageSemantic == "anthropic" || usage.UsageSource == "anthropic" || usage.UsageSource == "openai-compatible") {
		cacheCreation5m, cacheCreation1h := NormalizeCacheCreationSplit(
			usage.PromptTokensDetails.CachedCreationTokens,
			usage.ClaudeCacheCreation5mTokens,
			usage.ClaudeCacheCreation1hTokens,
		)
		cc5m = float64(cacheCreation5m)
		cc1h = float64(cacheCreation1h)
	}

	img := float64(usage.PromptTokensDetails.ImageTokens)
	ai := float64(usage.PromptTokensDetails.AudioTokens)
	imgO := float64(usage.CompletionTokenDetails.ImageTokens)
	ao := float64(usage.CompletionTokenDetails.AudioTokens)
	rt := float64(usage.CompletionTokenDetails.ReasoningTokens)
	unknown := 0.0
	fallback := false

	// len = total input context length for tier condition evaluation.
	// Non-Claude: prompt_tokens already includes everything.
	// Claude: input_tokens is text-only, so add cache read + cache creation.
	inputLen := p
	if isClaudeUsageSemantic {
		inputLen = p + cr + cc5m + cc1h
	}

	if !isClaudeUsageSemantic {
		if usedVars["cr"] {
			p -= cr
		}
		if usedVars["cc"] {
			p -= cc5m
		}
		if usedVars["cc1h"] {
			p -= cc1h
		}
		if usedVars["img"] {
			p -= img
		}
		if usedVars["ai"] {
			p -= ai
		}
		if usedVars["img_o"] {
			c -= imgO
		}
		if usedVars["ao"] {
			c -= ao
		}
	}
	if usedVars["rt"] {
		if usage.CompletionTokenDetails.ReasoningTokensPresent {
			c -= rt
		} else {
			// Keep both possible allocations so settlement can charge the
			// higher applicable output rate when the counter is absent.
			rt = c
			unknown = c
			c = 0
			fallback = true
		}
	}

	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}

	return buildTieredTokenParams(p, c, inputLen, cr, cc5m, cc1h, img, imgO, ai, ao, rt, unknown, fallback)
}

func buildTieredTokenParams(p, c, inputLen, cr, cc5m, cc1h, img, imgO, ai, ao, rt, unknown float64, fallback bool) billingexpr.TokenParams {
	return billingexpr.TokenParams{
		P:                       p,
		C:                       c,
		Len:                     inputLen,
		CR:                      cr,
		CC:                      cc5m,
		CC1h:                    cc1h,
		Img:                     img,
		ImgO:                    imgO,
		AI:                      ai,
		AO:                      ao,
		RT:                      rt,
		ReasoningTokensUnknown:  unknown,
		ReasoningTokensFallback: fallback,
	}
}

// BuildRealtimeTieredTokenParams applies the same opt-in cache and audio
// normalization to realtime usage, whose DTO exposes cache and audio details.
func BuildRealtimeTieredTokenParams(usage *dto.RealtimeUsage, usedVars map[string]bool) billingexpr.TokenParams {
	p := float64(usage.InputTokens)
	c := float64(usage.OutputTokens)
	cr := float64(usage.InputTokenDetails.CachedTokens)
	ai := float64(usage.InputTokenDetails.AudioTokens)
	ao := float64(usage.OutputTokenDetails.AudioTokens)
	rt := float64(usage.OutputTokenDetails.ReasoningTokens)
	unknown := 0.0
	fallback := false
	if usedVars["cr"] {
		p -= cr
	}
	if usedVars["ai"] {
		p -= ai
	}
	if usedVars["ao"] {
		c -= ao
	}
	if usedVars["rt"] {
		if usage.OutputTokenDetails.ReasoningTokensPresent {
			c -= rt
		} else {
			rt = c
			unknown = c
			c = 0
			fallback = true
		}
	}
	if p < 0 {
		p = 0
	}
	if c < 0 {
		c = 0
	}
	return buildRealtimeTieredTokenParams(p, c, float64(usage.InputTokens), cr, ai, ao, rt, unknown, fallback)
}

func buildRealtimeTieredTokenParams(p, c, inputLen, cr, ai, ao, rt, unknown float64, fallback bool) billingexpr.TokenParams {
	return billingexpr.TokenParams{
		P:                       p,
		C:                       c,
		Len:                     inputLen,
		CR:                      cr,
		AI:                      ai,
		AO:                      ao,
		RT:                      rt,
		ReasoningTokensUnknown:  unknown,
		ReasoningTokensFallback: fallback,
	}
}

// TryTieredSettle checks if the request uses tiered_expr billing and, if so,
// computes the actual quota using the frozen BillingSnapshot. Returns:
//   - ok=true, quota, result  when tiered billing applies
//   - ok=false, 0, nil        when it doesn't (caller should fall through to existing logic)
func TryTieredSettle(relayInfo *relaycommon.RelayInfo, params billingexpr.TokenParams) (ok bool, quota int, result *billingexpr.TieredResult) {
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil || snap.BillingMode != "tiered_expr" {
		return false, 0, nil
	}

	requestInput := billingexpr.RequestInput{}
	if relayInfo.BillingRequestInput != nil {
		requestInput = *relayInfo.BillingRequestInput
	}

	tr, err := billingexpr.ComputeTieredQuotaWithRequest(snap, params, requestInput)
	if err != nil {
		quota = relayInfo.FinalPreConsumedQuota
		if quota <= 0 {
			quota = snap.EstimatedQuotaAfterGroup
		}
		return true, quota, nil
	}
	if params.ReasoningTokensFallback {
		unknown := params.ReasoningTokensUnknown
		if unknown <= 0 {
			unknown = params.RT
		}
		known := params
		known.RT -= unknown
		known.ReasoningTokensUnknown = 0
		known.ReasoningTokensFallback = false
		reasoning := known
		reasoning.RT += unknown
		completion := known
		completion.C += unknown
		reasoningResult, reasoningErr := billingexpr.ComputeTieredQuotaWithRequest(snap, reasoning, requestInput)
		completionResult, completionErr := billingexpr.ComputeTieredQuotaWithRequest(snap, completion, requestInput)
		if reasoningErr != nil || completionErr != nil {
			safeQuota := tr.ActualQuotaAfterGroup
			if snap.EstimatedQuotaAfterGroup > safeQuota {
				safeQuota = snap.EstimatedQuotaAfterGroup
			}
			if relayInfo.FinalPreConsumedQuota > safeQuota {
				safeQuota = relayInfo.FinalPreConsumedQuota
			}
			return true, safeQuota, nil
		}
		if completionResult.ActualQuotaAfterGroup > reasoningResult.ActualQuotaAfterGroup {
			tr = completionResult
		} else {
			tr = reasoningResult
		}
	}

	return true, tr.ActualQuotaAfterGroup, &tr
}

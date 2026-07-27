package controller

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/stretchr/testify/require"
)

func TestBuildUpstreamPricingSyncPatches(t *testing.T) {
	t.Run("applies unanimous ratio update", func(t *testing.T) {
		local := map[string]any{
			"model_ratio":      map[string]any{"model-a": 1.0},
			"completion_ratio": map[string]any{"model-a": 1.0},
		}
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(local, upstreams, nil)

		require.Equal(t, 1, applied)
		require.Empty(t, skipped)
		require.Equal(t, 2.0, patches["ModelRatio"].Set["model-a"])
		require.Equal(t, 3.0, patches["CompletionRatio"].Set["model-a"])
		require.Equal(t, 1.0, valueMap(local["model_ratio"])["model-a"])
	})

	t.Run("skips billing category transition", func(t *testing.T) {
		local := map[string]any{"model_price": map[string]any{"model-a": 0.2}}
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(local, upstreams, nil)

		require.Empty(t, patches)
		require.Equal(t, []string{"model-a"}, skipped)
		require.Zero(t, applied)
	})

	t.Run("skips upstream disagreement", func(t *testing.T) {
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}},
			{"model_ratio": map[string]any{"model-a": 3.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(map[string]any{}, upstreams, nil)

		require.Empty(t, patches)
		require.Equal(t, []string{"model-a"}, skipped)
		require.Zero(t, applied)
	})

	t.Run("skips model unsupported by another upstream", func(t *testing.T) {
		upstreams := []map[string]any{
			{},
			{"model_ratio": map[string]any{"model-a": 2.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(
			map[string]any{},
			upstreams,
			map[string]struct{}{"model-a": {}},
		)

		require.Empty(t, patches)
		require.Equal(t, []string{"model-a"}, skipped)
		require.Zero(t, applied)
	})

	t.Run("deletes optional ratio absent from every source", func(t *testing.T) {
		local := map[string]any{
			"model_ratio":      map[string]any{"model-a": 2.0},
			"completion_ratio": map[string]any{"model-a": 3.0},
			"cache_ratio":      map[string]any{"model-a": 0.5},
		}
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(local, upstreams, nil)

		require.Empty(t, skipped)
		require.Equal(t, 1, applied)
		require.Equal(t, []string{"model-a"}, patches["CacheRatio"].Delete)
	})

	t.Run("skips mixed optional ratio presence", func(t *testing.T) {
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}, "cache_ratio": map[string]any{"model-a": 0.5}},
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(map[string]any{}, upstreams, nil)

		require.Empty(t, patches)
		require.Equal(t, []string{"model-a"}, skipped)
		require.Zero(t, applied)
	})
}

func TestIsUpstreamPricingSyncChannel(t *testing.T) {
	hosts := map[string]struct{}{"yunwu.ai": {}}
	require.True(t, isUpstreamPricingSyncChannel(&model.Channel{BaseURL: ptr("https://yunwu.ai")}, hosts))
	require.False(t, isUpstreamPricingSyncChannel(&model.Channel{BaseURL: ptr("http://yunwu.ai")}, hosts))
	require.False(t, isUpstreamPricingSyncChannel(&model.Channel{BaseURL: ptr("https://other.example")}, hosts))
}

func TestPricingSourceIdentityNormalizesURLWithoutChangingPathCase(t *testing.T) {
	channel := &model.Channel{Id: 7, BaseURL: ptr("HTTPS://YUNWU.AI/Api/")}
	require.Equal(t, "7:https://yunwu.ai/Api", pricingSourceIdentity(channel))
}

func TestFetchChannelPricingStopsReadingBodyAtContextDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := fetchChannelPricing(ctx, &model.Channel{BaseURL: ptr(server.URL)}, nil)

	require.Error(t, err)
	require.True(t, errors.Is(err, context.DeadlineExceeded), err)
}

func TestFetchChannelPricingRejectsRedirectOutsideAllowlist(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://untrusted.example/api/pricing", http.StatusFound)
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)

	_, err = fetchChannelPricingWithClient(
		context.Background(),
		&model.Channel{BaseURL: ptr(server.URL)},
		map[string]struct{}{parsed.Hostname(): {}},
		server.Client(),
	)

	require.ErrorContains(t, err, "redirect target is not allowlisted")
}

func TestPricingStepRatiosExpr(t *testing.T) {
	cacheRatio := 0.1
	createCacheRatio := 1.25
	imageRatio := 0.5
	audioRatio := 4.0
	audioCompletionRatio := 5.0
	exprText, ok := pricingStepRatiosExpr(upstreamPricingItem{
		ModelRatio:           3,
		CompletionRatio:      4,
		CacheRatio:           &cacheRatio,
		CreateCacheRatio:     &createCacheRatio,
		ImageRatio:           &imageRatio,
		AudioRatio:           &audioRatio,
		AudioCompletionRatio: &audioCompletionRatio,
		StepRatios: []upstreamPricingStep{
			{StepSize: 32000, CompletionStepSize: -1, PromptStepRatio: 1, CompletionStepRatio: 1, CacheStepRatio: 1},
			{StepSize: 128000, CompletionStepSize: -1, PromptStepRatio: 1.5, CompletionStepRatio: 1.5},
			{StepSize: 256000, CompletionStepSize: -1, PromptStepRatio: 2.5, CompletionStepRatio: 2.5},
			{StepSize: 10000000, CompletionStepSize: -1, PromptStepRatio: 0.5, CompletionStepRatio: 0.125},
		},
	})
	require.True(t, ok)
	standard, _, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{P: 1, C: 1, Len: 32000})
	require.NoError(t, err)
	require.Equal(t, 30.0, standard)
	longContext, _, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{P: 1, C: 1, Len: 128001})
	require.NoError(t, err)
	require.Equal(t, 75.0, longContext)
	require.Contains(t, exprText, `len <= 32000`)
	require.Contains(t, exprText, `p * 6 + c * 24`)
	cacheCreation5m, _, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{CC: 1, Len: 32000})
	require.NoError(t, err)
	cacheCreation1h, _, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{CC1h: 1, Len: 32000})
	require.NoError(t, err)
	require.Equal(t, cacheCreation5m*upstreamCacheCreation1hMultiplier, cacheCreation1h)
	multimodal, _, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{Img: 1, AI: 1, AO: 1, Len: 32000})
	require.NoError(t, err)
	require.Equal(t, 3.0+24.0+120.0, multimodal)
}

func TestPricingStepRatiosExprUsesLegacyDefaultsForOptionalRatios(t *testing.T) {
	exprText, ok := pricingStepRatiosExpr(upstreamPricingItem{
		ModelRatio:      2,
		CompletionRatio: 3,
		StepRatios: []upstreamPricingStep{{
			StepSize:            1000,
			CompletionStepSize:  -1,
			PromptStepRatio:     1,
			CompletionStepRatio: 1,
			CacheStepRatio:      1,
		}},
	})
	require.True(t, ok)

	tests := []struct {
		name   string
		params billingexpr.TokenParams
		want   float64
	}{
		{name: "cache read", params: billingexpr.TokenParams{CR: 1}, want: 4},
		{name: "cache creation 5m", params: billingexpr.TokenParams{CC: 1}, want: 5},
		{name: "cache creation 1h", params: billingexpr.TokenParams{CC1h: 1}, want: 8},
		{name: "audio input", params: billingexpr.TokenParams{AI: 1}, want: 4},
		{name: "audio output", params: billingexpr.TokenParams{AO: 1}, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, _, err := billingexpr.RunExpr(exprText, tt.params)
			require.NoError(t, err)
			require.Equal(t, tt.want, cost)
		})
	}
}

func TestUpstreamPricingItemsRejectInvalidValues(t *testing.T) {
	negative := -0.1
	tests := []upstreamPricingItem{
		{ModelName: "negative-ratio", ModelRatio: -1, CompletionRatio: 1},
		{ModelName: "infinite-price", QuotaType: 1, ModelPrice: math.Inf(1)},
		{ModelName: "negative-optional", ModelRatio: 1, CompletionRatio: 1, CacheRatio: &negative},
		{ModelName: "invalid-expr", BillingMode: billing_setting.BillingModeTieredExpr, BillingExpr: `tier("x", p *)`},
		{ModelName: "negative-expr", BillingMode: billing_setting.BillingModeTieredExpr, BillingExpr: `tier("x", p * -1)`},
		{ModelName: "infinite-expr", BillingMode: billing_setting.BillingModeTieredExpr, BillingExpr: `tier("x", p * 1e308)`},
		{ModelName: "missing-expr", BillingMode: billing_setting.BillingModeTieredExpr},
	}

	data, unsupported := upstreamPricingItemsToSyncData(tests)

	require.ElementsMatch(t, []string{"negative-ratio", "infinite-price", "negative-optional", "invalid-expr", "negative-expr", "infinite-expr", "missing-expr"}, unsupported)
	for _, field := range pricingSyncFields {
		require.Empty(t, valueMap(data[field]))
	}
}

func TestUpstreamPricingItemsAcceptZeroValues(t *testing.T) {
	zero := 0.0
	data, unsupported := upstreamPricingItemsToSyncData([]upstreamPricingItem{{
		ModelName:       "free-model",
		ModelRatio:      0,
		CompletionRatio: 0,
		CacheRatio:      &zero,
	}})

	require.Empty(t, unsupported)
	require.Equal(t, 0.0, valueMap(data["model_ratio"])["free-model"])
	require.Equal(t, 0.0, valueMap(data["cache_ratio"])["free-model"])
}

func TestUpstreamPricingItemsRejectDuplicateModel(t *testing.T) {
	data, unsupported := upstreamPricingItemsToSyncData([]upstreamPricingItem{
		{ModelName: "duplicate", ModelRatio: 1, CompletionRatio: 2},
		{ModelName: "duplicate", ModelRatio: 3, CompletionRatio: 4},
	})

	require.Equal(t, []string{"duplicate"}, unsupported)
	for _, field := range pricingSyncFields {
		require.NotContains(t, valueMap(data[field]), "duplicate")
	}
}

func TestPricingCategoryPrefersTieredBilling(t *testing.T) {
	data := map[string]any{
		"model_price":                    map[string]any{"model-a": 0.1},
		billing_setting.BillingModeField: map[string]any{"model-a": billing_setting.BillingModeTieredExpr},
		billing_setting.BillingExprField: map[string]any{"model-a": `tier("x", p)`},
	}
	require.Equal(t, "tiered", pricingCategory(data, "model-a"))
}

func TestPricingCategoryUsesTieredModeEvenWithoutExpression(t *testing.T) {
	data := map[string]any{
		"model_price":                    map[string]any{"model-a": 0.1},
		billing_setting.BillingModeField: map[string]any{"model-a": billing_setting.BillingModeTieredExpr},
	}
	require.Equal(t, "tiered", pricingCategory(data, "model-a"))
}

func TestUnsupportedPricingStepsDoNotFallBackToBaseRatio(t *testing.T) {
	data, unsupported := upstreamPricingItemsToSyncData([]upstreamPricingItem{{
		ModelName:       "thinking-model",
		ModelRatio:      2,
		CompletionRatio: 4,
		StepRatios: []upstreamPricingStep{{
			StepSize:                    128000,
			PromptStepRatio:             1,
			CompletionStepRatio:         1,
			PromptThinkingStepRatio:     2,
			CompletionThinkingStepRatio: 3,
		}},
	}})

	require.Equal(t, []string{"thinking-model"}, unsupported)
	require.NotContains(t, valueMap(data["model_ratio"]), "thinking-model")
	require.NotContains(t, valueMap(data["completion_ratio"]), "thinking-model")
}

func TestPricingStepRatiosExprRejectsInvalidOrder(t *testing.T) {
	validStep := func(stepSize, completionStepSize int) upstreamPricingStep {
		return upstreamPricingStep{
			StepSize:            stepSize,
			CompletionStepSize:  completionStepSize,
			PromptStepRatio:     1,
			CompletionStepRatio: 1,
		}
	}
	tests := map[string][]upstreamPricingStep{
		"decreasing input bound": {
			validStep(128000, -1),
			validStep(32000, -1),
		},
		"duplicate bounds": {
			validStep(128000, 32000),
			validStep(128000, 32000),
		},
		"range after unbounded completion is unreachable": {
			validStep(128000, -1),
			validStep(128000, 64000),
		},
	}

	for name, steps := range tests {
		t.Run(name, func(t *testing.T) {
			_, ok := pricingStepRatiosExpr(upstreamPricingItem{
				ModelRatio:      1,
				CompletionRatio: 1,
				StepRatios:      steps,
			})
			require.False(t, ok)
		})
	}
}

func TestPricingStepRatiosExprUsesTotalAudioOutputForTierCondition(t *testing.T) {
	audioRatio := 2.0
	audioCompletionRatio := 3.0
	exprText, ok := pricingStepRatiosExpr(upstreamPricingItem{
		ModelRatio:           1,
		CompletionRatio:      1,
		AudioRatio:           &audioRatio,
		AudioCompletionRatio: &audioCompletionRatio,
		StepRatios: []upstreamPricingStep{
			{StepSize: 1000, CompletionStepSize: 100, PromptStepRatio: 1, CompletionStepRatio: 1},
			{StepSize: 2000, CompletionStepSize: -1, PromptStepRatio: 2, CompletionStepRatio: 2},
		},
	})

	require.True(t, ok)
	require.Contains(t, exprText, "c + ao <= 100")
	_, trace, err := billingexpr.RunExpr(exprText, billingexpr.TokenParams{C: 80, AO: 30, Len: 100})
	require.NoError(t, err)
	require.Equal(t, "step_2", trace.MatchedTier)
}

func TestPricingCandidateRequiresTwoIdenticalChecks(t *testing.T) {
	patches := map[string]model.JSONObjectPatch{"ModelRatio": {Set: map[string]any{"model-a": 2.0}}}
	firstHash, err := pricingCandidateHash([]string{"2:https://yunwu.ai", "1:https://yunwu.ai"}, patches)
	require.NoError(t, err)
	sameHash, err := pricingCandidateHash([]string{"1:https://yunwu.ai", "2:https://yunwu.ai"}, patches)
	require.NoError(t, err)
	changedHash, err := pricingCandidateHash([]string{"1:https://yunwu.ai", "2:https://yunwu.ai"}, map[string]model.JSONObjectPatch{"ModelRatio": {Set: map[string]any{"model-a": 3.0}}})
	require.NoError(t, err)
	changedSourcesHash, err := pricingCandidateHash([]string{"1:https://yunwu.ai"}, patches)
	require.NoError(t, err)

	require.False(t, pricingCandidateConfirmed("", firstHash))
	require.True(t, pricingCandidateConfirmed(firstHash, sameHash))
	require.False(t, pricingCandidateConfirmed(firstHash, changedHash))
	require.False(t, pricingCandidateConfirmed(firstHash, changedSourcesHash))
}

func ptr(value string) *string { return &value }

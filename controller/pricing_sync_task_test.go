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

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/glebarez/sqlite"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
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

	t.Run("resolves upstream disagreement with highest price", func(t *testing.T) {
		upstreams := []map[string]any{
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 1.0}},
			{"model_ratio": map[string]any{"model-a": 3.0}, "completion_ratio": map[string]any{"model-a": 1.0}},
		}

		patches, skipped, applied := buildUpstreamPricingSyncPatches(map[string]any{}, upstreams, nil)

		require.Empty(t, skipped)
		require.Equal(t, 1, applied)
		require.Equal(t, 3.0, patches["ModelRatio"].Set["model-a"])
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

	t.Run("honors the selected source for a model", func(t *testing.T) {
		upstreams := []map[string]any{
			{"model_price": map[string]any{"model-a": 1.0}},
			{"model_price": map[string]any{"model-a": 2.0}},
		}
		patches, skipped, applied := buildUpstreamPricingSyncPatchesWithPreferences(
			map[string]any{},
			upstreams,
			[]int{10, 20},
			nil,
			map[string]model.PricingSyncModelState{
				"model-a": {Mode: model.PricingSyncModelModeChannel, ChannelID: 10},
			},
		)

		require.Empty(t, skipped)
		require.Equal(t, 1, applied)
		require.Equal(t, 1.0, patches["ModelPrice"].Set["model-a"])
	})

	t.Run("selected source replaces a different local billing category", func(t *testing.T) {
		patches, skipped, applied := buildUpstreamPricingSyncPatchesWithPreferences(
			map[string]any{"model_price": map[string]any{"model-a": 0.5}},
			[]map[string]any{{
				"model_ratio":      map[string]any{"model-a": 2.0},
				"completion_ratio": map[string]any{"model-a": 3.0},
			}},
			[]int{10},
			nil,
			map[string]model.PricingSyncModelState{
				"model-a": {Mode: model.PricingSyncModelModeChannel, ChannelID: 10},
			},
		)

		require.Empty(t, skipped)
		require.Equal(t, 1, applied)
		require.Equal(t, 2.0, patches["ModelRatio"].Set["model-a"])
		require.Equal(t, 3.0, patches["CompletionRatio"].Set["model-a"])
		require.Equal(t, []string{"model-a"}, patches["ModelPrice"].Delete)
	})

	t.Run("never overwrites a manually priced model", func(t *testing.T) {
		patches, skipped, applied := buildUpstreamPricingSyncPatchesWithPreferences(
			map[string]any{},
			[]map[string]any{{"model_price": map[string]any{"model-a": 2.0}}},
			[]int{10},
			nil,
			map[string]model.PricingSyncModelState{
				"model-a": {Mode: model.PricingSyncModelModeManual},
			},
		)

		require.Empty(t, patches)
		require.Equal(t, []string{"model-a"}, skipped)
		require.Zero(t, applied)
	})
}

func TestResolveComparableNumericPricingAverageRatioUsesLanePrices(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	original := common.OptionMap
	options := make(map[string]string, len(original)+1)
	for key, value := range original {
		options[key] = value
	}
	options["PricingSyncStrategy"] = model.PricingSyncStrategyAverage
	common.OptionMap = options
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = original
		common.OptionMapRWMutex.Unlock()
	})

	resolved, ok := resolveComparableNumericPricing([]map[string]any{
		{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 4.0}, "cache_ratio": map[string]any{"model-a": 0.5}},
		{"model_ratio": map[string]any{"model-a": 6.0}, "completion_ratio": map[string]any{"model-a": 2.0}, "cache_ratio": map[string]any{"model-a": 0.25}},
	}, "model-a", "ratio")

	require.True(t, ok)
	require.Equal(t, 4.0, resolved["model_ratio"])
	// Average completion price is (2*4 + 6*2)/2 = 10; 10 / 4 = 2.5.
	require.Equal(t, 2.5, resolved["completion_ratio"])
	require.Equal(t, 0.3125, resolved["cache_ratio"])
}

func TestResolveComparableNumericPricingHighestUsesActualLanePrices(t *testing.T) {
	common.OptionMapRWMutex.Lock()
	original := common.OptionMap
	options := make(map[string]string, len(original)+1)
	for key, value := range original {
		options[key] = value
	}
	options["PricingSyncStrategy"] = model.PricingSyncStrategyHighest
	common.OptionMap = options
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		common.OptionMap = original
		common.OptionMapRWMutex.Unlock()
	})

	resolved, ok := resolveComparableNumericPricing([]map[string]any{
		{"model_ratio": map[string]any{"model-a": 5.0}, "completion_ratio": map[string]any{"model-a": 2.0}},
		{"model_ratio": map[string]any{"model-a": 4.0}, "completion_ratio": map[string]any{"model-a": 30.0}},
	}, "model-a", "ratio")

	require.True(t, ok)
	require.Equal(t, 5.0, resolved["model_ratio"])
	require.Equal(t, 24.0, resolved["completion_ratio"])
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

func TestFetchPricingSyncURLParsesRatioConfig(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"model_ratio":{"model-a":2},"completion_ratio":{"model-a":3}}}`))
	}))
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)

	fetched, err := fetchPricingSyncURL(context.Background(), server.URL+"/api/ratio_config", "ratio_config", map[string]struct{}{parsed.Hostname(): {}}, "", server.Client())

	require.NoError(t, err)
	require.Equal(t, 2.0, valueMap(fetched.Data["model_ratio"])["model-a"])
	require.Equal(t, 3.0, valueMap(fetched.Data["completion_ratio"])["model-a"])
}

func TestPricingSyncFetchTargetRejectsProtocolRelativeEndpoint(t *testing.T) {
	_, err := pricingSyncFetchTargetForSource(
		model.PricingSyncSource{ChannelID: 1, Endpoint: "//untrusted.example/prices"},
		&model.Channel{Id: 1, BaseURL: ptr("https://trusted.example")},
	)

	require.ErrorContains(t, err, "relative path")
}

func TestPricingSyncFetchTargetAcceptsOfficialAbsolutePreset(t *testing.T) {
	const endpoint = "https://basellm.github.io/llm-metadata/api/newapi/ratio_config-v1-base.json"
	target, err := pricingSyncFetchTargetForSource(
		model.PricingSyncSource{ChannelID: officialRatioPresetID, Endpoint: endpoint},
		nil,
	)

	require.NoError(t, err)
	require.Equal(t, endpoint, target.URL)
	require.Equal(t, "ratio_config", target.Mode)
}

func TestConfirmPricingSyncQuotesRequiresTwoIdenticalSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-quotes?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncQuote{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	first := map[string]any{
		"model_ratio":      map[string]any{"model-a": 2.0},
		"completion_ratio": map[string]any{"model-a": 3.0},
	}
	confirmed, err := confirmPricingSyncQuotes(8, first, 100)
	require.NoError(t, err)
	require.Empty(t, confirmed)

	confirmed, err = confirmPricingSyncQuotes(8, first, 101)
	require.NoError(t, err)
	require.Equal(t, 2.0, valueMap(confirmed["model_ratio"])["model-a"])

	changed := map[string]any{
		"model_ratio":      map[string]any{"model-a": 4.0},
		"completion_ratio": map[string]any{"model-a": 3.0},
	}
	confirmed, err = confirmPricingSyncQuotes(8, changed, 102)
	require.NoError(t, err)
	require.Empty(t, confirmed)
}

func TestConfirmPricingSyncQuotesTreatsZeroBaseAsMissing(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-zero?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncQuote{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.Create(&model.PricingSyncQuote{
		ChannelID: 8, ModelName: "model-a", Confirmations: 2,
		Data: `{"model_ratio":2,"completion_ratio":3}`,
	}).Error)

	zero := map[string]any{
		"model_ratio":      map[string]any{"model-a": 0.0},
		"completion_ratio": map[string]any{"model-a": 3.0},
	}
	_, err = confirmPricingSyncQuotes(8, zero, 100)
	require.NoError(t, err)
	_, err = confirmPricingSyncQuotes(8, zero, 101)
	require.NoError(t, err)

	var quote model.PricingSyncQuote
	require.NoError(t, db.First(&quote, "channel_id = ? AND model_name = ?", 8, "model-a").Error)
	require.Equal(t, 2, quote.MissingCount)
	require.Zero(t, quote.Confirmations)
}

func TestPricingSyncMissingModelsRequiresOwnedAbsentPrice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-missing?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncQuote{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.Create(&model.PricingSyncQuote{
		ChannelID: 8, ModelName: "model-a", MissingCount: 2,
	}).Error)

	missing, err := pricingSyncMissingModels(
		[]int{8},
		map[string]model.PricingSyncModelState{
			"model-a": {
				ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral,
				Provenance: "[8]",
			},
		},
	)

	require.NoError(t, err)
	require.Contains(t, missing, "model-a")
}

func TestPricingSyncMissingModelsDoesNotFallbackFromLostProvenance(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-missing-provenance?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PricingSyncQuote{}))
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })
	require.NoError(t, db.Create(&model.PricingSyncQuote{
		ChannelID: 8, ModelName: "model-a", MissingCount: 2,
	}).Error)
	require.NoError(t, db.Create(&model.PricingSyncQuote{
		ChannelID: 9, ModelName: "model-a", Confirmations: 2,
	}).Error)

	missing, err := pricingSyncMissingModels(
		[]int{8, 9},
		map[string]model.PricingSyncModelState{
			"model-a": {
				ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral,
				Provenance: "[8,9]",
			},
		},
	)

	require.NoError(t, err)
	require.Contains(t, missing, "model-a")
}

func TestRemoveUnavailablePricingSyncModelsDropsFallbackSet(t *testing.T) {
	patches := map[string]model.JSONObjectPatch{
		"ModelPrice": {
			Set: map[string]any{"model-a": 0.5, "model-b": 1.0},
		},
	}

	removeUnavailablePricingSyncModels(patches, map[string]struct{}{"model-a": {}})

	require.NotContains(t, patches["ModelPrice"].Set, "model-a")
	require.Equal(t, 1.0, patches["ModelPrice"].Set["model-b"])
	require.Contains(t, patches["ModelPrice"].Delete, "model-a")
}

func TestPricingSyncSourceWritesRequireCurrentConfiguration(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:pricing-sync-source-version?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&model.Option{},
		&model.PricingSyncSource{},
		&model.PricingSyncQuote{},
	))
	require.NoError(t, db.Create(&model.Option{Key: "PricingSyncConfigVersion", Value: "2"}).Error)
	current := model.PricingSyncSource{
		ChannelID: 8, Enabled: true, Endpoint: "/api/pricing", IntervalSeconds: 60,
	}
	require.NoError(t, db.Create(&current).Error)
	originalDB := model.DB
	model.DB = db
	t.Cleanup(func() { model.DB = originalDB })

	stale := current
	stale.Endpoint = "/api/ratio_config"
	require.Error(t, updatePricingSyncSourceIfCurrent(stale, 2, map[string]any{"last_attempt_at": 100}))
	_, err = confirmPricingSyncQuotesIfCurrent(
		current,
		map[string]any{"model_price": map[string]any{"model-a": 1.0}},
		100,
		1,
	)
	require.Error(t, err)

	var source model.PricingSyncSource
	require.NoError(t, db.First(&source, "channel_id = ?", 8).Error)
	require.Zero(t, source.LastAttemptAt)
	var quoteCount int64
	require.NoError(t, db.Model(&model.PricingSyncQuote{}).Count(&quoteCount).Error)
	require.Zero(t, quoteCount)
}

func TestPricingSyncAppliedStatesTracksRealConflictsWithoutPricePatch(t *testing.T) {
	preferences := map[string]model.PricingSyncModelState{
		"same": {ModelName: "same", Mode: model.PricingSyncModelModeGeneral, Status: model.PricingSyncModelStatusStale},
		"diff": {ModelName: "diff", Mode: model.PricingSyncModelModeGeneral},
	}
	states := pricingSyncAppliedStates(nil, nil, nil, []map[string]any{
		{"model_price": map[string]any{"same": 1.0, "diff": 1.0}},
		{"model_price": map[string]any{"same": 1.0, "diff": 2.0}},
	}, []int{10, 20}, preferences, 100)
	byName := lo.SliceToMap(states, func(state model.PricingSyncModelState) (string, model.PricingSyncModelState) {
		return state.ModelName, state
	})

	require.Equal(t, model.PricingSyncModelStatusReady, byName["same"].Status)
	require.Equal(t, "[10,20]", byName["same"].Provenance)
	require.Equal(t, model.PricingSyncModelStatusConflict, byName["diff"].Status)
	require.NotEmpty(t, byName["diff"].ConflictDetails)
}

func TestPricingSyncAppliedStatesKeepsAndClearsStaleStatus(t *testing.T) {
	preferences := map[string]model.PricingSyncModelState{
		"model-a": {ModelName: "model-a", Mode: model.PricingSyncModelModeGeneral},
	}
	upstreams := []map[string]any{
		{"model_price": map[string]any{"model-a": 1.0}},
	}

	stale := pricingSyncAppliedStates(nil, nil, map[int]struct{}{8: {}}, upstreams, []int{8}, preferences, 100)
	require.Len(t, stale, 1)
	require.Equal(t, model.PricingSyncModelStatusStale, stale[0].Status)

	recovered := pricingSyncAppliedStates(nil, nil, nil, upstreams, []int{8}, preferences, 101)
	require.Len(t, recovered, 1)
	require.Equal(t, model.PricingSyncModelStatusReady, recovered[0].Status)
}

func TestPricingSyncIncompatibleStatesCreatesInitialConflict(t *testing.T) {
	states := pricingSyncIncompatibleStates(
		map[string]any{},
		[]map[string]any{
			{"model_price": map[string]any{"model-a": 1.0}},
			{"model_ratio": map[string]any{"model-a": 2.0}, "completion_ratio": map[string]any{"model-a": 3.0}},
		},
		[]int{8, 9},
		nil,
		nil,
		100,
	)

	require.Len(t, states, 1)
	require.Equal(t, "model-a", states[0].ModelName)
	require.Equal(t, model.PricingSyncModelModeGeneral, states[0].Mode)
	require.Equal(t, model.PricingSyncModelStatusConflict, states[0].Status)
	require.JSONEq(t, `[8,9]`, states[0].Provenance)
	require.NotEmpty(t, states[0].ConflictDetails)
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

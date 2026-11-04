package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSupplierPricingBBLABUNormalizesActualUSDOnce(t *testing.T) {
	body := []byte(`{"success":true,"group_ratio":{"kiro":1},"usable_group":{"kiro":"Kiro"},"data":[{"model_name":"claude-test","quota_type":0,"model_ratio":1,"completion_ratio":5,"cache_ratio":0.1,"create_cache_ratio":1.25,"enable_groups":["kiro","other"]}]}`)
	tariffs, err := normalizeSupplierCatalogue(body, "bblabu", model.SupplierPricingProfile{UpstreamGroup: "kiro", USDPerCredit: 0.006})
	require.NoError(t, err)
	require.Len(t, tariffs, 1)
	require.Equal(t, 0.012, *tariffs[0].InputUSD)
	require.Equal(t, 0.06, *tariffs[0].OutputUSD)
	require.Equal(t, 0.0012, *tariffs[0].CacheReadUSD)
	require.Equal(t, 0.015, *tariffs[0].CacheWriteUSD)
	require.Equal(t, 0.06, *tariffs[0].ReasoningUSD)
	require.Nil(t, tariffs[0].CacheWrite1hUSD)
	require.Nil(t, tariffs[0].AudioInputUSD)
	_, err = normalizeSupplierCatalogue(body, "bblabu", model.SupplierPricingProfile{UpstreamGroup: "kiro", USDPerCredit: 0})
	require.Error(t, err)
}

func TestSupplierPricingPreservesContextBoundaryAndCachePrices(t *testing.T) {
	body := []byte(`{"success":true,"group_ratio":{"OpenAI":1},"usable_group":{"OpenAI":"GPT"},"data":[{"model_name":"gpt-test","quota_type":0,"model_ratio":1,"completion_ratio":5,"cache_ratio":0.05,"create_cache_ratio":1.25,"enable_groups":["OpenAI"],"billing_mode":"tiered_expr","billing_expr":"v1:len <= 272000 ? tier(\"standard\", p*2+c*10+cr*0.1+cc*2.5) : tier(\"long\", p*4+c*15+cr*0.2+cc*2.5)"}]}`)
	tariffs, err := normalizeSupplierCatalogue(body, "bblabu", model.SupplierPricingProfile{UpstreamGroup: "OpenAI", USDPerCredit: 0.006})
	require.NoError(t, err)
	require.Len(t, tariffs, 1)
	require.Equal(t, types.SupplierBillingModeExpression, tariffs[0].BillingMode)
	for _, test := range []struct {
		length float64
		cost   float64
		tier   string
	}{
		{272000, 0.012 + 0.06 + 0.0006 + 0.015, "standard"},
		{272001, 0.024 + 0.09 + 0.0012 + 0.015, "long"},
	} {
		cost, tier, err := billingexpr.RunExprWithRequest(tariffs[0].Expression, billingexpr.TokenParams{P: 1, C: 1, CR: 1, CC: 1, Len: test.length}, billingexpr.RequestInput{})
		require.NoError(t, err)
		require.InDelta(t, test.cost, cost, 1e-12)
		require.Equal(t, test.tier, tier.MatchedTier)
	}
}

func TestSupplierPricingRelayRouteReplacesDefaultDiscount(t *testing.T) {
	body := []byte(`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"Default"},"data":[{"model_name":"gpt-test","quota_type":0,"model_ratio":1,"completion_ratio":5,"cache_ratio":0.05,"enable_groups":["default"],"group_ratio":{"default":0.02}}],"routing_sources":[{"tier":"special","family":"gpt","status":"active","models":["gpt-test"],"model_discounts":[{"model":"gpt-test","price_ratio":0.015}]}]}`)
	tariffs, err := normalizeSupplierCatalogue(body, "relay-fast", model.SupplierPricingProfile{UpstreamGroup: "gpt:special", USDPerCredit: 1})
	require.NoError(t, err)
	require.Len(t, tariffs, 1)
	require.Equal(t, 0.03, *tariffs[0].InputUSD)
	require.Equal(t, 0.15, *tariffs[0].OutputUSD)
	require.Equal(t, 0.0015, *tariffs[0].CacheReadUSD)
	defaultTariffs, err := normalizeSupplierCatalogue(body, "relay-fast", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
	require.NoError(t, err)
	require.Equal(t, 0.04, *defaultTariffs[0].InputUSD)
	conflicting := strings.Replace(string(body), `"price_ratio":0.015`, `"price_ratio":0.015,"price_multiplier":0.02`, 1)
	_, err = normalizeSupplierCatalogue([]byte(conflicting), "relay-fast", model.SupplierPricingProfile{UpstreamGroup: "gpt:special", USDPerCredit: 1})
	require.ErrorContains(t, err, "conflicting price_ratio/price_multiplier")
}

func TestSupplierPricingRejectsIncompleteOrUnsupportedCatalogue(t *testing.T) {
	tests := []string{
		`{"success":false,"data":[]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":null}`,
		`{"success":true,"group_ratio":{"other":1},"usable_group":{"other":"D"},"data":[]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{},"data":[]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":0,"model_ratio":1,"enable_groups":["default"]}]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":1,"enable_groups":["default"]}]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":0,"model_ratio":-1,"completion_ratio":-1,"enable_groups":["default"]}]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":0,"model_ratio":1,"completion_ratio":5,"billing_multiplier":2,"enable_groups":["default"]}]}`,
		`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":0,"model_ratio":1,"completion_ratio":5,"tool_prices":{"web_search":10},"enable_groups":["default"]}]}`,
	}
	for _, body := range tests {
		_, err := normalizeSupplierCatalogue([]byte(body), "new-api", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
		require.Error(t, err, body)
	}
}

func TestSupplierPricingSupportsExplicitZeroFixedUnitsAndToolConversion(t *testing.T) {
	body := []byte(`{"success":true,"group_ratio":{"default":2},"usable_group":{"default":"D"},"data":[{"model_name":"free-input","quota_type":0,"model_ratio":0,"completion_ratio":5,"cache_ratio":0,"tool_prices":{"web_search":10},"tool_price_unit":"per_1000_calls","enable_groups":["default"]},{"model_name":"video","quota_type":1,"model_price":3,"task_price_unit":"per_second","enable_groups":["default"]}]}`)
	tariffs, err := normalizeSupplierCatalogue(body, "new-api", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 0.5})
	require.NoError(t, err)
	require.Len(t, tariffs, 2)
	require.NotNil(t, tariffs[0].InputUSD)
	require.Zero(t, *tariffs[0].InputUSD)
	require.NotNil(t, tariffs[0].CacheReadUSD)
	require.Zero(t, *tariffs[0].CacheReadUSD)
	require.Equal(t, 0.01, tariffs[0].Tools["web_search"])
	require.Equal(t, types.SupplierBillingModeFixed, tariffs[1].BillingMode)
	require.Equal(t, "per_second", tariffs[1].PriceUnit)
	require.Equal(t, 3.0, *tariffs[1].PriceUSD)
}

func TestSupplierPricingRejectsUnsafeOriginsAndPrivateDialTargets(t *testing.T) {
	for _, profile := range []model.SupplierPricingProfile{
		{BaseURL: "http://provider.example", Endpoint: "/api/pricing"},
		{BaseURL: "https://user:password@provider.example", Endpoint: "/api/pricing"},
		{BaseURL: "https://provider.example:8443", Endpoint: "/api/pricing"},
		{BaseURL: "https://provider.example", Endpoint: "//localhost/api/pricing"},
		{BaseURL: "https://provider.example", Endpoint: "https://localhost/api/pricing"},
	} {
		_, err := supplierPricingURL(profile)
		require.Error(t, err)
	}
	for _, address := range []string{"127.0.0.1:443", "169.254.169.254:443", "[::1]:443", "10.0.0.1:443", "provider.example:80"} {
		_, err := supplierPricingDialContext(context.Background(), "tcp", address)
		require.Error(t, err, address)
	}
	origin, err := url.Parse("https://provider.example/api/pricing")
	require.NoError(t, err)
	client := newSupplierPricingHTTPClient(origin)
	for _, target := range []string{"http://provider.example/p", "https://other.example/p", "https://user:pass@provider.example/p"} {
		request, err := http.NewRequest(http.MethodGet, target, nil)
		require.NoError(t, err)
		require.Error(t, client.CheckRedirect(request, nil))
	}
}

func TestSupplierPricingEndpointsHideProcurementFromNonRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []int{0, common.RoleCommonUser, common.RoleAdminUser} {
		for _, handler := range []gin.HandlerFunc{GetSupplierPricingTariffs, PublishSupplierPricingTariffs, SyncSupplierPricingProfile} {
			response := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(response)
			c.Set("role", role)
			c.Params = gin.Params{{Key: "id", Value: "1"}}
			c.Request = httptest.NewRequest(http.MethodPost, "/api/suppliers/profiles/1/tariffs", strings.NewReader(`{}`))
			handler(c)
			require.Equal(t, http.StatusForbidden, response.Code)
			require.NotContains(t, response.Body.String(), "input_usd")
			require.NotContains(t, response.Body.String(), "base_url")
		}
	}
}

func TestSupplierPricingPublicCatalogueSamples(t *testing.T) {
	// Samples preserve published billing fields from public catalogues captured
	// on 2026-11-04; they contain no keys or account-specific information.
	load := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		require.NoError(t, err)
		return data
	}
	find := func(tariffs []types.SupplierModelTariff, name string) types.SupplierModelTariff {
		for _, tariff := range tariffs {
			if tariff.ModelName == name {
				return tariff
			}
		}
		t.Fatalf("missing fixture model %s", name)
		return types.SupplierModelTariff{}
	}
	t.Run("openlux-exact-cache-ttls", func(t *testing.T) {
		tariffs, err := normalizeSupplierCatalogue(load("supplier-openlux-claude.json"), "openlux", model.SupplierPricingProfile{UpstreamGroup: "Kiro-Claude-1", USDPerCredit: 1})
		require.NoError(t, err)
		sonnet := find(tariffs, "claude-sonnet-5-5")
		require.Equal(t, 0.17648, *sonnet.InputUSD)
		require.Equal(t, 0.8824, *sonnet.OutputUSD)
		require.Equal(t, 0.008824, *sonnet.CacheReadUSD)
		require.Equal(t, 0.2206, *sonnet.CacheWriteUSD)
		require.Equal(t, 0.35296, *sonnet.CacheWrite1hUSD)
		haiku := find(tariffs, "claude-haiku-5-5")
		require.Equal(t, types.SupplierBillingModeExpression, haiku.BillingMode)
		variables := billingexpr.UsedVars(haiku.Expression)
		require.False(t, variables["ai"], "a missing source audio lane must not gain a default")
		require.False(t, variables["ao"])
		for _, test := range []struct{ length, cost float64 }{{100000, 0.017648}, {100001, 0.08824}} {
			cost, _, err := billingexpr.RunExprWithRequest(haiku.Expression, billingexpr.TokenParams{CC1h: 1, Len: test.length}, billingexpr.RequestInput{})
			require.NoError(t, err)
			require.InDelta(t, test.cost, cost, 1e-12)
		}
	})
	t.Run("bblabu-clause-and-expression", func(t *testing.T) {
		body := load("supplier-bblabu.json")
		claude, err := normalizeSupplierCatalogue(body, "bblabu", model.SupplierPricingProfile{UpstreamGroup: "Claude kiro渠道", USDPerCredit: 0.006})
		require.NoError(t, err)
		sonnet := find(claude, "claude-sonnet-5-5")
		require.Equal(t, 0.0012, *sonnet.CacheReadUSD)
		require.Equal(t, 0.015, *sonnet.CacheWriteUSD)
		gpt, err := normalizeSupplierCatalogue(body, "bblabu", model.SupplierPricingProfile{UpstreamGroup: "OpenAI", USDPerCredit: 0.006})
		require.NoError(t, err)
		cost, _, err := billingexpr.RunExprWithRequest(find(gpt, "gpt-6.1-sol").Expression, billingexpr.TokenParams{C: 1, Len: 272001}, billingexpr.RequestInput{})
		require.NoError(t, err)
		require.InDelta(t, 0.09, cost, 1e-12)
	})
	t.Run("relay-pinned-source", func(t *testing.T) {
		tariffs, err := normalizeSupplierCatalogue(load("supplier-relay-gpt.json"), "relay-fast", model.SupplierPricingProfile{UpstreamGroup: "gpt:gpt-special-2", USDPerCredit: 1})
		require.NoError(t, err)
		tariff := find(tariffs, "gpt-6.1-sol")
		cost, _, err := billingexpr.RunExprWithRequest(tariff.Expression, billingexpr.TokenParams{P: 1, C: 1, CR: 1, CC: 1, Len: 272001}, billingexpr.RequestInput{})
		require.NoError(t, err)
		require.InDelta(t, 0.06+0.225+0.003+0.075, cost, 1e-12)
	})
}

func TestSupplierPricingRejectsUnsupportedModifiersAndConflictingAliases(t *testing.T) {
	base := `{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"m","quota_type":0,"model_ratio":1,"completion_ratio":5,"enable_groups":["default"],"group_ratio":{"default":1}}]}`
	for _, adapter := range []string{"new-api", "openlux", "bblabu", "relay-fast"} {
		for _, field := range []string{"model_group_ratio", "peak_pricing"} {
			var document map[string]any
			require.NoError(t, common.Unmarshal([]byte(base), &document))
			document[field] = map[string]any{"enabled": true, "factor": 2}
			body, err := common.Marshal(document)
			require.NoError(t, err)
			_, err = normalizeSupplierCatalogue(body, adapter, model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
			require.ErrorContains(t, err, field)
		}
	}
	for _, modifier := range []struct {
		field string
		value any
	}{
		{"cache_creation_12h_ratio", 2.5}, {"image_cache_ratio", 0.64},
		{"video_resolution_ratios", map[string]float64{"720p": 10}},
		{"tiers", []any{map[string]any{"price": 2}}},
		{"image_billing", map[string]any{"high": 2}},
		{"model_price_min", 1},
	} {
		var document map[string]any
		require.NoError(t, common.Unmarshal([]byte(base), &document))
		item := document["data"].([]any)[0].(map[string]any)
		delete(item, "group_ratio")
		item[modifier.field] = modifier.value
		body, err := common.Marshal(document)
		require.NoError(t, err)
		_, err = normalizeSupplierCatalogue(body, "openlux", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
		require.ErrorContains(t, err, modifier.field)
	}
	for _, test := range []struct {
		aliases   string
		wantError bool
	}{
		{`"create_cache_ratio":1.25,"cache_creation_5m_ratio":1.25,"create_cache_ratio_1h":2,"cache_creation_1h_ratio":2`, false},
		{`"create_cache_ratio":1.25,"cache_creation_5m_ratio":1.5`, true},
		{`"create_cache_ratio_1h":2,"cache_creation_1h_ratio":3`, true},
	} {
		body := strings.Replace(base, `"group_ratio":{"default":1}}]}`, test.aliases+`}]}`, 1)
		_, err := normalizeSupplierCatalogue([]byte(body), "openlux", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
		if test.wantError {
			require.ErrorContains(t, err, "conflicting")
		} else {
			require.NoError(t, err)
		}
	}
}

func TestSupplierPricingStepsPreserveExplicitLanesAndReasoningAcrossBranches(t *testing.T) {
	body := []byte(`{"success":true,"group_ratio":{"default":1},"usable_group":{"default":"D"},"data":[{"model_name":"steps","quota_type":0,"model_ratio":1,"completion_ratio":5,"cache_creation_5m_ratio":1.25,"cache_creation_1h_ratio":3,"enable_groups":["default"],"step_ratios":[{"step_size":100,"completion_step_size":20,"prompt_step_ratio":1,"completion_step_ratio":1,"cache_step_ratio":1,"completion_thinking_step_ratio":0},{"step_size":1000,"completion_step_size":-1,"prompt_step_ratio":2,"completion_step_ratio":2,"cache_step_ratio":2,"completion_thinking_step_ratio":3}]}]}`)
	tariffs, err := normalizeSupplierCatalogue(body, "openlux", model.SupplierPricingProfile{UpstreamGroup: "default", USDPerCredit: 1})
	require.NoError(t, err)
	require.Len(t, tariffs, 1)
	expression := tariffs[0].Expression
	variables := billingexpr.UsedVars(expression)
	require.False(t, variables["ai"])
	require.False(t, variables["ao"])
	require.False(t, variables["cr"])
	for _, test := range []struct {
		tokens billingexpr.TokenParams
		cost   float64
	}{
		{billingexpr.TokenParams{CC1h: 1, Len: 100}, 6},
		{billingexpr.TokenParams{CC1h: 1, Len: 101}, 12},
		{billingexpr.TokenParams{RT: 1, Len: 100}, 10},
		{billingexpr.TokenParams{RT: 1, Len: 101}, 30},
	} {
		cost, _, err := billingexpr.RunExprWithRequest(expression, test.tokens, billingexpr.RequestInput{})
		require.NoError(t, err)
		require.Equal(t, test.cost, cost)
	}
}

func TestSupplierPricingPreservedPublicCatalogues(t *testing.T) {
	directory := os.Getenv("SUPPLIER_TEST_CATALOGUE_DIR")
	if directory == "" {
		t.Skip("SUPPLIER_TEST_CATALOGUE_DIR is not configured")
	}
	for _, test := range []struct {
		file, adapter, group string
		conversion           float64
		models               int
		rejection            string
	}{
		{"openlux-research-pricing.json", "openlux", "Kiro-Claude-1", 1, 12, ""},
		{"openlux-research-pricing.json", "openlux", "Claude-Code-1", 1, 14, ""},
		{"openlux-research-pricing.json", "openlux", "Codex-Gpt-1", 1, 0, "cache_creation_12h_ratio"},
		{"openlux-research-pricing.json", "openlux", "Alibaba-video-1", 1, 0, "video_resolution_ratios"},
		{"api-bblabu-cn-pricing.json", "bblabu", "Claude kiro渠道", 0.006, 8, ""},
		{"api-bblabu-cn-pricing.json", "bblabu", "OpenAI", 0.006, 10, ""},
		{"relayfast-research-pricing.json", "relay-fast", "gpt:gpt-special-2", 1, 6, ""},
		{"packy-public.json", "new-api", "default", 1, 0, "model_group_ratio"},
	} {
		t.Run(test.adapter+"/"+test.group, func(t *testing.T) {
			body, err := os.ReadFile(filepath.Join(directory, test.file))
			require.NoError(t, err)
			tariffs, err := normalizeSupplierCatalogue(body, test.adapter, model.SupplierPricingProfile{UpstreamGroup: test.group, USDPerCredit: test.conversion})
			if test.rejection != "" {
				require.ErrorContains(t, err, test.rejection)
				t.Log("rejected:", err)
				return
			}
			require.NoError(t, err)
			if test.models > 0 {
				require.Len(t, tariffs, test.models)
			}
			t.Logf("normalized %d complete model contracts", len(tariffs))
		})
	}
}

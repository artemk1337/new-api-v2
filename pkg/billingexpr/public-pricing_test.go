package billingexpr

import (
	"math"
	"reflect"
	"testing"
)

func TestPublicPricingTiersAllLanes(t *testing.T) {
	expression := `v1:tier("supplier-private-group", p*1+c*2+cr*3+cc*4+cc1h*5+img*6+img_o*7+ai*8+ao*9+rt*10)`
	tiers, ok := PublicPricingTiers(expression, 1.3)
	if !ok || len(tiers) != 1 {
		t.Fatalf("missing static pricing: %#v, %v", tiers, ok)
	}
	tier := tiers[0]
	values := []*float64{tier.InputUSD, tier.OutputUSD, tier.CacheReadUSD, tier.CacheWriteUSD, tier.CacheWrite1hUSD, tier.ImageUSD, tier.ImageOutputUSD, tier.AudioInputUSD, tier.AudioOutputUSD, tier.ReasoningOutputUSD}
	for i, value := range values {
		want := float64(i+1) * 1.3
		if value == nil || math.Abs(*value-want) > 1e-12 {
			t.Fatalf("lane %d = %v, want %v", i, value, want)
		}
	}
	if tier.MinInputTokens != nil || tier.MaxInputTokens != nil {
		t.Fatal("flat pricing acquired a length bound")
	}
	// Only numeric values can reach the public response; private tier names and
	// expression fragments have no representable field in the contract.
	typ := reflect.TypeOf(tier)
	for i := range typ.NumField() {
		field := typ.Field(i)
		if field.Type.Kind() != reflect.Pointer || (field.Type.Elem().Kind() != reflect.Float64 && field.Type.Elem().Kind() != reflect.Int64) {
			t.Fatalf("non-numeric public field: %s", field.Name)
		}
	}
}

func TestPublicPricingTiersLengthBoundaries(t *testing.T) {
	cases := []struct {
		condition              string
		cutoff                 int64
		lowerPrice, upperPrice float64
	}{
		{"len <= 200000", 200000, 3, 6},
		{"len < 200000", 199999, 3, 6},
		{"len > 200000", 200000, 6, 3},
		{"len >= 200000", 199999, 6, 3},
		{"200000 >= len", 200000, 3, 6},
		{"200000 > len", 199999, 3, 6},
		{"200000 < len", 200000, 6, 3},
		{"200000 <= len", 199999, 6, 3},
	}
	for _, tc := range cases {
		t.Run(tc.condition, func(t *testing.T) {
			expression := tc.condition + ` ? tier("supplier-a", p*3+c*15+cr*0.3) : tier("supplier-b", p*6+c*22.5)`
			tiers, ok := PublicPricingTiers(expression, 2)
			if !ok || len(tiers) != 2 {
				t.Fatalf("missing tiers: %#v %v", tiers, ok)
			}
			if tiers[0].MinInputTokens != nil || tiers[0].MaxInputTokens == nil || *tiers[0].MaxInputTokens != tc.cutoff || tiers[1].MinInputTokens == nil || *tiers[1].MinInputTokens != tc.cutoff+1 || tiers[1].MaxInputTokens != nil {
				t.Fatalf("incorrect inclusive boundary: %#v", tiers)
			}
			if *tiers[0].InputUSD != 2*tc.lowerPrice || *tiers[1].InputUSD != 2*tc.upperPrice {
				t.Fatalf("reversed pricing: %#v", tiers)
			}
			// Normalization detects cr across the whole expression, so the branch
			// without cr prices those cache tokens at zero, not at the p rate.
			if tiers[0].CacheReadUSD == nil || tiers[1].CacheReadUSD == nil {
				t.Fatal("lost an explicitly normalized cache lane")
			}
			expectedLowerCache, expectedUpperCache := 0.6, 0.0
			if tc.lowerPrice == 6 {
				expectedLowerCache, expectedUpperCache = 0, 0.6
			}
			if *tiers[0].CacheReadUSD != expectedLowerCache || *tiers[1].CacheReadUSD != expectedUpperCache {
				t.Fatal("branch without cache pricing must retain an explicit zero rate")
			}
		})
	}
}

func TestPublicPricingTiersNestedAndZeroRates(t *testing.T) {
	expression := `len <= 100 ? tier("a", p*0+c*0+cr*0) : len <= 200 ? tier("b", (p+c)*2) : tier("c", 3*p+c/2)`
	tiers, ok := PublicPricingTiers(expression, 1)
	if !ok || len(tiers) != 3 {
		t.Fatalf("missing nested tiers: %#v %v", tiers, ok)
	}
	if tiers[0].InputUSD == nil || *tiers[0].InputUSD != 0 || tiers[0].OutputUSD == nil || *tiers[0].OutputUSD != 0 || tiers[0].CacheReadUSD == nil || *tiers[0].CacheReadUSD != 0 {
		t.Fatal("explicit zero rates were lost")
	}
	if *tiers[1].MinInputTokens != 101 || *tiers[1].MaxInputTokens != 200 || *tiers[1].InputUSD != 2 || *tiers[2].MinInputTokens != 201 || *tiers[2].InputUSD != 3 || *tiers[2].OutputUSD != 0.5 {
		t.Fatalf("incorrect nested rates: %#v", tiers)
	}
	flat, ok := PublicPricingTiers(`p*0+c*1`, 0)
	if !ok || *flat[0].InputUSD != 0 || *flat[0].OutputUSD != 0 || flat[0].CacheReadUSD != nil {
		t.Fatal("zero multiplier or implicit cache fallback changed")
	}
}

func TestPublicPricingTiersRejectUnsupportedContracts(t *testing.T) {
	expressions := []string{
		`p*c`, `p*p*0`, `p/(c+1)`, `max(p*2,c*3)`, `p*2+1`, `p*-1`,
		`p*len`, `p < 100 ? p*2 : p*4`, `len == 100 ? p*2 : p*4`,
		`len <= 100.5 ? p*2 : p*4`,
		`hour("UTC") < 10 ? tier("a",p*2) : tier("b",p*4)`,
		`param("service_tier") == "fast" ? p*2 : p*4`,
		`header("x-tier") == "fast" ? p*2 : p*4`,
		`tier("base",p*2)|||when(header("x") has "fast") * 6`,
		`tier(header("x"),p*2)`, `p*1e308*10`, `not valid`,
	}
	for _, expression := range expressions {
		t.Run(expression, func(t *testing.T) {
			tiers, ok := PublicPricingTiers(expression, 1)
			if ok || tiers != nil {
				t.Fatalf("unsupported contract exposed rates: %#v", tiers)
			}
		})
	}
	for _, multiplier := range []float64{-1, math.Inf(1), math.NaN()} {
		if tiers, ok := PublicPricingTiers(`p*2+c*3`, multiplier); ok || tiers != nil {
			t.Fatalf("invalid multiplier accepted: %v", multiplier)
		}
	}
}

func TestPublicPricingTiersMatchesCompiledScalarArithmetic(t *testing.T) {
	expression := `p*(1000000000000*1000000000000)+c*(1/2)`
	tiers, ok := PublicPricingTiers(expression, 1.3)
	if !ok || len(tiers) != 1 {
		t.Fatalf("missing scalar pricing: %#v %v", tiers, ok)
	}
	cost, _, err := RunExpr(expression, TokenParams{P: 1, C: 1})
	if err != nil {
		t.Fatal(err)
	}
	if *tiers[0].InputUSD+*tiers[0].OutputUSD != cost*1.3 {
		t.Fatal("public coefficients differ from compiled billing arithmetic")
	}
}

func TestPublicPricingTiersScaledConditionalWrappers(t *testing.T) {
	for _, expression := range []string{
		`(len <= 200000 ? tier("source-short",p*3+c*15+cr*0.3) : tier("source-long",p*6+c*22.5+cr*0.6))*0.25`,
		`0.25*(len <= 200000 ? tier("source-short",p*3+c*15+cr*0.3) : tier("source-long",p*6+c*22.5+cr*0.6))`,
		`tier("supplier-private",(len <= 200000 ? tier("source-short",p*3+c*15+cr*0.3) : tier("source-long",p*6+c*22.5+cr*0.6))/4)`,
		`tier("supplier-private",(len <= 200000 ? tier("source-short",p*3+c*15+cr*0.3)*0.5 : tier("source-long",p*6+c*22.5+cr*0.6)/2)*0.5)`,
	} {
		t.Run(expression, func(t *testing.T) {
			tiers, ok := PublicPricingTiers(expression, 1.3)
			if !ok || len(tiers) != 2 {
				t.Fatalf("missing scaled tiers: %#v %v", tiers, ok)
			}
			if *tiers[0].MaxInputTokens != 200000 || *tiers[1].MinInputTokens != 200001 {
				t.Fatal("scaling changed tier boundaries")
			}
			for i, length := range []float64{200000, 200001} {
				cost, _, err := RunExpr(expression, TokenParams{P: 3, C: 5, CR: 7, Len: length})
				if err != nil {
					t.Fatal(err)
				}
				result := 3**tiers[i].InputUSD + 5**tiers[i].OutputUSD + 7**tiers[i].CacheReadUSD
				if math.Abs(result-cost*1.3) > 1e-10 {
					t.Fatalf("scaled public rate %v differs from billing cost %v", result, cost*1.3)
				}
			}
		})
	}
	for _, expression := range []string{
		`(len <= 100 ? tier("a",p) : tier("b",p*2))*c`,
		`(hour("UTC") < 10 ? tier("a",p) : tier("b",p*2))*0`,
		`tier("wrap",(len <= 100 ? tier("a",p) : tier("b",p*2)))/0`,
	} {
		if tiers, ok := PublicPricingTiers(expression, 1); ok || tiers != nil {
			t.Fatalf("unsafe scaled expression exposed: %s", expression)
		}
	}
}

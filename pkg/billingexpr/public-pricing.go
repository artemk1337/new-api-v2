package billingexpr

import (
	"math"
	"sort"
	"strings"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// PublicPricingTier contains final USD prices per million tokens, without source
// expressions or supplier labels. Bounds are inclusive; omitted bounds mean
// zero/no upper limit. Omitted optional lanes are included in the input/output
// rate rather than separately priced.
type PublicPricingTier struct {
	MinInputTokens     *int64   `json:"min_input_tokens,omitempty"`
	MaxInputTokens     *int64   `json:"max_input_tokens,omitempty"`
	InputUSD           *float64 `json:"input_usd,omitempty"`
	OutputUSD          *float64 `json:"output_usd,omitempty"`
	CacheReadUSD       *float64 `json:"cache_read_usd,omitempty"`
	CacheWriteUSD      *float64 `json:"cache_write_usd,omitempty"`
	CacheWrite1hUSD    *float64 `json:"cache_write_1h_usd,omitempty"`
	ImageUSD           *float64 `json:"image_usd,omitempty"`
	ImageOutputUSD     *float64 `json:"image_output_usd,omitempty"`
	AudioInputUSD      *float64 `json:"audio_input_usd,omitempty"`
	AudioOutputUSD     *float64 `json:"audio_output_usd,omitempty"`
	ReasoningOutputUSD *float64 `json:"reasoning_output_usd,omitempty"`
}

var publicPricingVariables = [...]string{"p", "c", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "rt"}

type publicPricingLinear struct {
	rates    [10]float64
	used     uint16
	constant float64
}

type publicPricingBranch struct {
	linear   publicPricingLinear
	min, max int64
}

// PublicPricingTiers extracts only static linear token prices and input-length
// tiers. Unsupported or request-dependent contracts have no public breakdown.
func PublicPricingTiers(expression string, multiplier float64) ([]PublicPricingTier, bool) {
	if !publicPricingFinite(multiplier) || multiplier < 0 || strings.Contains(expression, "|||") {
		return nil, false
	}
	program, err := CompileFromCache(expression)
	if err != nil {
		return nil, false
	}
	// Inspect the original tree so constant folding cannot conceal unsupported
	// request dependencies or nonlinear terms.
	_, body := ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return nil, false
	}
	if _, ok := publicPricingBranches(tree.Node, 0, math.MaxInt64, 0); !ok {
		return nil, false
	}
	// Use folded constants for the published values, matching expr arithmetic
	// (including integer overflow) rather than reinterpreting scalar operations.
	branches, ok := publicPricingBranches(program.Node(), 0, math.MaxInt64, 0)
	if !ok || len(branches) == 0 {
		return nil, false
	}
	sort.Slice(branches, func(i, j int) bool { return branches[i].min < branches[j].min })
	var used uint16 = 3 // Input and output prices remain explicit, including zero.
	variables := extractUsedVars(program)
	for i, name := range publicPricingVariables {
		if variables[name] {
			used |= 1 << i
		}
	}

	tiers := make([]PublicPricingTier, 0, len(branches))
	for _, branch := range branches {
		if branch.linear.constant != 0 {
			return nil, false // A fixed charge cannot be represented as a token rate.
		}
		tier := PublicPricingTier{}
		if branch.min > 0 {
			value := branch.min
			tier.MinInputTokens = &value
		}
		if branch.max < math.MaxInt64 {
			value := branch.max
			tier.MaxInputTokens = &value
		}
		fields := [...]**float64{&tier.InputUSD, &tier.OutputUSD, &tier.CacheReadUSD, &tier.CacheWriteUSD, &tier.CacheWrite1hUSD, &tier.ImageUSD, &tier.ImageOutputUSD, &tier.AudioInputUSD, &tier.AudioOutputUSD, &tier.ReasoningOutputUSD}
		for i, coefficient := range branch.linear.rates {
			rate := coefficient * multiplier
			if !publicPricingFinite(coefficient) || coefficient < 0 || !publicPricingFinite(rate) {
				return nil, false
			}
			if used&(1<<i) != 0 {
				*fields[i] = &rate
			}
		}
		tiers = append(tiers, tier)
	}
	return tiers, true
}

func publicPricingFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func publicPricingBranches(node ast.Node, minBound, maxBound int64, depth int) ([]publicPricingBranch, bool) {
	if depth > 32 {
		return nil, false
	}
	if minBound > maxBound {
		return nil, true
	}
	if call, ok := node.(*ast.CallNode); ok {
		value, valid := publicPricingTierValue(call)
		if !valid {
			return nil, false
		}
		return publicPricingBranches(value, minBound, maxBound, depth+1)
	}
	if binary, ok := node.(*ast.BinaryNode); ok {
		if binary.Operator == "*" {
			if scalar, valid := publicPricingLinearFor(binary.Left); valid && scalar.used == 0 {
				return publicPricingScaledBranches(binary.Right, scalar.constant, minBound, maxBound, depth)
			}
		}
		if binary.Operator == "*" || binary.Operator == "/" {
			if scalar, valid := publicPricingLinearFor(binary.Right); valid && scalar.used == 0 {
				factor := scalar.constant
				if binary.Operator == "/" {
					if factor == 0 {
						return nil, false
					}
					factor = 1 / factor
				}
				return publicPricingScaledBranches(binary.Left, factor, minBound, maxBound, depth)
			}
		}
	}
	if conditional, ok := node.(*ast.ConditionalNode); ok {
		cutoff, trueIsLower, valid := publicPricingCutoff(conditional.Cond)
		if !valid {
			return nil, false
		}
		lower, upper := conditional.Exp1, conditional.Exp2
		if !trueIsLower {
			lower, upper = upper, lower
		}
		left, valid := publicPricingBranches(lower, minBound, min(maxBound, cutoff), depth+1)
		if !valid {
			return nil, false
		}
		right, valid := publicPricingBranches(upper, max(minBound, cutoff+1), maxBound, depth+1)
		if !valid || len(left)+len(right) > 64 {
			return nil, false
		}
		return append(left, right...), true
	}
	linear, ok := publicPricingLinearFor(node)
	if !ok {
		return nil, false
	}
	return []publicPricingBranch{{linear: linear, min: minBound, max: maxBound}}, true
}

func publicPricingScaledBranches(node ast.Node, factor float64, minBound, maxBound int64, depth int) ([]publicPricingBranch, bool) {
	branches, ok := publicPricingBranches(node, minBound, maxBound, depth+1)
	if !ok || !publicPricingFinite(factor) {
		return nil, false
	}
	for index := range branches {
		branches[index].linear = publicPricingScale(branches[index].linear, factor)
	}
	return branches, true
}

func publicPricingCutoff(node ast.Node) (int64, bool, bool) {
	comparison, ok := node.(*ast.BinaryNode)
	if !ok {
		return 0, false, false
	}
	operator := comparison.Operator
	variable, isIdentifier := comparison.Left.(*ast.IdentifierNode)
	literal := comparison.Right
	if !isIdentifier || variable.Value != "len" {
		variable, isIdentifier = comparison.Right.(*ast.IdentifierNode)
		literal = comparison.Left
		switch operator {
		case "<":
			operator = ">"
		case "<=":
			operator = ">="
		case ">":
			operator = "<"
		case ">=":
			operator = "<="
		}
	}
	if !isIdentifier || variable.Value != "len" {
		return 0, false, false
	}
	value, valid := publicPricingLinearFor(literal)
	number := value.constant
	if !valid || value.used != 0 || !publicPricingFinite(number) || math.Trunc(number) != number || math.Abs(number) > 1<<53-1 {
		return 0, false, false
	}
	cutoff := int64(number)
	switch operator {
	case "<=":
		return cutoff, true, true
	case "<":
		return cutoff - 1, true, true
	case ">":
		return cutoff, false, true
	case ">=":
		return cutoff - 1, false, true
	default:
		return 0, false, false
	}
}

func publicPricingTierValue(node *ast.CallNode) (ast.Node, bool) {
	callee, ok := node.Callee.(*ast.IdentifierNode)
	if !ok || callee.Value != "tier" || len(node.Arguments) != 2 {
		return nil, false
	}
	if _, ok := node.Arguments[0].(*ast.StringNode); !ok {
		return nil, false
	}
	return node.Arguments[1], true
}

func publicPricingLinearFor(node ast.Node) (publicPricingLinear, bool) {
	switch node := node.(type) {
	case *ast.IntegerNode:
		return publicPricingLinear{constant: float64(node.Value)}, true
	case *ast.FloatNode:
		return publicPricingLinear{constant: node.Value}, publicPricingFinite(node.Value)
	case *ast.IdentifierNode:
		for i, name := range publicPricingVariables {
			if node.Value == name {
				form := publicPricingLinear{used: 1 << i}
				form.rates[i] = 1
				return form, true
			}
		}
	case *ast.UnaryNode:
		form, ok := publicPricingLinearFor(node.Node)
		if !ok {
			return form, false
		}
		switch node.Operator {
		case "+":
			return form, true
		case "-":
			return publicPricingScale(form, -1), true
		}
	case *ast.CallNode:
		value, ok := publicPricingTierValue(node)
		if ok {
			return publicPricingLinearFor(value)
		}
	case *ast.BinaryNode:
		left, leftOK := publicPricingLinearFor(node.Left)
		right, rightOK := publicPricingLinearFor(node.Right)
		if !leftOK || !rightOK {
			return publicPricingLinear{}, false
		}
		switch node.Operator {
		case "+", "-":
			sign := 1.0
			if node.Operator == "-" {
				sign = -1
			}
			left.used |= right.used
			left.constant += sign * right.constant
			for i := range left.rates {
				left.rates[i] += sign * right.rates[i]
			}
			return left, publicPricingFinite(left.constant)
		case "*":
			if left.used == 0 {
				return publicPricingScale(right, left.constant), true
			}
			if right.used == 0 {
				return publicPricingScale(left, right.constant), true
			}
		case "/":
			if right.used == 0 && right.constant != 0 {
				return publicPricingScale(left, 1/right.constant), true
			}
		}
	}
	return publicPricingLinear{}, false
}

func publicPricingScale(form publicPricingLinear, factor float64) publicPricingLinear {
	form.constant *= factor
	for i := range form.rates {
		form.rates[i] *= factor
	}
	return form
}

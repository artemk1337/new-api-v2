package billingexpr

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/vm"
)

const maxCacheSize = 256

// DefaultExprVersion is used when an expression string has no version prefix.
const DefaultExprVersion = 1

// ParseExprVersion extracts the version tag and body from an expression string.
// Format: "v1:tier(...)" → version=1, body="tier(...)".
// No prefix defaults to DefaultExprVersion.
func ParseExprVersion(exprStr string) (version int, body string) {
	if strings.HasPrefix(exprStr, "v1:") {
		return 1, exprStr[3:]
	}
	return DefaultExprVersion, exprStr
}

type cachedEntry struct {
	prog     *vm.Program
	usedVars map[string]bool
	version  int
}

var (
	cacheMu sync.RWMutex
	cache   = make(map[string]*cachedEntry, 64)
)

// compileEnvPrototypeV1 is the v1 type-checking prototype used at compile time.
var compileEnvPrototypeV1 = map[string]interface{}{
	"p":       float64(0),
	"c":       float64(0),
	"len":     float64(0),
	"cr":      float64(0),
	"cc":      float64(0),
	"cc1h":    float64(0),
	"img":     float64(0),
	"img_o":   float64(0),
	"ai":      float64(0),
	"ao":      float64(0),
	"rt":      float64(0),
	"tier":    func(string, float64) float64 { return 0 },
	"header":  func(string) string { return "" },
	"param":   func(string) interface{} { return nil },
	"has":     func(interface{}, string) bool { return false },
	"hour":    func(string) int { return 0 },
	"minute":  func(string) int { return 0 },
	"weekday": func(string) int { return 0 },
	"month":   func(string) int { return 0 },
	"day":     func(string) int { return 0 },
	"max":     math.Max,
	"min":     math.Min,
	"abs":     math.Abs,
	"ceil":    math.Ceil,
	"floor":   math.Floor,
}

func getCompileEnv(version int) map[string]interface{} {
	switch version {
	default:
		return compileEnvPrototypeV1
	}
}

// CompileFromCache compiles an expression string, using a cached program when
// available. The cache is keyed by the SHA-256 hex digest of the expression.
func CompileFromCache(exprStr string) (*vm.Program, error) {
	return compileFromCacheByHash(exprStr, ExprHashString(exprStr))
}

// CompileFromCacheByHash is like CompileFromCache but accepts a pre-computed
// hash, useful when the caller already has the BillingSnapshot.ExprHash.
func CompileFromCacheByHash(exprStr, hash string) (*vm.Program, error) {
	return compileFromCacheByHash(exprStr, hash)
}

func compileFromCacheByHash(exprStr, hash string) (*vm.Program, error) {
	cacheMu.RLock()
	if entry, ok := cache[hash]; ok {
		cacheMu.RUnlock()
		return entry.prog, nil
	}
	cacheMu.RUnlock()

	version, body := ParseExprVersion(exprStr)
	prog, err := expr.Compile(body, expr.Env(getCompileEnv(version)), expr.AsFloat64())
	if err != nil {
		return nil, fmt.Errorf("expr compile error: %w", err)
	}

	vars := extractUsedVars(prog)

	cacheMu.Lock()
	if len(cache) >= maxCacheSize {
		cache = make(map[string]*cachedEntry, 64)
	}
	cache[hash] = &cachedEntry{prog: prog, usedVars: vars, version: version}
	cacheMu.Unlock()

	return prog, nil
}

// ExprVersion returns the version of a cached expression. Returns DefaultExprVersion
// if the expression hasn't been compiled yet or is empty.
func ExprVersion(exprStr string) int {
	if exprStr == "" {
		return DefaultExprVersion
	}
	hash := ExprHashString(exprStr)
	cacheMu.RLock()
	if entry, ok := cache[hash]; ok {
		cacheMu.RUnlock()
		return entry.version
	}
	cacheMu.RUnlock()
	v, _ := ParseExprVersion(exprStr)
	return v
}

func extractUsedVars(prog *vm.Program) map[string]bool {
	vars := make(map[string]bool)
	node := prog.Node()
	ast.Find(node, func(n ast.Node) bool {
		if id, ok := n.(*ast.IdentifierNode); ok {
			vars[id.Value] = true
		}
		return false
	})
	return vars
}

// UsedVars returns the set of identifier names referenced by an expression.
// The result is cached alongside the compiled program. Returns nil for empty input.
func UsedVars(exprStr string) map[string]bool {
	if exprStr == "" {
		return nil
	}
	hash := ExprHashString(exprStr)
	cacheMu.RLock()
	if entry, ok := cache[hash]; ok {
		cacheMu.RUnlock()
		return entry.usedVars
	}
	cacheMu.RUnlock()

	// Compile (and cache) to populate usedVars
	if _, err := compileFromCacheByHash(exprStr, hash); err != nil {
		return nil
	}
	cacheMu.RLock()
	entry, ok := cache[hash]
	cacheMu.RUnlock()
	if ok {
		return entry.usedVars
	}
	return nil
}

type reasoningSplitForm struct {
	safe      bool
	invariant bool
	known     bool
	cCoeff    float64
	rtCoeff   float64
	literal   *float64
}

func reasoningConstant() reasoningSplitForm {
	return reasoningSplitForm{safe: true, invariant: true, known: true}
}

func reasoningLiteral(value float64) reasoningSplitForm {
	form := reasoningConstant()
	form.literal = &value
	return form
}

func reasoningInvalid() reasoningSplitForm {
	return reasoningSplitForm{}
}

func reasoningAdd(left, right reasoningSplitForm, sign float64) reasoningSplitForm {
	if !left.safe || !right.safe {
		return reasoningInvalid()
	}
	form := reasoningSplitForm{safe: true, known: left.known && right.known}
	if form.known {
		form.cCoeff = left.cCoeff + sign*right.cCoeff
		form.rtCoeff = left.rtCoeff + sign*right.rtCoeff
		form.invariant = form.cCoeff == form.rtCoeff
	} else {
		form.invariant = left.invariant && right.invariant
	}
	return form
}

func reasoningScale(form reasoningSplitForm, factor float64) reasoningSplitForm {
	if !form.safe {
		return reasoningInvalid()
	}
	form.cCoeff *= factor
	form.rtCoeff *= factor
	if form.literal != nil {
		value := *form.literal * factor
		form.literal = &value
	}
	return form
}

func reasoningSplitFormFor(node ast.Node) reasoningSplitForm {
	switch node := node.(type) {
	case *ast.IdentifierNode:
		switch node.Value {
		case "c":
			return reasoningSplitForm{safe: true, known: true, cCoeff: 1}
		case "rt":
			return reasoningSplitForm{safe: true, known: true, rtCoeff: 1}
		default:
			return reasoningConstant()
		}
	case *ast.IntegerNode:
		return reasoningLiteral(float64(node.Value))
	case *ast.FloatNode:
		return reasoningLiteral(node.Value)
	case *ast.NilNode, *ast.BoolNode, *ast.StringNode, *ast.BytesNode, *ast.ConstantNode:
		return reasoningConstant()
	case *ast.UnaryNode:
		form := reasoningSplitFormFor(node.Node)
		if node.Operator == "-" {
			return reasoningScale(form, -1)
		}
		if form.invariant {
			return reasoningConstant()
		}
		return reasoningInvalid()
	case *ast.BinaryNode:
		left := reasoningSplitFormFor(node.Left)
		right := reasoningSplitFormFor(node.Right)
		switch node.Operator {
		case "+":
			return reasoningAdd(left, right, 1)
		case "-":
			return reasoningAdd(left, right, -1)
		case "*":
			if left.literal != nil {
				return reasoningScale(right, *left.literal)
			}
			if right.literal != nil {
				return reasoningScale(left, *right.literal)
			}
		case "/":
			if right.literal != nil && *right.literal != 0 {
				return reasoningScale(left, 1 / *right.literal)
			}
		case "<", "<=", ">", ">=", "==", "!=":
			if left.safe && right.safe && left.invariant && right.invariant {
				return reasoningConstant()
			}
			return reasoningInvalid()
		default:
			if left.safe && right.safe && left.invariant && right.invariant {
				return reasoningConstant()
			}
			return reasoningInvalid()
		}
		if left.safe && right.safe && left.invariant && right.invariant {
			return reasoningConstant()
		}
		return reasoningInvalid()
	case *ast.ConditionalNode:
		cond := reasoningSplitFormFor(node.Cond)
		left := reasoningSplitFormFor(node.Exp1)
		right := reasoningSplitFormFor(node.Exp2)
		if !cond.safe || !cond.invariant || !left.safe || !right.safe {
			return reasoningInvalid()
		}
		form := reasoningSplitForm{safe: true, invariant: left.invariant && right.invariant}
		if left.known && right.known && left.cCoeff == right.cCoeff && left.rtCoeff == right.rtCoeff {
			form.known = true
			form.cCoeff = left.cCoeff
			form.rtCoeff = left.rtCoeff
			form.invariant = form.cCoeff == form.rtCoeff
		}
		return form
	case *ast.CallNode:
		if callee, ok := node.Callee.(*ast.IdentifierNode); ok && callee.Value == "tier" && len(node.Arguments) == 2 {
			return reasoningSplitFormFor(node.Arguments[1])
		}
		for _, argument := range node.Arguments {
			form := reasoningSplitFormFor(argument)
			if !form.safe || !form.invariant {
				return reasoningInvalid()
			}
		}
		return reasoningConstant()
	case *ast.BuiltinNode:
		for _, argument := range node.Arguments {
			form := reasoningSplitFormFor(argument)
			if !form.safe || !form.invariant {
				return reasoningInvalid()
			}
		}
		return reasoningConstant()
	case *ast.ChainNode:
		return reasoningSplitFormFor(node.Node)
	default:
		return reasoningInvalid()
	}
}

// ReasoningSplitIsSafe reports whether endpoint-only reasoning fallback is safe.
func ReasoningSplitIsSafe(exprStr string) bool {
	if !UsedVars(exprStr)["rt"] {
		return true
	}
	prog, err := CompileFromCache(exprStr)
	if err != nil {
		return false
	}
	return reasoningSplitFormFor(prog.Node()).safe
}

// InvalidateCache clears the compiled-expression cache.
// Called when billing rules are updated.
func InvalidateCache() {
	cacheMu.Lock()
	cache = make(map[string]*cachedEntry, 64)
	cacheMu.Unlock()
}

package billingexpr

import (
	"fmt"

	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
)

// ValidateSupplierExpression limits procurement prices to token/context inputs.
// Request overrides and clock changes must not change a frozen supplier tariff.
// Inspect the original AST so an optimizer cannot hide a forbidden branch.
func ValidateSupplierExpression(expression string) error {
	_, body := ParseExprVersion(expression)
	tree, err := parser.Parse(body)
	if err != nil {
		return err
	}
	var unsupported string
	ast.Find(tree.Node, func(node ast.Node) bool {
		switch item := node.(type) {
		case *ast.IdentifierNode:
			if !supplierExpressionIdentifier(item.Value) {
				unsupported = item.Value
			}
		case *ast.BuiltinNode:
			if !supplierExpressionFunction(item.Name) {
				unsupported = item.Name
			}
		}
		return unsupported != ""
	})
	if unsupported != "" {
		return fmt.Errorf("supplier pricing does not support %s", unsupported)
	}
	_, err = CompileFromCache(expression)
	return err
}

func supplierExpressionIdentifier(name string) bool {
	switch name {
	case "p", "c", "len", "cr", "cc", "cc1h", "img", "img_o", "ai", "ao", "rt":
		return true
	}
	return supplierExpressionFunction(name)
}

func supplierExpressionFunction(name string) bool {
	switch name {
	case "tier", "max", "min", "abs", "ceil", "floor":
		return true
	}
	return false
}

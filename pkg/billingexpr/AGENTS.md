# `pkg/billingexpr/` instructions

Parent: [../AGENTS.md](../AGENTS.md). Required design reference: [expr.md](expr.md).

Compiles and evaluates versioned billing expressions, normalizes token usage, tracks expression variables, and converts provider usage into billing inputs. `expr.md` is the source of truth for syntax, variable semantics, AST detection, rounding, request rules, and version compatibility.

Changes must preserve the same charge for documented input contracts, avoid double-counting separately priced token categories, and never lower billing when required reasoning usage is absent. Update deterministic contract tests for observable billing behavior.

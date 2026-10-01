# System settings model editors

Parent: [../AGENTS.md](../AGENTS.md).

Contains model-specific editors, including tiered billing expression UI. Keep visual and raw editing modes round-trippable: editing a model must not drop unknown or untouched expression data. Prompt helpers and editor UI should remain separate when they have distinct responsibilities.

The generated expression is stored and evaluated by `setting/billing_setting` and `pkg/billingexpr`; follow [`../../../../../../pkg/billingexpr/expr.md`](../../../../../../pkg/billingexpr/expr.md) before changing expression construction or semantics. Preserve all existing translation keys across locales.

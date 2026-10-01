# `setting/billing_setting/` instructions

Parent: [../AGENTS.md](../AGENTS.md). Before editing expression behavior, read [`../../pkg/billingexpr/expr.md`](../../pkg/billingexpr/expr.md).

Stores per-model billing mode and expression configuration, validates it before persistence, and exposes it to relay pre-consume and service settlement. Keep mode and expression changes consistent and invalidate pricing caches after updates.

Do not reinterpret expression syntax in this package; compilation, token normalization, quota conversion, and versioning belong to `pkg/billingexpr` and its documented contract.

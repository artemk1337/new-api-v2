# System settings feature

Parent: [../AGENTS.md](../AGENTS.md).

Owns administrator-facing runtime configuration editors. Integrations, model pricing, group ratios, and payment settings have separate subdomains; keep each editor's persistence contract and validation close to its form while using the shared admin API/query patterns.

Pricing editors must preserve the stored expression/configuration contract documented in [`../../../../../pkg/billingexpr/expr.md`](../../../../../pkg/billingexpr/expr.md). Do not silently rewrite expressions or option values when changing presentation. Payment settings must preserve configured method order and server-side validation. Model pricing editor guidance is in [models/AGENTS.md](models/AGENTS.md).

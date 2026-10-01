# `pkg/perf_metrics/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Collects, aggregates, flushes, and prices performance metrics. Preserve label dimensions, aggregation windows, and group/model pricing semantics because dashboard consumers and stored results depend on them.

Do not add high-cardinality labels without checking storage and query costs. Prefer deterministic contract tests for metric aggregation and pricing.

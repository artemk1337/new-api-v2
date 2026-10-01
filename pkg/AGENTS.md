# `pkg/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Contains small internal libraries with focused contracts. Keep dependencies below application layers (`router`, `controller`, `service`) and avoid importing those packages from here. Reusable integrations and domain algorithms should not depend on Gin handlers or global route state.

See [billingexpr/AGENTS.md](billingexpr/AGENTS.md), [cachex/AGENTS.md](cachex/AGENTS.md), [ionet/AGENTS.md](ionet/AGENTS.md), and [perf_metrics/AGENTS.md](perf_metrics/AGENTS.md).

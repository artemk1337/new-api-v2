# `relay/common/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Provides shared relay context and transformations, including `RelayInfo`, header/body overrides, usage data, and provider-independent helpers. Preserve context propagation and override precedence because provider adapters and billing consume these values.

`override.go` and `override_headers.go` split the request override behavior by responsibility. Keep header validation, pass-through filtering, and context storage consistent; never forward hop-by-hop or otherwise disallowed headers. Reuse shared conversion and request-body safeguards rather than duplicating them in adapters.

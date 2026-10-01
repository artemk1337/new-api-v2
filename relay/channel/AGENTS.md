# `relay/channel/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Each subpackage adapts one upstream provider or protocol to shared relay interfaces. Keep authentication, endpoint construction, request/response conversion, streaming, and provider error handling within the relevant adapter; shared behavior belongs in parent relay packages.

Preserve provider wire contracts and optional request-field presence (`*T` plus `omitempty` for optional scalars). Confirm streaming, tool, image/audio, and async-task capabilities from the provider contract before changing shared capability lists. Keep provider-specific behavior out of unrelated adapters.

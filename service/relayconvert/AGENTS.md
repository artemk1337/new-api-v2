# `service/relayconvert/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Converts between supported request/response relay formats. Keep conversion code protocol-focused and independent of route registration and persistence. Preserve optional-field presence, streaming event ordering, tool-call structure, and provider-specific usage semantics through every conversion path.

Check call sites in `relay/` and the target DTO definitions in `dto/` before changing mappings.

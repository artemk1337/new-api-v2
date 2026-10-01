# `types/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Holds shared domain and transport types such as relay formats, file sources, errors, and request metadata. Keep these types independent of HTTP handlers and database access; place wire-specific request/response DTOs in `dto/`. Changes to relay format and error types can cross many adapters, so inspect their consumers before editing.

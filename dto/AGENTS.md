# `dto/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Request and response shapes shared across handlers, relay adapters, and provider formats live here. Keep DTOs declarative and avoid embedding persistence or request-processing behavior. For optional scalar fields parsed from client JSON and forwarded upstream, use pointers with `omitempty` so explicit zero and false values survive serialization.

Inspect neighboring DTOs and their marshal/unmarshal call sites before changing a wire shape. JSON operations use `common` wrappers.

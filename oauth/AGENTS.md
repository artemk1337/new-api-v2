# `oauth/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Provider implementations are registered through `registry.go` and the blank import in `router/api-router.go`. `generic.go` and provider files implement the shared provider contract in `provider.go`/`types.go`.

Preserve state/redirect validation, callback URL compatibility, and provider-specific scopes. Register new implementations deliberately and keep secrets out of logs. OAuth routes and their rate limits are owned by `router/`.

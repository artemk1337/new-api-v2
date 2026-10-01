# `router/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

`router/main.go` applies global admission middleware and composes API, dashboard, relay, video, and web routes. `api-router.go` and `relay-router.go` register route groups and connect `controller` handlers with middleware.

Route order and middleware order are API/security contracts. Preserve wildcard precedence, authentication type, body limits, rate limits, distributor placement, streaming/SSE behavior, and CORS behavior when changing routes. `main.go` initializes resources and workers before calling `SetRouter`.

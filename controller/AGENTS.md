# `controller/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Gin handlers translate HTTP input into calls to `service` and `model`, then shape API responses. Validate transport input and permissions at the boundary; reusable business workflows belong in `service`. Keep this package's exported handler names stable because `router/` registers them directly.

Route-level authentication and limits are registered in `router/`; do not assume a handler is safe when mounted without its middleware. Preserve webhook signatures, response status/body contracts, and relay billing context. Large files are grouped by handler families in this flat package; new files should keep cohesive handlers together without changing their package API.

Useful entrypoints: `relay.go`, `user.go`, `channel.go`, `topup.go`, `pricing_sync_task.go`. Tests should cover externally observable handler behavior and billing/accounting contracts.

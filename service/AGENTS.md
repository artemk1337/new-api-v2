# `service/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Coordinates business use cases between controllers/middleware and models, including billing, payment provider callbacks, external clients, and background workers. Keep HTTP response formatting in `controller` and persistence details in `model`.

## Invariants

- Billing consumes a frozen `BillingSnapshot` when present, settles actual usage, and refunds unused pre-consume quota; preserve the legacy path only where existing callers still require it.
- Payment callbacks must validate provider identity/status and remain idempotent. Direct-crypto watchers observe chains read-only, stop cleanly, and delegate settlement/reconciliation to model transactions.
- System task handlers must finish task state and honor cancellation. Register handlers before starting the runner, which snapshots registered work at startup.
- Background loops need bounded lifecycle and must not leak across shutdown or tests.

`billing.go`, `tiered_settle.go`, `system_task.go`, and `direct_usdt_*_watcher.go` are useful references. Subareas have local instructions in [authz/AGENTS.md](authz/AGENTS.md), [passkey/AGENTS.md](passkey/AGENTS.md), and [relayconvert/AGENTS.md](relayconvert/AGENTS.md).

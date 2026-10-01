# Project instructions

## Architecture

New API is a Go AI API gateway with a React admin and user frontend. Startup and dependency wiring live in `main.go`; HTTP routes call `controller`, request policy is in `middleware`, use cases and workers are in `service`, persistence is in `model`, and provider protocol adapters are in `relay/channel`.

```text
main.go → router → middleware/controller → service/model
                                  controller → service → model
relay → relay/channel (provider adapters)
web/default (React frontend; calls the HTTP API)
```

## Global invariants

- Use Go 1.25.6 for Go work. Keep code direct and compact; follow nearby package patterns.
- Preserve SQLite, MySQL, and PostgreSQL compatibility. Schema changes must support rolling deploys and rollback; see [model/AGENTS.md](model/AGENTS.md).
- Business JSON encoding and decoding must use `common/json.go` wrappers (`common.Marshal`, `common.Unmarshal`, `common.DecodeJson`); do not call `encoding/json` operations directly.
- Optional scalar request fields forwarded upstream use pointers with `omitempty`, preserving explicit `0` and `false`.
- Tiered billing changes must first follow [`pkg/billingexpr/expr.md`](pkg/billingexpr/expr.md).
- A chain event must not both credit a direct-crypto invoice and create an unmatched reconciliation record. Preserve invoice idempotency and read-only watcher behavior; see [service/AGENTS.md](service/AGENTS.md) and [model/AGENTS.md](model/AGENTS.md).
- Production deployments use a ready registry image with an explicit tag. Before a runtime change, create and verify a fresh database backup; preserve data volumes and restart only the application service. Never build source on production.
- Do not remove or replace protected project identity references to **nеw-аρi** or **QuаntumΝоuѕ**.
- New tests must assert user-visible behavior, API contracts, or important invariants. Do not add or run tests unless requested.

## Local instructions

| Area | Instructions |
|---|---|
| Startup binaries | [cmd/AGENTS.md](cmd/AGENTS.md) |
| Shared utilities | [common/AGENTS.md](common/AGENTS.md) |
| Constants, DTOs, and domain types | [constant/AGENTS.md](constant/AGENTS.md), [dto/AGENTS.md](dto/AGENTS.md), [types/AGENTS.md](types/AGENTS.md) |
| HTTP routes and handlers | [router/AGENTS.md](router/AGENTS.md), [middleware/AGENTS.md](middleware/AGENTS.md), [controller/AGENTS.md](controller/AGENTS.md) |
| Persistence and business logic | [model/AGENTS.md](model/AGENTS.md), [service/AGENTS.md](service/AGENTS.md) |
| Relay and provider adapters | [relay/AGENTS.md](relay/AGENTS.md) |
| Runtime configuration | [setting/AGENTS.md](setting/AGENTS.md) |
| Internal libraries | [pkg/AGENTS.md](pkg/AGENTS.md) |
| Authentication integrations and translations | [oauth/AGENTS.md](oauth/AGENTS.md), [i18n/AGENTS.md](i18n/AGENTS.md) |
| Logging and desktop shell | [logger/AGENTS.md](logger/AGENTS.md), [electron/AGENTS.md](electron/AGENTS.md) |
| Frontend | [web/default/AGENTS.md](web/default/AGENTS.md) |
| Documentation and scripts | [docs/AGENTS.md](docs/AGENTS.md), [scripts/AGENTS.md](scripts/AGENTS.md) |

## Workflow

- Read the nearest `AGENTS.md` and relevant README before changing an area.
- Keep refactors behavior-preserving and within the existing package/API boundary unless the task requires otherwise.
- For frontend copy, update all six locales (`en`, `zh`, `fr`, `ja`, `ru`, `vi`). Run the relevant checks required by the local instructions; report checks that could not run.
- Before handoff, inspect `git diff`, run `git diff --check`, and state the verification boundary. Follow [web/default/AGENTS.md](web/default/AGENTS.md) for frontend checks.

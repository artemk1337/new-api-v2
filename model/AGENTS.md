# `model/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Owns GORM persistence, database initialization and migrations, transactions, durable options, and model-level caches. Application layers call these APIs; provider and HTTP behavior should remain outside this package.

## Invariants

- Support SQLite, MySQL, and PostgreSQL. Prefer GORM; dialect-specific SQL must use the shared dialect helpers in `main.go`.
- Use `common.UsingMainDatabase` and `common.UsingLogDatabase` for database-specific branches. Keep reserved-column quoting portable.
- Migrations must tolerate rolling deploys and rollback: old code must survive an expanded schema and new code must handle pre-migration state until startup migration runs.
- Preserve transaction boundaries, idempotency, and persisted snapshots for billing/payment records. Direct-crypto invoice settlement and reconciliation must be mutually exclusive for the same chain event.
- Keep runtime option/cache updates consistent with the persisted value; options and pricing have cross-process synchronization paths.

Large flat files may be split by cohesive domain into additional files in this package; preserve exported symbols and transaction semantics. Key references: `main.go`, `option.go`, `subscription.go`, `direct_crypto_payment.go`, `topup.go`. Tests should assert database compatibility, durable state, and accounting behavior.

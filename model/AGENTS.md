# `model/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Owns GORM persistence, database initialization and migrations, transactions, durable options, and model-level caches. Application layers call these APIs; provider and HTTP behavior should remain outside this package.

## Invariants

- Support SQLite, MySQL, and PostgreSQL. Prefer GORM; dialect-specific SQL must use the shared dialect helpers in `main.go`.
- Use `common.UsingMainDatabase` and `common.UsingLogDatabase` for database-specific branches. Keep reserved-column quoting portable.
- Migrations must tolerate rolling deploys and rollback: old code must survive an expanded schema and new code must handle pre-migration state until startup migration runs.
- Preserve transaction boundaries, idempotency, and persisted snapshots for billing/payment records. Direct-crypto invoice settlement and reconciliation must be mutually exclusive for the same chain event.
- Registration IP is immutable after creation: store IPv4 as 4 bytes, IPv6 as 16 bytes, and unknown as NULL. Capture it only from the trusted HTTP client IP during self-registration; never backfill from login or administrator addresses.
- Keep runtime option/cache updates consistent with the persisted value; options and pricing have cross-process synchronization paths.
- Supplier procurement data is administrative. Persist group supplier/profile bindings by stable group ID outside public pricing options; serialize binding and channel membership changes through the supplier binding state row, and never route a bound group through a different supplier profile or the default tariff.
- Shared channel/group tags are independent administrative metadata. Preserve their registry and values; never infer supplier identity or tariff bindings from a tag.
- Persist each new async task's private transport fingerprint and minimal result destination before insertion. Active tasks validate supplier/profile and transport against the authoritative channel; a mismatch must not dispatch, settle, or refund automatically. Completed status stays local. Completed media uses the original destination and stored credential; a rotated live credential is eligible only at the same destination and exact supplier/profile.

Large flat files may be split by cohesive domain into additional files in this package; preserve exported symbols and transaction semantics. Key references: `main.go`, `option.go`, `subscription.go`, `direct_crypto_payment.go`, `topup.go`. Tests should assert database compatibility, durable state, and accounting behavior.

# Supplier pricing

This guide describes the supplier-pricing implementation in this source revision. Its presence in the repository does not prove that a server runs it. No production deployment or binding migration is implied by this document.

## Pricing domains

```text
Supplier → Pricing profile → Immutable procurement tariff revision (real USD)
                       ↑
               Pricing group + channel
                       ↓
              Retail rate = tariff × group multiplier
```

A supplier is the procurement identity, separate from a model provider/developer (`Vendor` in the existing API). The metadata catalogue labels Anthropic and OpenAI as model providers; it never turns those records into proxy suppliers. A profile selects an exact upstream group or routing source, its catalogue endpoint, and an explicit real-USD-per-credit conversion. Each active revision contains a complete model catalogue. Token lanes use USD per million tokens; fixed rates use USD per request or second. Supplier expressions support token/context tiers and pure arithmetic. Header, request-parameter and time-dependent supplier expressions are rejected: channel overrides could otherwise change paid parameters after reservation. Existing unbound expressions retain their legacy behavior.

An unbound group keeps the legacy global pricing path. A bound group and every channel serving it must have the same supplier/profile pair. Both channel fields are selected together or both absent; bindings use stable pricing-group IDs outside the public `PricingGroups` option. Group multiplier is a retail margin over the normalized procurement tariff, not a second provider-group coefficient.

Shared platform tags and private group notes remain independent administrative metadata. Tags can classify channels and groups, but never select a supplier, profile or tariff. Supplier bindings use their dedicated controls. Public pricing responses omit private notes and tags.

Missing tariffs, incompatible bindings, disabled suppliers/profiles, and invalidated profile configurations make the route unavailable. They never fall back to global/default prices or another supplier profile. Pre-consume and settlement use the request's frozen tariff revision. Changing the active catalogue does not reprice an already reserved request.

## Funding conversion and markup

Record the actual payment amount, usable credits received, fees, expiry, payment method, and applicable FX. A supplier displaying dollar-denominated credits does not establish one real USD per credit. Advertised discounts and a generic `price` field are insufficient evidence of the user's funding cost.

```text
USDPerCredit = actual cash USD paid / usable provider credits received
procurement USD rate = provider base rate × exact source factor × USDPerCredit
retail USD rate = procurement USD rate × pricing-group multiplier
```

For example, a separately verified quote of 1,000 credits for USD 6 yields `USDPerCredit = 0.006`. With upstream group ratio 1 and a base price of 2/10 credits per million input/output tokens, the procurement rate is USD 0.012/0.060. A bound group with multiplier **2** sells at USD 0.024/0.120. The example is not a default conversion or proof that a purchase occurred. If paid in USDT, record the actual conversion assumption and fees.

Do not assign group ratio `0.012` to this supplier-bound setup: that would apply funding conversion again. That legacy shortcut cannot generally represent different upstream cache rates, expressions, or fixed-price models.

OpenLux's normal native retail margin is **1.3**. Its upstream group factor is already part of the normalized tariff; do not apply that factor or the legacy two-decimal rounding rule again. If an explicitly reviewed migration must preserve an old rounded OpenLux retail coefficient and the tariff/funding bases are equivalent, the native multiplier is `old_saved_ratio / raw_source_group_ratio`. Nonproportional cache, tier, or unit differences cannot be preserved by one scalar. BBLABU retains its separate **2** margin; relay.fast has no inherited OpenLux/BBLABU markup.

## Catalogue publication

Root can publish a reviewed complete catalogue with the expected profile configuration revision and active pricing revision. Automatic synchronization requires two identical complete catalogue fetches under the same configuration before publishing a replacement revision atomically. A failed or malformed fetch preserves the last good active catalogue. Missing models are removed only by a confirmed replacement catalogue.

Editing a profile increments its configuration revision and invalidates the old active tariff until a new revision is published. Disabled supplier/profile state prevents new routing. Interval zero disables automatic checks; enabled intervals are 60–604800 seconds. Legacy global price synchronization excludes supplier-bound channels; never feed their procurement prices into global model settings or the shared `highest` strategy.

Price fetching uses public HTTPS on port 443, validates DNS-resolved addresses, pins the connection to allowed public IPs, restricts redirects to the original origin, and bounds response size and duration. Profile URLs contain no credentials. Keys belong in their intended channel credential field, not supplier profile URLs or logs.

## Administrative API

Use authenticated server access. Management credentials differ from inference API keys. These endpoints require root unless stated otherwise; see `router/supplier-router.go` for the deployed route contract.

| Method/path | Purpose |
|---|---|
| `GET`, `POST /api/suppliers` | List/create supplier identities |
| `PUT`, `DELETE /api/suppliers/:id` | Update/delete supplier |
| `GET`, `POST /api/suppliers/:id/profiles` | List/create profiles under a supplier |
| `PUT`, `DELETE /api/suppliers/profiles/:id` | Update/delete profile |
| `GET /api/suppliers/profiles/:id/tariffs` | Read tariffs, active revision, configuration revision |
| `POST /api/suppliers/profiles/:id/tariffs` | Publish reviewed complete tariffs |
| `POST /api/suppliers/profiles/:id/sync` | Fetch/confirm a complete catalogue |
| `GET`, `PUT /api/suppliers/group-bindings` | Read/apply atomic group and channel bindings |
| `GET /api/suppliers/migration-preview` | Read-only legacy classification preview |
| `GET /api/suppliers/choices` | Minimal supplier/profile choices; admin + `ChannelSensitiveWrite` |
| `GET`, `PUT`, `DELETE /api/official-prices` | Maintain separate official reference prices; DELETE takes `{ "model_name": "..." }` |

Tariff publish/sync requests require `expected_config_revision` and `expected_revision`. Publish additionally requires the full `tariffs` array; `[]` explicitly removes every model. A stale revision returns a conflict: reread and review before retrying.

An atomic binding request has this shape, with existing stable IDs:

```json
{
  "bindings": [{"group_id": 42, "supplier_id": 3, "profile_id": 8}],
  "channels": [{"channel_id": 100, "supplier_id": 3, "profile_id": 8, "group": "42"}]
}
```

Both nullable IDs must be provided together when changing a channel binding. General channel writes still require their channel RBAC permissions; supplier-sensitive changes require `ChannelSensitiveWrite`. A compatible binding is not proof that the credential actually uses that upstream source: verify the provider key's group, source pinning, fallback, and supported protocols separately.

## Public prices and privacy

`GET /api/pricing/v2` returns final retail group rates and tiers. It does not expose supplier identity, profile IDs, procurement conversion, tariff revisions, raw expressions, or markup. The legacy pricing API represents unbound/default pricing only. Ordinary user/token access cannot manage suppliers or hidden channel fields. Supplier task snapshots remain private; user task/log responses redact supplier routing and procurement details.

Official model-developer reference prices are a separate dataset with a source URL, quote timestamp, and unit. They are neither a billing fallback nor procurement data. Public display must retain their source/date and must not invent percentage-savings claims from incomparable tariffs.

Public model-provider metadata (`vendor_id`, name, icon and description) remains separate from proxy supplier identities. The public provider facet uses the same catalogue metadata as model cards, including the existing default model-name rules when explicit metadata is missing. It must continue to list model developers such as OpenAI and Anthropic. Proxy suppliers never populate this facet.

The optional public `base_reference` exposes the existing global/default input-output pair for display comparison. It never fills a missing supplier tariff or changes billing. Price cards select the minimum input-plus-output sum from an available group under the selected filters, keeping both rates and cache prices from that same group. Savings are `max(0, (1 - retail_sum / reference_sum) * 100)` using the input/output pair; cache/tool rates are excluded. Benefit is the default catalogue sort, with unknown savings last; explicit name and price sorts remain available. Official references take precedence when comparable, and unknown or incompatible units do not produce a guessed saving. A tiered global reference uses a valid configured base pair or its first standard tier, not an invented price for an unknown model. The card omits the redundant short-context heading; detailed group prices retain their actual context boundaries.

For Auto routing, verifiable token quotes are ordered by the frozen retail input-plus-output sum for the applicable request context and tier. For one model this gives the same benefit order against a shared baseline. Both rates must be known; explicit zero is valid. Unavailable routes are skipped and the next eligible group is tried. Fixed-unit and opaque-expression candidates retain their existing quote-cost positions; their units are not guessed or compared as token pairs. Reservation remains sufficient for eligible retries and settlement still uses each request's actual frozen contract.

## Administrator usage-log filters

Common usage logs support an administrator-only `proxy_supplier_id` filter on the list, total and statistics. Omission selects all records; `0` selects records with no recorded supplier. Supplier choices and text suggestions are available only through authenticated administrator routes:

- `GET /api/log/supplier-choices`: recorded positive supplier IDs and display names, independent of the channel's current binding.
- `GET /api/log/suggestions`: existing model, group, token-name and username values from the current log-filter scope, excluding the field being edited.

Both use literal substring search, an empty-query first page, `limit` up to 100, and a continuation cursor. Choices exclude their own active supplier filter so another supplier remains selectable. Ordinary users and limited API-key sessions cannot enumerate these values or apply supplier filters; the standard authorization layer may use its existing HTTP 200 `success:false` envelope. Numeric/request ID fields retain ordinary text entry. The client debounces option searches by 300 ms, cancels stale requests and partitions caches by actor and role. Masked fields do not show or query suggestions.

New log records freeze the selected supplier ID before durable outbox delivery. A private metadata sidecar preserves it through JSON serialization and pre-migration writers; the indexed `logs.proxy_supplier_id` column is never part of public log JSON. Task completion/refund and error logs retain their original identity after channel rebinding or deletion. Historical records with no snapshot remain `0`; no backfill guesses their supplier from a live channel. Legacy privacy classification may still redact a record without assigning it a supplier ID.

Supplier names are resolved separately from the main database, so a distinct log database remains supported. Missing/deleted identities use an ID label. Migrations add the log column without rewriting historical attribution; ClickHouse adds the column explicitly. ClickHouse reads keep synthetic display IDs and outbox replay preserves the original partition/order timestamp. The column-readiness cache avoids a metadata round trip on every stable log write.

## Async limits

Supplier-backed asynchronous tasks currently support **fixed per-request or per-second tariffs only**. Supplier token/expression async tariffs are rejected before submission because aggregate-only polling cannot safely reconstruct all chargeable token lanes. A fixed per-second model still needs a supported request duration contract.

Each new task stores a private transport fingerprint, supplier/profile provenance, and minimal result destination (channel type, effective base URL and proxy). Polling, realtime refresh and actions verify the fingerprint against the authoritative channel before forwarding any stored upstream ID or credential. Channel binding/connection changes are blocked while jobs remain unfinished. The fingerprint also catches a change made while submission was in flight, before the task row existed. A mismatch retains the unresolved task and its held quota for administrative review; it never sends the old credentials to the changed destination or automatically refunds an ambiguous request.

Terminal status is read from the saved task, so later key rotation or channel replacement does not hide historical jobs. Completed video retrieval stays at the saved destination with its selected credential. A current enabled key may replace that credential only when the channel still has the identical type, effective base URL, proxy and supplier/profile. Changed destinations never receive an old task ID or credential. Older tasks without a result destination can still expose local status and saved public assets without channel credentials; authenticated content cannot be recovered if its original credential/destination is no longer available.

Supplier-backed Suno keeps its protocol-specific response: submission returns `TaskResponse<string>` with the public task ID; fetch and batch fetch retain typed songs or lyrics. Supplier fields are redacted and audio/video/image URLs point to the owned-task media proxy. `GET /v1/tasks/:task_id/content?kind=audio|video|image|image-large&index=N` accepts a user session or inference token and serves an asset from that user's persisted Suno result. It accepts no arbitrary URL and forwards no channel credential to the asset host. Supplier media responses use a safe header allowlist and `Cache-Control: private, no-store`.

Supplier-backed Midjourney requests are blocked until that legacy task path has complete immutable pricing snapshots. Existing unbound Midjourney behavior remains available. Do not bypass these guards by treating token/expr rates as per-request prices or stripping supplier metadata.

Supplier-backed fixed-price image generation is also rejected before dispatch: a single per-request rate does not establish variant, quality, size and quantity costs. Token-priced image generation remains supported when all used token lanes are priced. The public catalogue marks those unsupported fixed-image routes unavailable.

Supported server tools are explicitly priced web search and file search with invocation counters. Client function tools remain available. Other server tools, including image-generation tools without complete per-item usage counters, are rejected before dispatch even when a scalar tool price is present. Realtime server tools are also rejected because their fee usage is unavailable in that protocol.

Unsupported catalogue modifiers are rejected rather than discarded. This includes unimplemented cache TTLs (such as OpenLux GPT 12-hour cache writes), image-cache rates, video-resolution modifiers and generic per-model/peak-price modifiers. A selected profile containing such a contract cannot synchronize until that billing condition is implemented and verified. Request guards also reject unknown cache TTLs/scopes and extended cache-retention modes. Supported cache write TTLs are 5 minutes and 1 hour.

## Captured provider catalogue compatibility

The following parser checks used public catalogue captures dated **2026-11-04**. Counts describe those captures, not permanent provider inventories. Normalization is separate from authenticated key/source binding, protocol support, a paid model request, and complete runtime support.

| Supplier | Exact group/source selector | Captured result |
|---|---|---|
| OpenLux | `Kiro-Claude-1` | 12 complete model contracts normalized |
| OpenLux | `Claude-Code-1` | 14 complete model contracts normalized |
| OpenLux | `Codex-Gpt-1` | Rejected because `cache_creation_12h_ratio` is unsupported |
| OpenLux | `Alibaba-video-1` | Rejected because `video_resolution_ratios` is unsupported |
| BBLABU | `Claude kiro渠道` | 8 complete model contracts normalized |
| BBLABU | `OpenAI` | 10 contracts normalized; the two fixed-price image models remain unavailable for dispatch |
| relay.fast | `gpt:gpt-special-2` | 6 complete model contracts normalized; `family:tier` identifies the selected source |

The BBLABU capture checks explicitly supplied `usd_per_credit = 0.006` as an illustrative verified-quote basis; it is not a hardcoded funding rate. Other captures used an explicit conversion of 1 for parser verification, which does not establish an account's actual funding cost. Unsupported request/header/time-dependent expressions are rejected at publication rather than reduced to a guessed input/output pair. Keep the original source conditions when analyzing a rejected catalogue.

## Migration and operation

1. Confirm the running deployment supports supplier pricing through authenticated capability reads. A local branch or a frontend control does not establish live server capability. An old server must stay on its legacy workflow.
2. Obtain a read-only migration preview. Legacy tags are hints for classification, not proof of supplier identity, upstream binding, or real funding conversion. No automatic binding migration is safe from tags alone.
3. Verify procurement conversion, provider credential binding, complete tariff coverage, protocols, and the intended retail margin. Preserve every existing group ID and unrelated resource.
4. Create/review supplier profiles and tariffs within the authorized scope. Preview the exact channel/group changes and bind them atomically only after the tariffs and credentials are verified. Preserve channel/group tags and the shared tag registry for compatibility and classification; keep them separate from supplier controls.
5. Verify retail prices, source/profile membership, discovery boundaries, and user-access redaction. Keep supplier-bound channels out of global price-sync sources. An unpriced newly discovered model must remain unavailable until its tariff is published.

Deploy only after an explicit deployment instruction, following the registry-image/database-backup procedure. Upgrade **all application nodes before activating supplier bindings**: an older node cannot enforce these rules. Preserve data volumes and supplier tables. For rollback, first disable bound channels while the supplier-aware release is still running, then follow the rollback procedure; do not drop supplier data or leave bound traffic running on an older binary.

## Verification boundary

The schema and transaction design must support SQLite, MySQL, and PostgreSQL. Run the relevant database-backed suites on each engine before claiming three-database verification; SQLite-only results do not establish MySQL/PostgreSQL behavior. Preserve request/task snapshots and verify final outgoing feature guards after channel overrides when reviewing changes to the transport path. Record final checks against the finished working tree; an earlier full-suite pass does not verify later transport edits.

### Local verification record — 2026-11-04

This work used branch `codex/supplier-pricing` in a separate worktree based on `5239abc8`. The following checks were performed locally during implementation:

| Area | Executed checks and boundary |
|---|---|
| Persistence dialects | `TestSupplierPersistenceDialects` passed on SQLite, PostgreSQL 15 (`postgres:15-alpine`), and MySQL 8.4 in isolated test databases. SQLite runs unconditionally; PostgreSQL/MySQL were enabled with the two test DSNs. Temporary database containers were stopped and removed after verification. |
| Backend | Go 1.25.6: `go test ./... -count=1 -timeout=180s` passed after the final transport guards: 2,014 tests across 90 packages. |
| Frontend | Frontend tests, `bun run build:check`, type checking, targeted lint on changed files, and protected-header formatting checks passed during implementation. |
| Local UI | Browser review covered desktop width 1728, mobile width 390, keyboard behavior, supplier-bound guards and Russian model metadata. |
| Provider catalogues | Captured public catalogue normalization checks produced the compatibility results above. These are not paid inference or credential/source-pinning probes. |

The cross-database command was:

```bash
rtk env SUPPLIER_TEST_POSTGRES_DSN='<isolated PostgreSQL DSN>' \
  SUPPLIER_TEST_MYSQL_DSN='<isolated MySQL DSN>' \
  go test ./model -run '^TestSupplierPersistenceDialects$' -count=1 -timeout=90s
```

No paid provider inference, production migration, deployment, or production configuration change is claimed by these local checks.

### Review corrections — 2026-11-04

The follow-up on `codex/supplier-pricing`, based on `c7bfaa70`, corrects partial-usage settlement and overflow handling, locked-channel transport checks, Auto route isolation, stale legacy synchronization, supplier-only model discovery, completed-task retrieval, Suno and Responses wire contracts, request-feature validation, tool-price display, catalogue query growth, and pre-migration channel writes. Binding mutations resolve persisted groups rather than a stale cache. Pricing option queries quote the reserved `key` column through GORM conditions on every affected MySQL synchronization path.

| Area | Final follow-up checks |
|---|---|
| Backend | Go 1.25.6: `go test ./... -count=1 -timeout=180s` passed: 2,095 tests across 90 packages. Regression tests use real local wallets, persisted tasks and HTTP test servers; no paid inference is involved. |
| Persistence | `TestSupplierPersistenceDialects` passed without dialect skips on SQLite, PostgreSQL 15 and MySQL 8.4. It exercises two retail groups sharing one profile, mapped model tariffs, official references, private-field redaction, unavailable revisions, disabled suppliers, binding generations and stale JSON-price updates. |
| Frontend | All 347 tests and `bun run build:check` passed. Targeted lint has no errors; one existing toolbar warning remains. Protected-header-safe formatting passes for changed files and all six locales. The full repository formatting check still reports pre-existing issues in unrelated frontend files. |
| Browser | Desktop and 390 px mobile: tool fees, zero prices, USD/CNY conversion, recharge conversion, 1K token units, keyboard focus and overflow checked; no console warnings/errors. Token units never rescale per-call fees. Model-provider labels remain distinct from proxy suppliers. |
| API contract | The new owned-task media operation and inference-token security declaration pass OpenAPI validation. Completed-task transport/key rotation, sanitized Suno results and supplier media headers have HTTP regressions. |

Owned local servers and temporary database containers were stopped after verification. These checks do not recover missing historical credentials for old tasks without a saved result destination; their local status remains readable.

### Compatibility with the running release — 2026-11-04

A read-only production `/api/status` check reported **v1.1.214**, newer than this worktree's original v1.1.206 base. Merge `f0edaa4d` includes the v1.1.214 release commit `d31df151`; it preserves shared platform tags, limited API-key sessions, sign-in methods, referral changes and the home page. Never deploy the earlier supplier-only branch without these intervening production changes.

Comparison with v1.1.214 found and fixed two additional unbound regressions. Auto quotes now normalize global `per_call` candidates before applying duration multipliers while retaining duration for supplier `per_second` candidates. Explicit forced unbound channels retain their previous ability-list bypass, subject to a configured global billing contract; supplier routes retain binding and model checks. New HTTP regressions cover both cases. Limited key-session tests confirm that root-owned inference keys still receive only limited access and cannot expose procurement logs or manage suppliers.

The following rehearsal used immutable source archives, Go 1.25.6 Darwin/arm64 server binaries, a synthetic SQLite database with 29 rows across 10 business tables, and one persistent `SESSION_SECRET_FILE`:

```text
v1.1.214 → f0edaa4d → repeated new-version startup → v1.1.214 rollback
```

Actual startup migrations and HTTP probes passed at every step. Every previous column in users, tokens, channels, abilities, tasks, midjourneys, models, options, setups and platform tags stayed unchanged. Balances, credentials, task JSON, group/model/cache coefficients and tag values were preserved. A session cookie issued once on v1.1.214 remained valid throughout. Token authentication, model discovery, self/group endpoints and both pricing APIs passed; no suppliers, profiles or bindings were automatically created. A separate live SQLite/PostgreSQL 15/MySQL 8.4 dialect fixture passed on this same source, including repeatable tag migrations and supplier isolation.

The full populated startup/rollback rehearsal was **SQLite only**. PostgreSQL/MySQL verification covers the dialect fixture, not the full startup sequence on existing production data. The rehearsal uses synthetic data and minimal embedded frontend assets; frontend/browser checks are separate. Task polling and paid inference were disabled, and unrelated log, telemetry, currency and system-task metadata were not asserted unchanged. Old-version rollback with active supplier bindings remains unsupported: disable bound channels before reverting application nodes.

Final validation after integration passed: **2,121 Go tests across 90 packages**, **347 frontend tests**, frontend type checking/production build, and static Linux amd64/arm64 builds with Go 1.25.6. Build frontend assets before Go tests/builds: both embed `web/default/dist`, so concurrent frontend cleanup can make the Go embed step fail. The global model-test fixture now includes the existing platform-tag table. Separate pricing fixtures were made independent of earlier tests; the former standalone hang was a nil-map panic while holding a test mutex, not an application deadlock.

Desktop/mobile browser checks cover supplier/profile selection alongside shared tags, group tiers/cache/tool prices, custom-tag Enter/save/reload and clearing, and nested Escape behavior. Group tags retain a newly entered selection; the first Escape closes the combobox list and the second closes the drawer. Backend runtime code is unchanged after the immutable migration rehearsal; subsequent changes affect these UI controls, test setup and this record. Temporary servers, rehearsal archives/secrets/databases and owned PostgreSQL/MySQL containers were removed or stopped; a sanitized local rehearsal report was retained.

First deploy the supplier-aware release with existing bindings unchanged. Verify legacy traffic and authentication before enabling new supplier bindings. The production release image, current image/version and a fresh verified backup still need the normal deployment preflight; local rehearsals do not replace a production backup. Production was not changed by this verification.

# Return to shared model pricing

This release removes supplier/profile management, isolated supplier tariffs, and the supplier log filter. Channels and pricing groups again use the shared tag catalogue. Model prices use the global pricing configuration and group multipliers; the public catalogue shows the previous prices, savings, and group coefficients.

The rollback preserves unrelated releases, existing groups and tags, database tables, historical records, balances, and stored credentials. It does not infer old group coefficients from supplier tariffs: different input/output, cache, and tool rates cannot generally be converted to one multiplier.

## Before installing on an existing supplier-aware deployment

Use the supplier-aware release to review and prepare the database before replacing the application:

1. Stop new supplier-bound traffic on every application node.
2. Export and verify the affected group tariffs and original shared-price coefficients. Configure the intended global prices and legacy group coefficients explicitly.
3. Complete or reconcile supplier-priced tasks and durable billing work under the supplier-aware release. Preserve the task history and billing evidence in a verified archive before removing incompatible task rows from the active application database.
4. Remove supplier/profile bindings from both channels and pricing groups using the supplier-aware administrative APIs. Clear disabled-channel bindings too.
5. Stop every supplier-aware application node and other supplier-aware writer, create and verify a fresh database backup, then install the ready registry image following the normal production update procedure. Do not run mixed supplier-aware and rollback nodes against the same database: a startup-only check cannot prevent an older node from creating a new native contract later.

Startup performs a read-only compatibility check before options, routing, authorization initialization, or workers start. Remaining supplier bindings, supplier-bound channel fields, native supplier task records (including completed tasks), or incompatible pending billing events stop startup with an actionable error. This prevents legacy billing from silently reinterpreting a supplier markup, replaying an unsupported billing contract, or sending historical task credentials through a changed channel.

Do not bypass this check by deleting balances, overwriting private task snapshots, or dropping supplier tables. Empty retained supplier tables and unused columns are safe. Historical usage logs remain available with procurement and routing details removed from user responses.

All application nodes must use a compatible pricing configuration. Publishing this release does not install it or modify a production database.

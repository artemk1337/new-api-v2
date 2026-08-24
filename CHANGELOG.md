# Changelog

## v1.1.150

- Hardened payment-method snapshots and server-authoritative top-up quotes, including exact decimal conversion, provider availability checks, and safer payment metadata lookups.
- Aligned wallet previews with backend rounding and commission/cashback calculations, and redirected retired currency settings to platform currencies.

## v1.1.149

- Redesigned wallet top-ups with payment-method comparison, paginated history, promo and referral details, and configurable cashback thresholds.
- Added platform currencies and per-payment-method settlement currency, commission, rate synchronization, and server-authoritative checkout quotes.
- Hardened payment settlement with immutable provider snapshots, configurable pending-payment expiry and creation limits, direct-referral rewards, and concurrency-safe quota updates.

## v1.1.148

- Normalize Claude-to-OpenAI-compatible cache usage before billing so cached input is not charged twice.
- Preserve generic and split 5-minute/1-hour cache-creation tokens in tiered billing and context-length tiers.

## v1.1.145

- Keep renamed channel models available to users while inheriting the original model's pricing, quota, compact, task, audio, and retry billing rules.
- Preserve mapped targets during channel synchronization and pricing resolution, including duplicate aliases and empty mappings.

## v1.1.144

- Preserve provider models used as mapping targets during channel auto-sync, including duplicate aliases pointing to one model.

## v1.1.143

- Return HTTP 429 for explicitly saturated or overloaded upstream 502 responses while preserving ordinary 502 errors.

## v1.1.142

- Fixed structured dynamic pricing display on pricing pages without a request-selected tier.
- Kept usage-log dynamic pricing details on a safe raw-expression fallback when the logged tier is missing or unknown.

## v1.1.141

- Added configurable referral rewards from successful wallet top-ups, with atomic callback handling and referral-balance transfer support.
- Added a dedicated Referral Program section to billing settings.

## v1.1.140

- Fixed Claude usage-log details to keep text input separate from cache tokens.

## v1.1.139

- Show input and output rates in billing details while keeping token breakdowns count-only.

## v1.1.138

- Removed the provider-returned model explanatory text from usage-log details.

## v1.1.137

- Made usage-log billing details compact and consistent across fixed, per-call, and dynamic pricing.
- Show the matched dynamic tier, its conditions, actual billed token categories, and a reconciled calculation.
- Correctly represent cache-read tokens in dynamic pricing details for both Claude and OpenAI-compatible usage semantics.
- Added a repeatable local billing-details demo seed.

## v1.1.136

- Display requested, sent upstream, and provider-returned model details in the usage-log dialog when available.

## v1.1.135

- Sort pricing sidebar groups alphabetically by their display names.

## v1.1.134

- Renamed the Russian FAQ settings heading to “FAQ”.

## v1.1.133

- Added requested, sent upstream, and provider-returned model names to usage-log details when explicitly available from the provider response.

## v1.1.132

- Fixed Anthropic streaming tool-use SSE block indexing so content_block_start, content_block_delta, and content_block_stop remain matched for sparse, parallel, and metadata-only streams.

## v1.1.131

- Support ranged video proxy streaming, including HTTP 206 partial responses.
- Safely auto-fallback known upstream saturation and concurrency-limit 429 errors.
- Avoid routing unsupported Alibaba TTS requests upstream.

## v1.1.130

- Added configurable fixed per-call pricing for task models, preventing duration multipliers from being applied to providers that charge per request.
- Use the task's original upstream key when polling Gemini and Vertex video tasks in real time.

## v1.1.129

- Added an API-key onboarding dialog after creating a key, with a protected revealable secret, Base URL, and copyable cURL, Python, Node.js, and Go examples.
- Kept API-key onboarding responsive and safe: code samples are scroll-free, language tabs adapt to mobile, and full secrets are fetched only through the protected token endpoint.

## v1.1.128

- Added total, active, and inactive channel counts to each admin pricing group.

## v1.1.127

- Added a Yesterday quick date-range preset to the usage journal.

## v1.1.126

- Automatically remove unavailable upstream models during channel auto-sync and report those removals accurately in update task summaries and notifications.

## v1.1.125

- Keep incomplete pricing model-card rows left-aligned with stable card widths.

## v1.1.124

- Automatically associate Veo video models with Google and display the Gemini provider icon in the pricing catalog.

## v1.1.123

- Made the pricing sidebar height dynamic so it no longer has an internal scroll area.
- Added a model-card page-size selector to choose how many pricing models are shown per page.

## v1.1.122

- Renamed the dynamic pricing tier Length field to Context in the model details view.

## v1.1.121

- Added an idempotent one-click API-key onboarding flow on Dashboard Overview with safe first-key defaults and atomic duplicate and limit protection.
- Removed the redundant Create API Key action from the expanded setup guide.

## v1.1.120

- Sort pricing models by benefit by default and place the Benefit option first in the sort menu.

## v1.1.119

- Highlight ready-to-deploy new system-update releases with a subtle green glowing card border.

## v1.1.118

- Replaced top-up discounts with threshold-based cashback while keeping payment totals at full price.
- Added cashback configuration and previews to the admin and wallet interfaces across supported locales.
- Persisted the exact cashback-adjusted quota on new payment orders so later configuration changes cannot alter fulfillment.
- Made provider and manual top-up completion atomic and idempotent, with guarded legacy recovery and fail-closed handling for ambiguous quotas.

## v1.1.117

- Restored system updates from the admin UI after a release image is published and ready.
- Safely deploy updates with a verified PostgreSQL backup, health/version checks, rollback, and host-correct Docker Compose paths.

## v1.1.116

- Consolidated usage logs into one Journal page with General, Drawing, and Tasks tabs while preserving direct section links.
- Renamed the Russian sidebar entry to "Журнал" and removed the duplicate task-log entry.

## v1.1.115

- Prevented system updates to releases whose Docker image is still building or not marked ready for deployment; enforced readiness validation in both the update UI and backend.

## v1.1.114

- Clarified sidebar navigation labels for Overview and Dashboard across supported locales.

## v1.1.113

- Simplified profile security cards by removing the unused Passkey last-used status and redundant two-factor authentication guidance.

## v1.1.112

- Show update and rollback releases together in the system update dialog, separated by clear headings and a divider.
- Do not present an older rollback release as a new version.

## v1.1.111

- Replaced in-container automatic updates with a safe host-side update script and recovery checks.
- Made telemetry agent controls idempotent and added a manual host-side agent setup command.

## v1.1.110

- Renamed the Russian wallet top-up heading to "Пополнение" and removed its redundant payment-method hint.

## v1.1.109

- Removed the legacy Chat tab, presets, and related admin settings; renamed Playground to Chat and moved it to General.

## v1.1.108

- Removed quota and request statistics from the profile header and wallet page.
## v1.1.107

- Renamed the Russian dashboard overview block to "Статистика".

## v1.1.106

- Automatically identify Midjourney `mj_*` models and show their Midjourney provider icon, including existing model metadata without a provider.

## v1.1.105

- Separated available updates from previous rollback versions in the system update dialog.
- Kept rollback actions available in a collapsed section and added localized release group labels.

## v1.1.104

- Added a Benefit sort option to the pricing catalog, ordering models from the largest displayed savings to the smallest.

## v1.1.103

- Fixed channel group search to match displayed group names as well as internal IDs.
- Removed the preselected default group when creating a channel.

## v1.1.102

- Added suggestions of existing channel tags in the channel editor while allowing custom values.
- Clarified internal notes as user-invisible and removed duplicate helper text.

## v1.1.101

- Updated pricing preview cards with group-aware current and base token prices, a savings percentage, and compact responsive layout.
- Added clearer price comparison styling: green savings and red strikethrough base prices, with copy and details actions retained.

## v1.1.100

- Added a smooth animation when opening model details in the pricing catalog, while respecting reduced-motion preferences.

## v1.1.99

- Read update build statuses from GitHub Actions in one request instead of checking every release image serially.
- Keep the registry manifest check only when starting the selected update.

## v1.1.98

- Removed the Performance health panel from the dashboard overview.

## v1.1.97

- Automatically schedule an updater-sidecar upgrade to the selected release after a successful system update.
- Run the sidecar replacement through a detached helper so the updater can safely replace itself.

## v1.1.96

- Added dollar symbols to preset top-up amounts in the recharge form.

## v1.1.95

- Added version-specific system updates with selectable stable releases and safe rollback support.
- Show whether each release image is ready, building, or unavailable; installation is enabled only after its image is ready.
- Added updater-sidecar image readiness checks and GitHub Actions build tracking for tagged releases.

## v1.1.94

- Added fractional top-up amounts across payment providers, including configurable minimums and preset amounts.
- Accepted both `.` and `,` decimal separators in wallet and payment settings inputs.
- Preserved the requested fractional amount in top-up history while keeping legacy records compatible.

## v1.1.93

- Show each top-up coefficient group together with its coefficient when configuring a payment method.

## v1.1.92

- Added an explicit NOWPayments enable switch in payment settings. Disabling it hides new crypto top-ups while preserving webhook processing for pending payments.

## v1.1.91

- Added a configurable top-up coefficient group for each payment method, so preset prices and charged amounts use the same method-specific calculation.
- Removed the payment compliance confirmation gate from payment settings, top-ups, redemption codes, subscriptions, and referral transfers.

## v1.1.90

- Removed the introductory description from the Model Square header.

## v1.1.89

- Simplified pricing model cards by removing descriptions, group and endpoint labels, and redundant billing-type labels while preserving aggregated latency, throughput, and status metrics.

## v1.1.88

- Removed the global Auto group allowlist. Auto now always uses pricing groups available to the user, while API keys can still limit their own candidate groups.
## v1.1.87

- Added configurable exchange-rate history with a master-only background updater.
- Added Bybit P2P USDT/RUB quotes based on the median of current sell advertisements, plus the Bank of Russia USD/RUB provider.
- Added billing settings for the provider and update period from one minute to one day.

## v1.1.86

- Removed the Uptime Kuma dashboard widget, console settings, status API, and unused configuration support.

## v1.1.85

- Removed the remaining Classic UI configuration, translations, documentation, and internal migration tooling while preserving redirects for legacy `/console/*` links.

## v1.1.84

- Sorted model group pricing from the lowest effective coefficient to the highest, matching the Auto group routing chain.

## v1.1.83

- Fixed clipped left-axis labels in the System Telemetry I/O wait chart.

## v1.1.82

- Fixed incremental `<think>` parsing for multiple tags in one stream chunk and partial tags split across chunks.

## v1.1.81

- Optimized streaming chat rendering: incremental reasoning parsing, memoized message rows, and lightweight code fences while responses stream.
- Improved admin user filtering by avoiding empty substring scans and adding indexes for group, role, and status filters.
- Reduced pricing catalog recomputation by indexing vendors and processing each enabled model only once per metadata rule.

## v1.1.80

- Made pricing sync clearly separate protected manual prices from automatically managed models, with per-model and filtered bulk controls to restore Auto mode.
- Preserved each model's pricing mode when applying checked prices and added version-safe snapshots so stale checks cannot overwrite newer configuration.
- Added frontend test automation for the default admin interface.

## v1.1.79

- Hid the secondary load-average axis in System Telemetry so both charts share the same horizontal time scale.

## v1.1.78

- Added searchable selection of pricing groups when configuring group request-rate limits, while retaining support for custom user-group and `auto` keys.
- Prevented duplicate group-rate-limit selections and preserved accessible validation feedback in the shared combobox input.

## v1.1.77

- Added a selectable model request rate-limit period in seconds, minutes, or hours.
- Preserved compatibility with the legacy minute-based setting and added validation for canonical periods such as `10s`, `5m`, and `1h`.
- Switched rate-limit counters safely across rolling replicas while keeping Redis and in-memory windows consistent with the selected period.

## v1.1.76

- Stopped repeated telemetry-agent error notifications when its controller is unavailable or has not yet been upgraded.
- Show the controller as unavailable and disable its controls until it can be reached.

## v1.1.75

- Added an independent host telemetry agent that records CPU, memory, swap, I/O wait, load, disk usage, and the top three CPU-consuming processes every 10 seconds.
- Added 24-hour telemetry history with hourly retention cleanup, Root-only API controls, and System Info charts for 1, 6, and 24-hour periods.
- Added fixed updater-sidecar controls for starting and stopping the telemetry agent, plus release images for amd64 and arm64.

## v1.1.74

- Replaced the Chinese channel-test log labels with the English `Model test` text.

## v1.1.73

- Removed the legacy frontend and its runtime theme switcher; the platform now serves only the default interface.
- Simplified Docker, local development, Electron, and release builds to build one frontend only.
- Removed obsolete frontend-theme settings and the legacy synchronous log-cleanup endpoint.

## v1.1.72

- Added reasoning-output pricing to synchronized Yunwu tier contracts, including OpenAI, Gemini, Responses, and realtime usage normalization.
- Made reasoning settlement conservative when upstream usage omits the reasoning-token split, with AST validation that prevents unsafe billing expressions.
- Added reasoning pricing controls and accurate cost previews to both admin interfaces while preserving explicit zero-price lanes and warning before unsupported visual conversions.

## v1.1.71

- Renamed the header documentation link in Russian from «Документы» to «Документация».

## v1.1.70

- Displayed group names instead of technical IDs in the model rate-limits table.

## v1.1.69

- Fixed the displayed Auto group chain on model pricing pages so it follows the effective price order used for routing.

## v1.1.68

- Moved upstream pricing synchronization from environment variables into persisted channel settings in both admin interfaces, with per-channel endpoint mode and update interval.
- Added confirmed upstream price snapshots, conflict resolution strategies, per-model source selection, and safe price removal when a selected source disappears.
- Models without a positive base price or a valid tiered billing expression are no longer listed or relayed.
- Translated Chinese log messages into English and sanitized backend logs to prevent CJK text from reaching log outputs.

## v1.1.67

- Restricted manual upstream pricing synchronization to the Yunwu-compatible `/api/pricing` format and removed misleading endpoint-type choices from the channel selector.

## v1.1.66

- Added a guarded one-minute upstream pricing synchronization task for allowlisted Yunwu channels: it applies only unanimous model prices after two identical checks and reports unsupported or conflicting models.
- Converted supported upstream context tiers into billing expressions, including cache and audio pricing, and kept realtime tiered settlement consistent with accumulated upstream usage.
- Added concurrent-safe pricing configuration updates and surfaced skipped-pricing warnings in both admin interfaces.

## v1.1.64

- Made the Telegram sign-in action match the default OAuth button layout and moved the provider widget into a dialog.

## v1.1.63

- Simplified the Auto-group description in API-key creation and moved detailed routing and reserve guidance into an accessible popover.

## v1.1.62

- Fixed Auto group descriptions overflowing the API-key creation drawer by wrapping the text within the selected-group control.

## v1.1.61

- Returned the reserved quota for upstream errors unless the response explicitly confirms a charge; errors without billing evidence are no longer charged by estimate.

## v1.1.60

- Made Auto consider every pricing group available to the user; the legacy global Auto-group list no longer restricts routing or API-key group choices.

## v1.1.59

- Clarified the Russian Auto-group guidance: failover conditions, reserve return after a successful request, and insufficient-balance recovery.

## v1.1.58

- Fixed API-key group descriptions overflowing the group selection card by wrapping long text within the available width.

## v1.1.57

- Added the `Auto` token group: it tries eligible pricing groups from the lowest effective price to the highest and transparently switches only after an upstream-confirmed no-charge failure.
- Made token group selection mandatory, migrated legacy empty selections to `Auto`, and clarified group and Auto behavior in both interfaces.
- Reserved Auto requests at the highest eligible group price and added clear insufficient-reserve guidance.
- Preserved billing and usage logs for charged failures, including asynchronous task settlement and retryable log delivery.

## v1.1.56

- Fixed duplicate-looking Telegram Login Widget rendering while keeping it centered in the OAuth form.

## v1.1.55

- Aligned the Telegram Login Widget with the default frontend OAuth button layout.

## v1.1.54

- Added Telegram Login Widget support in the default frontend for sign-in and account binding.
- Masked saved Telegram bot tokens in system settings and made Telegram OAuth configuration validation safer.

## v1.1.53

- Shortened Russian labels in the system-settings sidebar and clarified its section names.
- Added sidebar-specific translation keys so shared labels outside the menu remain unchanged.

## v1.1.52

- Reduced database load from admin user searches by debouncing the search input.
- Removed the unnecessary read transaction around user search queries, reducing the time SQLite read snapshots are held.

## v1.1.51

- Close the release dialog after a system update is successfully started.

## v1.1.50

- Refined the quota adjustment dialog with a clear segmented mode selector and an action-specific primary button.

## v1.1.49

- Removed the auxiliary pricing-group labels below group names.

## v1.1.48

- Removed the recommended actions card from the dashboard overview.

## v1.1.47

- Renamed the pricing filter sharing button to “Share” and added localized labels.

## v1.1.46

- Added a copyable link for sharing the current pricing-page filters.
- Shortened the Russian text showing the number of available models.

## v1.1.45

- Made `PricingGroups[].selectable` the source of user-visible pricing groups across model listings, pricing responses, API keys, and auto-group selection.
- Migrated legacy group availability safely into canonical pricing groups while preserving stable IDs, descriptions, and rollback compatibility.
- Rebuilt incomplete channel abilities atomically and invalidated pricing data immediately after group or channel-status changes.

## v1.1.44

- Вынесено управление новостями в отдельный раздел меню Super Admin с сохранением совместимости со старыми настройками анонсов.

## v1.1.43

- Переведены сообщения и экран двухфакторной аутентификации с китайского на русский язык.

## v1.1.42

- Normalized relay channel-selection failures to English for 503 cases, including model/group no-available-channel errors.

## v1.1.41

- Added a visible NOWPayments configuration tab to payment settings.
- Restored NOWPayments as a selectable crypto top-up method in the wallet.

## v1.1.39

- Normalized saturated upstream-model errors to English relay messages.

## v1.1.38

- Replaced hardcoded Chinese YooKassa top-up messages and logs with English text.

## v1.1.37

- Display quota-warning thresholds in the configured platform currency in both interfaces, including decimal input and correct conversion back to internal quota units.
- Keep quota-warning threshold values synchronized when the platform currency configuration changes.

## v1.1.36

- Added Telegram notifications for quota warnings, channel events, and upstream model updates, with per-user Chat ID settings and Worker support.
- Added localized Telegram notification settings in the Default and Classic interfaces.

## v1.1.34

- Normalized relay/admin error logs for known upstream Chinese errors while preserving raw diagnostics in structured log metadata.

## v1.1.33

- Redesigned the system update progress block with vertical stage indicators and localized stage labels.
- Redirected users to the dashboard sign-in route after a completed system update and hid the completed update block.

## v1.1.32

- Fixed pricing-group renames so storefront-usable groups and channel abilities are normalized to stable group ids before display names change.
- Restored model storefront visibility for channels bound to renamed pricing groups.

## v1.1.31

- Fixed stale system update notifications after a newer version is already running.

## v1.1.30

- Fixed updater readiness checks to recognize the current `/api/status` response shape and avoid rolling back a healthy updated service.

## v1.1.29

- Renamed the billing settings group and model pricing sections to Groups and Models in the default frontend.

## v1.1.28

- Renamed the top navigation Console tab to Dashboard in the default frontend.

## v1.1.27

- Changed the platform default interface language to Russian while preserving saved user language preferences.
- Reordered language selectors so Russian is first, English is second, and the remaining languages follow after them.

## v1.1.26

- Completed pricing group migration to stable internal ids while keeping names as UI display labels across channels, tokens, pricing, logs, flow data, tasks, performance metrics, and model listings.
- Added id/name companion refs and catalog responses for pricing-group API surfaces, with legacy name input normalized to id values at backend boundaries.
- Preserved user-group domains for users, subscriptions, top-up ratios, and rate-limit settings so they are not rewritten as pricing-group ids.
- Hardened pricing group settings: duplicate trimmed names are rejected, `default` keeps reserved id `1`, and default deletion is blocked.
- Fixed pricing group edge cases for model storefront visibility, auto-group/error logs, legacy channel/ability/token/task migration, and model request rate-limit updates.

## v1.1.24

- Finished pricing group id hardening across backend and default/classic UI: pricing requests now keep id values while display surfaces resolve names through group refs/catalogs.
- Fixed playground, API key, channel, usage log, performance, flow, and task billing group handling so legacy names are normalized at boundaries without overwriting user-group domains.
- Kept users, top-up ratios, and subscription upgrade/downgrade groups in the user-group domain instead of reading pricing groups.
- Added admin visibility for pricing group ids, blocked reserved default pricing group deletion, and preserved duplicate-name validation.
- Added/updated regression coverage for legacy pricing-group migration, channel/ability normalization, playground locked token groups, task/log/flow refs, and user-group boundaries.
- Added YooKassa payment return synchronization so users can refresh a pending payment by trade number after returning to the wallet page.

## v1.1.20

- Fixed the system update UI so a completed deploying task does not trigger repeated reloads after the target version is already running.

## v1.1.19

- Fixed wallet top-up amount discounts so JSON map entries are treated as minimum amount thresholds, matching the admin visual editor and payment calculation behavior.
- Updated the payment settings hint to describe amount discount maps as threshold-based discounts.

## v1.1.18

- Removed the updater's dependency on the installation directory name by reusing the Docker Compose project label from the running service.
- Kept explicit compose project overrides via environment variables and `.env` for operators who set them intentionally.

## v1.1.17

- Fixed fresh installs to pin the Docker Compose project name to `new-api`, keeping initial deployment, updater deployment, and rollback on the same Compose project regardless of install directory.

## v1.1.16

- Fixed updater deployments to use a stable Docker Compose project name when running from the sidecar workspace, avoiding container-name conflicts during update and rollback.
- Included Docker command output in updater errors so failed Compose operations show the real stderr in the admin UI.

## v1.1.15

- Switched default Chinese provider names in pricing facets to English names.
- Added compatibility mapping so existing legacy Chinese vendor records are displayed with English names without changing stored data.

## v1.1.12

- Removed the redundant enable flag from the system update flow.
- Made update checks always fetch stable tags and show changelog entries for each newer version, while update installation is gated by updater sidecar credentials.

## v1.1.11

- Added production deployment guardrails requiring prebuilt Docker images by release tag and forbidding server-side builds.
- Documented the image-only production deployment flow in README, AGENTS, and the local Codex deployment skill.

## v1.1.10

- Added automatic group selection for API keys without an explicit group, choosing the lowest-priced accessible group that supports the requested model.
- Updated API key creation UI to leave the group empty by default, explain automatic selection, and allow clearing a selected group.

## v1.1.9

- Added a safe one-command install script that pins the selected release tag in `.env`, starts the updater sidecar, and preserves existing Docker volumes.

## v1.1.8

- Fixed the Electron build workflow to install and build `web/classic` separately, matching the release workflow dependency layout.

## v1.1.7

- Fixed the Electron build workflow to build both `web/default` and `web/classic` before compiling the embedded Windows binary.

## v1.1.6

- Removed the legacy `UPDATE_REPOSITORY` compatibility alias from update checks.

## v1.1.5

- Published the updater sidecar as a prebuilt GHCR image and switched Docker Compose to pull it instead of building it on the server.
- Removed the unused Git package from the updater image.
- Fixed the Electron build workflow to run the frontend build from `web/default`.

## v1.1.4

- Changed the updater sidecar to pull prebuilt GHCR images instead of cloning the repository and building Docker images on the server.
- Removed updater repository cache settings and compose volumes that were only needed for server-side builds.

## v1.1.3

- Switched Docker image publishing and deployment defaults from the legacy Docker Hub image to `ghcr.io/artemk1337/new-api-v2`.
- Updated system update defaults to check `artemk1337/new-api-v2`.

## v1.1.2

- Updated the Bun lockfile so Docker production builds pass with the pinned Bun image and frozen lockfile checks.

## v1.1.1

- Switched system update checks from GitHub releases to stable GitHub tags and ignore pre-release tags for latest-version detection.
- Made the updater sidecar opt-in, disabled self-updates by default, and protected updater endpoints with a shared token.
- Added manual installation of a specific tag in the admin UI so operators can roll back to an older tagged version.
- Persisted updater environment settings during self-update so the update flow remains available after restart.
- Changed saturated group 429 errors to English and recommend switching to another group.
- Documented database migration rollback expectations for future schema changes.

## v1.0.0-rc.11

- Added root-only system update checks and tag-based self-update task flow.
- Added updater container support for building selected release tags and redeploying via Docker Compose.
- Added deploy health checks, version verification, and rollback to the previous container version on failed updates.
- Added admin UI controls for checking, starting, and tracking system updates with translated status messages.

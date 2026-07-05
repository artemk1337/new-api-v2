# Changelog

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

# Changelog

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

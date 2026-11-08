# `cmd/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Contains standalone entrypoints for the updater and system telemetry agent. Keep each binary's flags, environment, lifecycle, and image/build inputs local to that command; shared runtime code belongs in reusable packages rather than copied between binaries.

The updater alone accesses the Docker socket. Its build-cache endpoint reads Engine cache usage and prunes unused build cache older than 24 hours; never expand it to prune images, containers, or volumes. Separate buildx builders are outside this endpoint's scope. Cache cleanup and application updates must exclude each other under `updateMu`.

Separate storage endpoints may prune Docker images unused by all containers for at least 30 days and remove top-level entries older than 30 days from `${UPDATER_COMPOSE_DIR}/backups`, while keeping the two newest. Never prune containers or volumes, follow symlinks, or cross filesystems while measuring backups. The updater stores backups locally and does not copy them off-host; see [../docs/backup-storage.md](../docs/backup-storage.md).

Sources: `new-api-updater/main.go`, `system-telemetry-agent/main.go`; tests are adjacent. Run focused package checks when requested.

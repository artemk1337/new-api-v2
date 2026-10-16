# `cmd/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Contains standalone entrypoints for the updater and system telemetry agent. Keep each binary's flags, environment, lifecycle, and image/build inputs local to that command; shared runtime code belongs in reusable packages rather than copied between binaries.

The updater alone accesses the Docker socket. Its build-cache endpoint reads Engine cache usage and prunes unused build cache older than 24 hours; never expand it to prune images, containers, or volumes. Separate buildx builders are outside this endpoint's scope. Cache cleanup and application updates must exclude each other under `updateMu`.

Sources: `new-api-updater/main.go`, `system-telemetry-agent/main.go`; tests are adjacent. Run focused package checks when requested.

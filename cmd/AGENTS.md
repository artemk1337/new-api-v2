# `cmd/` instructions

Parent: [../AGENTS.md](../AGENTS.md).

Contains standalone entrypoints for the updater and system telemetry agent. Keep each binary's flags, environment, lifecycle, and image/build inputs local to that command; shared runtime code belongs in reusable packages rather than copied between binaries.

Sources: `new-api-updater/main.go`, `system-telemetry-agent/main.go`; tests are adjacent. Run focused package checks when requested.

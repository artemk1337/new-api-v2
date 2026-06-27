# Changelog

## v1.0.0-rc.11

- Added root-only system update checks and tag-based self-update task flow.
- Added updater container support for building selected release tags and redeploying via Docker Compose.
- Added deploy health checks, version verification, and rollback to the previous container version on failed updates.
- Added admin UI controls for checking, starting, and tracking system updates with translated status messages.

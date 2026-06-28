---
name: production-image-deploy
description: >-
  Use for production deployment, redeployment, update, rollback, or server
  troubleshooting of new-api/new-api-v2, especially vibecode-api.online or any
  Docker Compose host. Enforces image-only deployment from a registry by
  immutable release tag, mandatory database backup before changes, and a hard
  ban on building images on production servers.
---

# Production Image Deploy

## Non-Negotiable Rule

Never build Docker images on a production server.

Forbidden on production hosts:

- `docker build`
- `docker buildx build`
- `docker compose build`
- `docker compose up --build`
- any `bun install`, `bun run build`, `go build`, or other source build step

Production hosts must only pull already-built images from the configured
registry by an explicit release tag.

## Safe Deployment Flow

1. Verify the target host and service health in read-only mode.
2. Verify the target release tag exists in the image registry.
3. Create a fresh database backup before any runtime change.
4. Verify the backup is readable, for example with `pg_restore -l` for
   PostgreSQL custom-format dumps.
5. Record the current running image and current `.env` version.
6. Update only image/version variables needed by Docker Compose.
7. Pull the target image by tag.
8. Recreate only the application service, never database or Redis:
   `docker compose up -d --no-deps new-api`.
9. Verify container health, public `/api/status`, version, and recent logs.
10. If health/version checks fail, restore the previous image/version variables
    and recreate only `new-api` again.

## Data Safety

- Never remove Docker volumes during deploy.
- Never run `docker compose down -v`.
- Never delete `/root/repos/new-api/data`, `/root/repos/new-api/logs`, or
  database backup files.
- Database containers and Redis should stay running during application updates.

## vibecode-api.online Notes

- Treat `vibecode-api.online` as production.
- The known compose directory is `/root/repos/new-api`.
- The deployment repository is `artemk1337/new-api-v2`.
- The application image must come from a registry tag, not a server-side build.
- If the configured registry image for a release tag does not exist, stop and
  report the blocker instead of building locally.

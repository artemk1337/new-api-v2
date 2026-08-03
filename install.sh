#!/bin/sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$SCRIPT_DIR"

REPOSITORY="${UPDATE_CHECK_REPOSITORY:-artemk1337/new-api-v2}"
APP_IMAGE="${NEW_API_IMAGE:-ghcr.io/artemk1337/new-api-v2}"
UPDATER_IMAGE="${UPDATER_SIDECAR_IMAGE:-ghcr.io/artemk1337/new-api-v2-updater}"
ENV_FILE="${ENV_FILE:-.env}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-new-api}"
START_UPDATER="${START_UPDATER:-true}"
BACKUP_DIR="${BACKUP_DIR:-backups}"
POSTGRES_SERVICE="${POSTGRES_SERVICE:-postgres}"
POSTGRES_USER="${POSTGRES_USER:-root}"
POSTGRES_DB="${POSTGRES_DB:-new-api}"

usage() {
  cat <<'EOF'
Usage:
  ./install.sh [vX.Y.Z]                 Install a release and start the stack.
  ./install.sh update [vX.Y.Z]          Update only new-api with a PostgreSQL backup and rollback.
  ./install.sh update-updater [vX.Y.Z]  Manually update only new-api-updater from the host.
  ./install.sh telemetry-agent         Create or start only the system telemetry agent.
EOF
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

compose() {
  docker compose -p "$COMPOSE_PROJECT_NAME" --env-file "$ENV_FILE" -f "$COMPOSE_FILE" "$@"
}

latest_stable_tag() {
  require_command curl
  require_command sort

  curl -fsSL "https://api.github.com/repos/${REPOSITORY}/git/matching-refs/tags/v" \
    | sed -n 's/.*"ref": "refs\/tags\/\([^"]*\)".*/\1/p' \
    | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' \
    | sort -V \
    | tail -n 1
}

env_value() {
  key="$1"
  if [ -f "$ENV_FILE" ]; then
    sed -n "s/^${key}=//p" "$ENV_FILE" | tail -n 1
  fi
}

random_token() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
    return
  fi
  date +%s | sed 's/.*/update-token-&/'
}

upsert_env() {
  key="$1"
  value="$2"
  tmp="${ENV_FILE}.tmp"

  if [ -f "$ENV_FILE" ]; then
    awk -v key="$key" -v value="$value" '
      BEGIN { done = 0 }
      $0 ~ "^[[:space:]]*" key "=" {
        print key "=" value
        done = 1
        next
      }
      { print }
      END {
        if (!done) {
          print key "=" value
        }
      }
    ' "$ENV_FILE" > "$tmp"
  else
    printf '%s=%s\n' "$key" "$value" > "$tmp"
  fi
  mv "$tmp" "$ENV_FILE"
}

release_version() {
  version="${1:-}"
  if [ -z "$version" ]; then
    version="$(latest_stable_tag)"
  fi
  if ! echo "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
    echo "invalid version: $version" >&2
    echo "expected stable tag format like v1.1.8" >&2
    exit 1
  fi
  printf '%s\n' "$version"
}

backup_database() {
  require_command pg_restore
  if ! compose ps --services --status running | grep -Fx "$POSTGRES_SERVICE" >/dev/null; then
    echo "PostgreSQL service ${POSTGRES_SERVICE} is not running; update stopped before changing anything" >&2
    exit 1
  fi

  mkdir -p "$BACKUP_DIR"
  backup_file="$BACKUP_DIR/new-api-$(date -u +%Y%m%dT%H%M%SZ).dump"
  compose exec -T "$POSTGRES_SERVICE" pg_dump -Fc -U "$POSTGRES_USER" -d "$POSTGRES_DB" > "$backup_file"
  pg_restore -l "$backup_file" >/dev/null
  printf '%s\n' "$backup_file"
}

wait_for_application() {
  require_command curl
  deadline=$(( $(date +%s) + 90 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    container_id="$(compose ps -q new-api)"
    if [ -n "$container_id" ] && [ "$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container_id")" = "healthy" ]; then
      status="$(curl -fsS http://127.0.0.1:3000/api/status || true)"
      reported_version="$(printf '%s' "$status" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n 1)"
      if [ "$reported_version" = "$1" ]; then
        return 0
      fi
    fi
    sleep 3
  done
  return 1
}

wait_for_updater() {
  deadline=$(( $(date +%s) + 30 ))
  while [ "$(date +%s)" -lt "$deadline" ]; do
    container_id="$(compose ps -q new-api-updater)"
    if [ -n "$container_id" ] && [ "$(docker inspect -f '{{.State.Status}}' "$container_id")" = "running" ] && docker exec "$container_id" wget -q -O - http://127.0.0.1:18090/health | grep -qx "ok"; then
      return 0
    fi
    sleep 2
  done
  return 1
}

install_release() {
  version="$(release_version "${1:-}")"
  token="${UPDATE_SIDECAR_TOKEN:-$(env_value UPDATE_SIDECAR_TOKEN)}"
  if [ -z "$token" ]; then
    token="$(random_token)"
  fi

  upsert_env NEW_API_IMAGE "$APP_IMAGE"
  upsert_env NEW_API_VERSION "$version"
  upsert_env COMPOSE_PROJECT_NAME "$COMPOSE_PROJECT_NAME"
  upsert_env UPDATER_SIDECAR_IMAGE "$UPDATER_IMAGE"
  upsert_env UPDATER_SIDECAR_VERSION "$version"
  upsert_env UPDATER_HOST_COMPOSE_DIR "$SCRIPT_DIR"
  upsert_env UPDATE_CHECK_REPOSITORY "$REPOSITORY"
  upsert_env UPDATE_SIDECAR_TOKEN "$token"

  if [ "$START_UPDATER" = "true" ]; then
    compose --profile updater up -d
  else
    compose up -d
  fi
  echo "Installed ${APP_IMAGE}:${version}"
}

update_application() {
  version="$(release_version "${1:-}")"
  require_command pg_restore
  require_command curl
  upsert_env UPDATER_HOST_COMPOSE_DIR "$SCRIPT_DIR"
  compose config -q
  docker manifest inspect "${APP_IMAGE}:${version}" >/dev/null

  backup_file="$(backup_database)"
  env_backup="${backup_file}.env"
  cp "$ENV_FILE" "$env_backup"

  rollback() {
    cp "$env_backup" "$ENV_FILE"
    rm -f "$env_backup"
    compose pull new-api || true
    compose up -d --no-deps new-api || true
    echo "Update failed. Restored the previous image configuration; database backup: ${backup_file}" >&2
    exit 1
  }

  upsert_env NEW_API_IMAGE "$APP_IMAGE"
  upsert_env NEW_API_VERSION "$version"
  compose pull new-api || rollback
  compose up -d --no-deps new-api || rollback
  wait_for_application "$version" || rollback
  rm -f "$env_backup"
  echo "Updated ${APP_IMAGE}:${version}; database backup: ${backup_file}"
}

update_updater() {
  version="$(release_version "${1:-}")"
  upsert_env UPDATER_HOST_COMPOSE_DIR "$SCRIPT_DIR"
  compose config -q
  docker manifest inspect "${UPDATER_IMAGE}:${version}" >/dev/null

  env_backup="${ENV_FILE}.updater-backup"
  cp "$ENV_FILE" "$env_backup"
  rollback() {
    cp "$env_backup" "$ENV_FILE"
    compose pull new-api-updater || true
    compose up -d --no-deps new-api-updater || true
    rm -f "$env_backup"
    echo "Updater update failed; restored the previous image configuration" >&2
    exit 1
  }

  upsert_env UPDATER_SIDECAR_IMAGE "$UPDATER_IMAGE"
  upsert_env UPDATER_SIDECAR_VERSION "$version"
  compose pull new-api-updater || rollback
  compose up -d --no-deps new-api-updater || rollback
  wait_for_updater || rollback
  rm -f "$env_backup"
  echo "Updated ${UPDATER_IMAGE}:${version}"
}

ensure_telemetry_agent() {
  telemetry_image="${SYSTEM_TELEMETRY_AGENT_IMAGE:-$(env_value SYSTEM_TELEMETRY_AGENT_IMAGE)}"
  telemetry_version="${SYSTEM_TELEMETRY_AGENT_VERSION:-$(env_value SYSTEM_TELEMETRY_AGENT_VERSION)}"
  if [ -z "$telemetry_image" ]; then
    telemetry_image="ghcr.io/artemk1337/new-api-v2-system-telemetry-agent"
  fi
  if [ -z "$telemetry_version" ]; then
    telemetry_version="latest"
  fi

  compose config -q
  docker manifest inspect "${telemetry_image}:${telemetry_version}" >/dev/null
  compose pull system-telemetry-agent
  compose up -d --no-deps system-telemetry-agent

  container_id="$(compose ps -q system-telemetry-agent)"
  if [ -z "$container_id" ] || [ "$(docker inspect -f '{{.State.Status}}' "$container_id")" != "running" ]; then
    echo "system-telemetry-agent did not start" >&2
    exit 1
  fi
  echo "Started ${telemetry_image}:${telemetry_version} (system-telemetry-agent only)"
}

require_command docker

case "${1:-}" in
  update)
    update_application "${2:-}"
    ;;
  update-updater)
    update_updater "${2:-}"
    ;;
  telemetry-agent)
    ensure_telemetry_agent
    ;;
  -h|--help)
    usage
    ;;
  *)
    install_release "${1:-}"
    ;;
esac

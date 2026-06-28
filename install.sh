#!/bin/sh
set -eu

REPOSITORY="${UPDATE_CHECK_REPOSITORY:-artemk1337/new-api-v2}"
APP_IMAGE="${NEW_API_IMAGE:-ghcr.io/artemk1337/new-api-v2}"
UPDATER_IMAGE="${UPDATER_SIDECAR_IMAGE:-ghcr.io/artemk1337/new-api-v2-updater}"
ENV_FILE="${ENV_FILE:-.env}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.yml}"
VERSION="${1:-${NEW_API_VERSION:-}}"
UPDATE_ENABLED_VALUE="${UPDATE_ENABLED:-true}"
START_UPDATER="${START_UPDATER:-$UPDATE_ENABLED_VALUE}"

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
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

require_command docker

if [ -z "$VERSION" ]; then
  VERSION="$(latest_stable_tag)"
fi

if ! echo "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "invalid version: $VERSION" >&2
  echo "expected stable tag format like v1.1.8" >&2
  exit 1
fi

token="${UPDATE_SIDECAR_TOKEN:-$(env_value UPDATE_SIDECAR_TOKEN)}"
if [ -z "$token" ]; then
  token="$(random_token)"
fi

upsert_env NEW_API_IMAGE "$APP_IMAGE"
upsert_env NEW_API_VERSION "$VERSION"
upsert_env UPDATER_SIDECAR_IMAGE "$UPDATER_IMAGE"
upsert_env UPDATER_SIDECAR_VERSION "$VERSION"
upsert_env UPDATE_CHECK_REPOSITORY "$REPOSITORY"
upsert_env UPDATE_ENABLED "$UPDATE_ENABLED_VALUE"
upsert_env UPDATE_SIDECAR_TOKEN "$token"

if [ "$START_UPDATER" = "true" ]; then
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" --profile updater up -d
else
  docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d
fi

echo "Installed ${APP_IMAGE}:${VERSION}"

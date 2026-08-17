#!/usr/bin/env bash
set -euo pipefail

# Заполняет локальную SQLite-БД одной безопасной синтетической записью журнала
# для ручной проверки деталей биллинга. Скрипт не трогает реальные записи:
# используются отдельные имена модели и request_id.
db_path="${1:-${SQLITE_PATH:-one-api.db}}"
db_path="${db_path%%\?*}"

if ! command -v sqlite3 >/dev/null 2>&1; then
  echo "sqlite3 is required" >&2
  exit 1
fi
if [[ ! -f "$db_path" ]]; then
  echo "SQLite database not found: $db_path (start the API once first)" >&2
  exit 1
fi

sqlite3 "$db_path" <<'SQL'
PRAGMA foreign_keys = OFF;
BEGIN;

-- Demo model/channel/ability are idempotent and isolated from user data.
INSERT OR REPLACE INTO models
  (id, model_name, description, status, sync_official, created_time, updated_time, name_rule)
VALUES
  (990001, 'billing-demo-model', 'Synthetic model for billing details demo', 1, 0,
   strftime('%s','now'), strftime('%s','now'), 0);

INSERT OR REPLACE INTO channels
  (id, type, key, status, name, created_time, models, "group", used_quota)
VALUES
  (990001, 1, 'billing-demo-key', 1, 'Billing demo channel', strftime('%s','now'),
   'billing-demo-model', 'default', 0);

INSERT OR REPLACE INTO abilities ("group", model, channel_id, enabled, weight)
VALUES ('default', 'billing-demo-model', 990001, 1, 1);

-- Keep the ratio settings available to the pricing cache and UI.
INSERT INTO options (key, value) VALUES
  ('ModelRatio', '{"billing-demo-model":1.25}'),
  ('CompletionRatio', '{"billing-demo-model":2.0}'),
  ('CacheRatio', '{"billing-demo-model":0.25}'),
  ('CreateCacheRatio', '{"billing-demo-model":1.25}'),
  ('GroupRatio', '{"default":0.8}')
ON CONFLICT(key) DO UPDATE SET value = json_patch(COALESCE(options.value, '{}'), excluded.value);

DELETE FROM logs WHERE request_id = 'billing-demo-request-0001';
INSERT INTO logs
  (user_id, created_at, type, content, username, token_name, model_name, quota,
   prompt_tokens, completion_tokens, use_time, is_stream, channel, token_id,
   "group", ip, request_id, other)
VALUES
  (1, strftime('%s','now'), 2, 'Synthetic billing demo', 'demo-user', 'Demo key',
   'billing-demo-model', 3288, 1200, 800, 41000, 1, 990001, 990001, 'default',
   '127.0.0.1', 'billing-demo-request-0001',
   '{
     "model_ratio":1.25,
     "completion_ratio":2.0,
     "group_ratio":0.8,
     "user_group_ratio":1.0,
     "model_price":0,
     "cache_tokens":300,
     "cache_ratio":0.25,
     "cache_creation_tokens":160,
     "cache_creation_ratio":1.25,
     "cache_creation_tokens_5m":100,
     "cache_creation_ratio_5m":1.25,
     "cache_creation_tokens_1h":60,
     "cache_creation_ratio_1h":1.5,
     "cache_write_tokens":160,
     "input_tokens_total":1200,
     "billing_source":"wallet",
     "billing_preference":"quota",
     "request_path":"/v1/chat/completions",
     "frt":77
   }');

COMMIT;
SQL

echo "Seeded billing demo in $db_path (request_id=billing-demo-request-0001)"

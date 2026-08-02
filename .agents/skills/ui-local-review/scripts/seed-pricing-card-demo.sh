#!/usr/bin/env bash

set -euo pipefail

# Seeds only the local docker-compose.dev.yml PostgreSQL volume.
# Current price is $1.5 / $4.5; crossed-out base price is $10 / $30 per 1M.

docker compose -f docker-compose.dev.yml exec -T postgres psql -U root -d new-api <<'SQL'
BEGIN;

DELETE FROM abilities
WHERE model IN ('claude-opus-4-8', 'gpt-5.4', 'kimi-k2');

DELETE FROM channels
WHERE name = 'Pricing card UI review demo';

WITH demo_channel AS (
  INSERT INTO channels (type, key, status, name, models, "group", base_url)
  VALUES (
    1,
    'pricing-card-ui-review-demo',
    1,
    'Pricing card UI review demo',
    'claude-opus-4-8,gpt-5.4,kimi-k2',
    'default',
    'https://example.invalid'
  )
  RETURNING id
)
INSERT INTO abilities ("group", model, channel_id, enabled, priority, weight)
SELECT 'default', model, demo_channel.id, true, 0, 0
FROM demo_channel
CROSS JOIN (
  VALUES ('claude-opus-4-8'), ('gpt-5.4'), ('kimi-k2')
) AS demo_models(model);

INSERT INTO options (key, value)
VALUES
  (
    'ModelRatio',
    '{"preview-card-model":1,"claude-opus-4-8":5,"gpt-5.4":5,"kimi-k2":5}'
  ),
  (
    'CompletionRatio',
    '{"claude-opus-4-8":3,"gpt-5.4":3,"kimi-k2":3}'
  ),
  (
    'PricingGroups',
    '[{"id":1,"name":"default","ratio":0.15,"selectable":true,"description":"UI review demo"}]'
  ),
  (
    'billing_setting.billing_mode',
    '{"claude-opus-4-8":"tiered_expr","gpt-5.4":"tiered_expr","kimi-k2":"tiered_expr"}'
  ),
  (
    'billing_setting.billing_expr',
    '{"claude-opus-4-8":"tier(\"base\", p * 10 + c * 30)","gpt-5.4":"tier(\"base\", p * 10 + c * 30)","kimi-k2":"tier(\"base\", p * 10 + c * 30)"}'
  )
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value;

COMMIT;
SQL

docker compose -f docker-compose.dev.yml restart new-api

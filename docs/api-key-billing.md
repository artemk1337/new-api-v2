# Balance and usage with an API key

Use the same API key as for model requests. A dashboard session and the
`New-Api-User` header are not required. These requests do not call a model or
consume credits.

## Account balance

```sh
curl https://vibecode-api.online/v1/credits \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "data": {
    "total_credits": 100,
    "total_usage": 25,
    "balance": 75,
    "currency": "USD",
    "request_count": 42
  }
}
```

- `balance` is the current wallet balance of the key's owner.
- `total_usage` is the lifetime account usage shown in the dashboard, including
  wallet and subscription funding.
- `total_credits` is the accounting total `balance + total_usage`. It is not the
  sum of paid deposits: grants, adjustments and subscriptions affect accounting.
- `request_count` is the dashboard's count of accounted requests, rather than
  all incoming HTTP requests.

Subscription allowances are separate from the wallet and are not included in
`balance`. An API key gives access to these financial statistics of its owner;
keep it private when sharing access to an account.

## Current key limits and usage

```sh
curl https://vibecode-api.online/v1/key \
  -H "Authorization: Bearer $API_KEY"
```

```json
{
  "data": {
    "label": "My app",
    "limit": 50,
    "limit_remaining": 40,
    "usage": 10,
    "unlimited_quota": false,
    "expires_at": null,
    "currency": "USD"
  }
}
```

`usage` belongs only to the current API key. `limit` is its remaining allowance
plus its accumulated usage. `limit_remaining` is the key allowance, which may
exceed the available wallet balance. Unlimited keys return JSON `null` for both
limit fields; they still use the owner's wallet or subscription allowance.
`expires_at` is a UTC RFC 3339 timestamp, or `null` for a key without expiration.
The API key secret is never returned.

## Compatibility and access

The routes also accept `/api/v1/credits` and `/api/v1/key` for clients using an
OpenRouter-style base URL. The core `data.total_credits`, `data.total_usage`,
`data.label`, `data.limit`, `data.limit_remaining` and `data.usage` fields follow
the shape of [OpenRouter credits](https://openrouter.ai/docs/api/api-reference/credits/get-credits)
and [current key](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-key).
Account credits are available to ordinary API keys here; OpenRouter restricts
its credits endpoint to management keys.

All monetary values are always USD, converted using the configured quota per
unit, regardless of the dashboard's currency, token display or
`DisplayTokenStatEnabled` setting. These are lifetime counters, without date
filters. Their freshness matches the dashboard: pending accounting batches can
delay usage and request counts until the next flush (normally five seconds).

Keys with zero remaining quota can query statistics. Disabled, expired and
deleted keys are rejected with HTTP 401. Blocked accounts and requests outside
the key's IP allowlist receive HTTP 403. Missing or invalid credentials receive
HTTP 401; API rate limits still apply. Responses use `Cache-Control: no-store`.

The existing `/api/usage/token` endpoint and legacy dashboard billing endpoints
retain their response formats and authentication behavior.

Other provider conventions were considered, including
[DeepSeek's balance endpoint](https://api-docs.deepseek.com/api/get-user-balance).
The OpenRouter-style responses expose both account and key usage without
inventing separate granted and topped-up wallet balances.

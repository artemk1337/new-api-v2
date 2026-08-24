import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'
import { getTokenBillingParts } from './token-billing'

const log: UsageLog = {
  id: 1,
  user_id: 1,
  created_at: 0,
  type: 2,
  content: '',
  username: '',
  token_name: '',
  model_name: 'claude-haiku-4-5-20251001',
  quota: 0,
  prompt_tokens: 5,
  completion_tokens: 11,
  use_time: 0,
  is_stream: false,
  channel: 19,
  channel_name: 'Kiro-Claude-1',
  token_id: 1,
  group: 'Kiro-Claude-1',
  ip: '',
  other: '',
  request_id: '',
  upstream_request_id: '',
}

const cacheUsage: LogOtherData = {
  cache_tokens: 22,
  cache_creation_tokens: 240,
  cache_creation_tokens_5m: 240,
}

describe('usage log token billing parts', () => {
  it('keeps Claude text input separate from cache tokens', () => {
    const parts = getTokenBillingParts(log, { ...cacheUsage, claude: true })

    assert.equal(parts.baseInput, 5)
    assert.equal(parts.cacheRead, 22)
    assert.equal(parts.cacheWrite5m, 240)
  })

  it('subtracts cache categories from non-Claude total input', () => {
    const nonClaudeLog = { ...log, prompt_tokens: 267 }
    const parts = getTokenBillingParts(nonClaudeLog, cacheUsage)

    assert.equal(parts.baseInput, 5)
  })
})

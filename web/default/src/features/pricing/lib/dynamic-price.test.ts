import assert from 'node:assert/strict'
import { test } from 'node:test'

import type { PricingModel } from '../types'
import { getDynamicPricingSummary } from './dynamic-price'

test('uses the first tier and keeps base/current prices for multi-tier expressions', () => {
  const model: PricingModel = {
    id: 1,
    model_name: 'tiered',
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups: ['standard'],
    group_ratio: { standard: 0.5 },
    billing_mode: 'tiered_expr',
    billing_expr:
      'len <= 1000 ? tier("step_1", p * 1 + c * 2) : tier("step_2", p * 3 + c * 4)',
  }
  const summary = getDynamicPricingSummary(model, {
    tokenUnit: 'M',
    groupRatioMultiplier: 0.5,
  })
  assert.equal(summary?.tierCount, 2)
  assert.deepEqual(
    summary?.primaryEntries.map((entry) => entry.key),
    ['p', 'c']
  )
  assert.deepEqual(
    summary?.baseEntries.map((entry) => entry.value),
    [1, 2]
  )
  assert.deepEqual(
    summary?.entries.map((entry) => entry.value),
    [1, 2]
  )
})

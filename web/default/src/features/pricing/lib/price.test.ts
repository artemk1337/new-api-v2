import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PricingModel } from '../types'
import { getDisplayGroupRatio } from './price'

const model = (overrides: Partial<PricingModel> = {}): PricingModel => ({
  id: 1,
  model_name: 'test',
  quota_type: 0,
  model_ratio: 1,
  completion_ratio: 2,
  enable_groups: ['standard', 'discount'],
  group_ratio: { standard: 1, discount: 0.5 },
  ...overrides,
})

describe('getDisplayGroupRatio', () => {
  test('uses minimum enabled ratio without a group filter', () => {
    assert.equal(getDisplayGroupRatio(model()), 0.5)
  })

  test('uses the explicitly selected group', () => {
    assert.equal(getDisplayGroupRatio(model(), 'standard'), 1)
  })

  test('preserves an explicit zero ratio', () => {
    assert.equal(
      getDisplayGroupRatio(
        model({ group_ratio: { standard: 1, discount: 0 } })
      ),
      0
    )
  })

  test('uses request model groups identically', () => {
    assert.equal(
      getDisplayGroupRatio(
        model({ quota_type: 1, group_ratio: { standard: 0.8 } })
      ),
      0.8
    )
  })
})

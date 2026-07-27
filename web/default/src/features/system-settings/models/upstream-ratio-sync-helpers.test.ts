/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildPricingMapDiffPatches,
  buildPricingSyncPatches,
} from './upstream-ratio-sync-helpers.ts'

const ratioOptionKeys = [
  'ModelRatio',
  'CompletionRatio',
  'CacheRatio',
  'CreateCacheRatio',
  'ImageRatio',
  'AudioRatio',
  'AudioCompletionRatio',
]

describe('pricing sync category patches', () => {
  test('fixed price removes ratio and tiered contracts', () => {
    const patches = buildPricingSyncPatches({
      'model-a': { model_price: 0.25 },
    })

    assert.deepEqual(patches.ModelPrice.set, { 'model-a': 0.25 })
    for (const key of [
      ...ratioOptionKeys,
      'billing_setting.billing_mode',
      'billing_setting.billing_expr',
    ]) {
      assert.deepEqual(patches[key].delete, ['model-a'])
    }
  })

  test('ratio price removes fixed and tiered contracts', () => {
    const patches = buildPricingSyncPatches({
      'model-a': { model_ratio: '2.5', completion_ratio: 6 },
    })

    assert.deepEqual(patches.ModelRatio.set, { 'model-a': 2.5 })
    assert.deepEqual(patches.CompletionRatio.set, { 'model-a': 6 })
    assert.deepEqual(patches.ModelPrice.delete, ['model-a'])
    assert.deepEqual(patches['billing_setting.billing_mode'].delete, [
      'model-a',
    ])
    assert.deepEqual(patches['billing_setting.billing_expr'].delete, [
      'model-a',
    ])
  })

  test('tiered expression removes fixed and ratio contracts', () => {
    const patches = buildPricingSyncPatches({
      'model-a': {
        billing_mode: 'tiered_expr',
        billing_expr: 'input_tokens * 2',
      },
    })

    assert.deepEqual(patches.ModelPrice.delete, ['model-a'])
    for (const key of ratioOptionKeys) {
      assert.deepEqual(patches[key].delete, ['model-a'])
    }
    assert.deepEqual(patches['billing_setting.billing_mode'].set, {
      'model-a': 'tiered_expr',
    })
    assert.deepEqual(patches['billing_setting.billing_expr'].set, {
      'model-a': 'input_tokens * 2',
    })
  })
})

describe('model editor pricing patches', () => {
  const emptyPricing = {
    ModelPrice: '{}',
    ModelRatio: '{}',
    CacheRatio: '{}',
    CreateCacheRatio: '{}',
    CompletionRatio: '{}',
    ImageRatio: '{}',
    AudioRatio: '{}',
    AudioCompletionRatio: '{}',
    BillingMode: '{}',
    BillingExpr: '{}',
  }

  test('combines changes across pricing maps in one patch request', () => {
    const patches = buildPricingMapDiffPatches(
      {
        ...emptyPricing,
        ModelPrice: '{"old-model":0.1}',
        ModelRatio: '{"model-a":1}',
      },
      {
        ...emptyPricing,
        ModelPrice: '{"new-model":0.2}',
        ModelRatio: '{"model-a":2}',
      }
    )

    assert.deepEqual(patches.ModelPrice, {
      set: { 'new-model': 0.2 },
      delete: ['old-model'],
    })
    assert.deepEqual(patches.ModelRatio, { set: { 'model-a': 2 } })
  })

  test('does not emit unchanged pricing maps', () => {
    assert.deepEqual(buildPricingMapDiffPatches(emptyPricing, emptyPricing), {})
  })
})

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
  getDisplaySyncFields,
  getPricingSyncErrorMessage,
  getRunnablePricingSyncSources,
  isPricingSyncVersionConflict,
  pricingSyncPreferencesForModels,
  rebasePricingSyncConfigDraft,
  splitPricingSyncDifferences,
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
  test('extracts pricing sync errors with the expected precedence', () => {
    assert.equal(
      getPricingSyncErrorMessage(
        { response: { data: { message: 'server message' } } },
        'fallback'
      ),
      'server message'
    )
    assert.equal(
      getPricingSyncErrorMessage(new Error('request failed'), 'fallback'),
      'request failed'
    )
    assert.equal(
      getPricingSyncErrorMessage(
        { response: { data: { message: '   ' } } },
        'fallback'
      ),
      'fallback'
    )
    assert.equal(getPricingSyncErrorMessage(null, 'fallback'), 'fallback')
  })

  test('detects only pricing sync version conflicts', () => {
    assert.equal(
      isPricingSyncVersionConflict({ response: { status: 409 } }),
      true
    )
    assert.equal(
      isPricingSyncVersionConflict({ response: { status: 400 } }),
      false
    )
    assert.equal(isPricingSyncVersionConflict(new Error('conflict')), false)
    assert.equal(isPricingSyncVersionConflict(null), false)
  })

  test('returns only persisted sources that can run automatically', () => {
    const channels = [
      { id: 1, name: 'runnable', base_url: '', status: 1 },
      { id: 2, name: 'disabled source', base_url: '', status: 1 },
      { id: 3, name: 'manual source', base_url: '', status: 1 },
      { id: 4, name: 'disabled channel', base_url: '', status: 2 },
      { id: 5, name: 'not configured', base_url: '', status: 1 },
    ]
    const config = {
      strategy: 'highest' as const,
      version: 7,
      sources: [
        { channel_id: 1, enabled: true, endpoint: '', interval_seconds: 60 },
        { channel_id: 2, enabled: false, endpoint: '', interval_seconds: 60 },
        { channel_id: 3, enabled: true, endpoint: '', interval_seconds: 0 },
        { channel_id: 4, enabled: true, endpoint: '', interval_seconds: 60 },
      ],
    }

    assert.deepEqual(getRunnablePricingSyncSources(channels, config), [
      { id: 1, name: 'runnable' },
    ])
  })

  test('does not expose automatic sources before persisted config loads', () => {
    const channels = [{ id: 1, name: 'channel', base_url: '', status: 1 }]

    assert.deepEqual(getRunnablePricingSyncSources(channels), [])
  })

  test('rebases only dirty pricing sync fields onto the latest config', () => {
    const base = {
      strategy: 'highest' as const,
      version: 1,
      sources: [
        {
          channel_id: 1,
          enabled: true,
          endpoint: '/old',
          interval_seconds: 60,
        },
        {
          channel_id: 2,
          enabled: true,
          endpoint: '/two',
          interval_seconds: 60,
        },
      ],
    }
    const draft = {
      strategy: 'lowest' as const,
      version: 1,
      sources: [
        {
          channel_id: 1,
          enabled: true,
          endpoint: '/draft',
          interval_seconds: 60,
        },
        {
          channel_id: 2,
          enabled: true,
          endpoint: '/two',
          interval_seconds: 60,
        },
      ],
    }
    const latest = {
      strategy: 'average' as const,
      version: 9,
      sources: [
        {
          channel_id: 1,
          enabled: false,
          endpoint: '/latest',
          interval_seconds: 120,
        },
        {
          channel_id: 2,
          enabled: false,
          endpoint: '/latest-two',
          interval_seconds: 30,
        },
      ],
    }

    assert.deepEqual(rebasePricingSyncConfigDraft(base, draft, latest), {
      strategy: 'lowest',
      version: 9,
      sources: [
        {
          channel_id: 1,
          enabled: false,
          endpoint: '/draft',
          interval_seconds: 120,
        },
        {
          channel_id: 2,
          enabled: false,
          endpoint: '/latest-two',
          interval_seconds: 30,
        },
      ],
    })
  })

  test('preserves local source additions and removals while rebasing', () => {
    const base = {
      strategy: 'highest' as const,
      version: 1,
      sources: [
        {
          channel_id: 1,
          enabled: true,
          endpoint: '/one',
          interval_seconds: 60,
        },
      ],
    }
    const draft = {
      ...base,
      sources: [
        {
          channel_id: 2,
          enabled: true,
          endpoint: '/two',
          interval_seconds: 30,
        },
      ],
    }
    const latest = {
      strategy: 'average' as const,
      version: 2,
      sources: [
        {
          channel_id: 1,
          enabled: false,
          endpoint: '/latest',
          interval_seconds: 120,
        },
        {
          channel_id: 3,
          enabled: true,
          endpoint: '/three',
          interval_seconds: 90,
        },
      ],
    }

    assert.deepEqual(rebasePricingSyncConfigDraft(base, draft, latest), {
      strategy: 'average',
      version: 2,
      sources: [
        {
          channel_id: 3,
          enabled: true,
          endpoint: '/three',
          interval_seconds: 90,
        },
        {
          channel_id: 2,
          enabled: true,
          endpoint: '/two',
          interval_seconds: 30,
        },
      ],
    })
  })

  test('separates manually protected differences from automatic models', () => {
    const differences = {
      automatic: { model_price: { current: 1, upstreams: {}, confidence: {} } },
      manual: { model_price: { current: 2, upstreams: {}, confidence: {} } },
    }

    const groups = splitPricingSyncDifferences(differences, {
      manual: {
        model_name: 'manual',
        mode: 'manual',
        channel_id: 0,
        status: 'ready',
      },
    })

    assert.deepEqual(Object.keys(groups.automatic), ['automatic'])
    assert.deepEqual(Object.keys(groups.manual), ['manual'])
  })

  test('keeps current automatic preference when applying selected prices', () => {
    const preferences = pricingSyncPreferencesForModels(
      ['general', 'channel', 'missing'],
      {
        general: {
          model_name: 'general',
          mode: 'general',
          channel_id: 0,
          status: 'ready',
        },
        channel: {
          model_name: 'channel',
          mode: 'channel',
          channel_id: 7,
          status: 'ready',
        },
      }
    )

    assert.deepEqual(
      preferences.map(({ model_name, mode, channel_id }) => ({
        model_name,
        mode,
        channel_id,
      })),
      [
        { model_name: 'general', mode: 'general', channel_id: 0 },
        { model_name: 'channel', mode: 'channel', channel_id: 7 },
        { model_name: 'missing', mode: 'general', channel_id: 0 },
      ]
    )
  })

  test('hides derived billing mode when expression pricing is available', () => {
    const fields = getDisplaySyncFields({
      billing_mode: { current: null, upstreams: {}, confidence: {} },
      billing_expr: { current: null, upstreams: {}, confidence: {} },
    })

    assert.deepEqual(fields, ['billing_expr'])
  })

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

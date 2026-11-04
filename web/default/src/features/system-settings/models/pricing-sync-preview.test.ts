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

import type { ModelPricingSnapshot } from './model-pricing-snapshots'
import { getPricingSyncPricePreview } from './pricing-sync-preview'

const snapshot: ModelPricingSnapshot = {
  name: 'model',
  ratio: '0.5',
  hasConflict: false,
}
const audioDifferences = {
  audio_ratio: { current: null, upstreams: { source: 0.93 }, confidence: {} },
  audio_completion_ratio: {
    current: null,
    upstreams: { source: 1.87 },
    confidence: {},
  },
}

describe('pricing sync monetary preview', () => {
  test('uses defaults for empty optional snapshot fields and previews the final selected tariff', () => {
    const result = getPricingSyncPricePreview(
      { ...snapshot, audioRatio: '', audioCompletionRatio: '' },
      {},
      { audio_ratio: 0.93, audio_completion_ratio: 1.87 },
      'audio_completion_ratio',
      null
    )
    assert.equal(result.current, 1)
    assert.ok(result.proposed !== null)
    assert.ok(Math.abs(result.proposed - 1.7391) < 1e-10)
  })
  test('respects configured quota units instead of assuming a fixed conversion', () => {
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        audioDifferences,
        {},
        'audio_ratio',
        'source',
        1_000_000
      ),
      { current: 0.5, proposed: 0.465 }
    )
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        audioDifferences,
        {},
        'audio_ratio',
        'source',
        0
      ),
      { current: null, proposed: null }
    )
  })
  test('combines selected input audio pricing with the candidate output audio rate', () => {
    const result = getPricingSyncPricePreview(
      snapshot,
      audioDifferences,
      { audio_ratio: 0.93 },
      'audio_completion_ratio',
      'source'
    )
    assert.equal(result.current, 1)
    assert.ok(result.proposed !== null)
    assert.ok(Math.abs(result.proposed - 1.7391) < 1e-10)
  })

  test('previews only the candidate field when no related field is selected', () => {
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        audioDifferences,
        {},
        'audio_completion_ratio',
        'source'
      ),
      { current: 1, proposed: 1.87 }
    )
  })

  test('uses the current base input price until a new base is selected', () => {
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        audioDifferences,
        { model_ratio: 2 },
        'audio_ratio',
        'source'
      ),
      { current: 1, proposed: 3.72 }
    )
  })

  test('does not invent a missing base price or missing cache multiplier', () => {
    assert.deepEqual(
      getPricingSyncPricePreview(
        undefined,
        audioDifferences,
        {},
        'audio_ratio',
        'source'
      ),
      { current: null, proposed: null }
    )
    const cache = {
      cache_ratio: {
        current: null,
        upstreams: { source: 0.1 },
        confidence: {},
      },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(snapshot, cache, {}, 'cache_ratio', 'source'),
      { current: null, proposed: 0.1 }
    )
  })

  test('preserves explicit zero rates and rejects non-finite values', () => {
    const diff = {
      model_ratio: { current: 0, upstreams: { source: 0 }, confidence: {} },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(snapshot, diff, {}, 'model_ratio', 'source'),
      { current: 0, proposed: 0 }
    )
    const invalid = {
      audio_ratio: { current: 1, upstreams: { source: 'NaN' }, confidence: {} },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        invalid,
        {},
        'audio_ratio',
        'source'
      ),
      { current: 1, proposed: null }
    )
  })

  test('uses the existing raw multiplier for upstream same sentinels', () => {
    const diff = {
      audio_ratio: {
        current: 0.8,
        upstreams: { source: 'same' },
        confidence: {},
      },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(snapshot, diff, {}, 'audio_ratio', 'source'),
      { current: 0.8, proposed: 0.8 }
    )
  })

  test('does not present token rates for a fixed or expression based current tariff', () => {
    for (const billingMode of ['per-request', 'tiered_expr']) {
      const fixed = { ...snapshot, billingMode, price: '0.25' }
      assert.deepEqual(
        getPricingSyncPricePreview(
          fixed,
          audioDifferences,
          {},
          'audio_ratio',
          'source'
        ),
        { current: null, proposed: 0.93 }
      )
      assert.deepEqual(
        getPricingSyncPricePreview(
          fixed,
          audioDifferences,
          { model_price: 0.2 },
          'audio_ratio',
          'source'
        ),
        { current: null, proposed: null }
      )
    }
  })

  test('shows fixed prices per request and does not evaluate expressions', () => {
    const diff = {
      model_price: {
        current: 0.25,
        upstreams: { source: 0.2 },
        confidence: {},
      },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(
        { ...snapshot, price: '0.25', billingMode: 'per-request' },
        diff,
        {},
        'model_price',
        'source'
      ),
      { current: 0.25, proposed: 0.2 }
    )
    const expression = {
      billing_expr: {
        current: null,
        upstreams: { source: 'input_tokens * 2' },
        confidence: {},
      },
    }
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        expression,
        {},
        'billing_expr',
        'source'
      ),
      { current: null, proposed: null }
    )
  })

  test('does not show a proposed price when this source has no value', () => {
    assert.deepEqual(
      getPricingSyncPricePreview(
        snapshot,
        audioDifferences,
        {},
        'audio_ratio',
        'missing'
      ),
      { current: 1, proposed: null }
    )
  })
})

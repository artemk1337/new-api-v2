import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { reconcilePricingSyncSourceDraft } from './pricing-sync-source-draft.ts'

describe('pricing sync source draft', () => {
  test('uses cached server state when switching away from a dirty model', () => {
    const next = reconcilePricingSyncSourceDraft(
      { modelName: 'model-a', value: 'channel:1', dirty: true },
      'model-b',
      'channel:2'
    )

    assert.deepEqual(next, {
      modelName: 'model-b',
      value: 'channel:2',
      dirty: false,
    })
  })

  test('does not overwrite a dirty draft for the same model', () => {
    const current = {
      modelName: 'model-a',
      value: 'channel:2',
      dirty: true,
    }

    assert.equal(
      reconcilePricingSyncSourceDraft(current, 'model-a', 'channel:1'),
      current
    )
  })
})

import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { getGroupFallback } from './playground-option-utils'

describe('playground option utilities', () => {
  test('uses id-based default group fallback', () => {
    const fallback = getGroupFallback(
      [
        { label: 'VIP', value: '2', ratio: 1 },
        { label: 'default', value: '1', ratio: 1 },
      ],
      'legacy-default'
    )

    assert.equal(fallback, '1')
  })
})

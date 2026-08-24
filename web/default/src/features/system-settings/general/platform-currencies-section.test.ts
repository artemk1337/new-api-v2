import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getSupportedSyncProviders,
  getSyncIntervalLabel,
} from './platform-currencies-sync'

describe('platform currency synchronization interval', () => {
  test('uses readable labels for supported intervals', () => {
    const translate = (key: string) => `translated:${key}`

    assert.equal(
      getSyncIntervalLabel('hour', translate),
      'translated:Every hour'
    )
  })

  test('preserves an unknown backend interval for a recoverable UI', () => {
    assert.equal(getSyncIntervalLabel('week', (key) => key), 'week')
  })
})

describe('platform currency synchronization providers', () => {
  test('only exposes USD-compatible sources for each currency', () => {
    assert.deepEqual(getSupportedSyncProviders('RUB'), ['cbr'])
    assert.deepEqual(getSupportedSyncProviders('USDT'), ['coingecko'])
    assert.deepEqual(getSupportedSyncProviders('XYZ'), [])
  })
})

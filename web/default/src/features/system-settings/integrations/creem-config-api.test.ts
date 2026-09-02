import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildPaymentSettingsPayload,
  getPaymentSettingsSaveErrorMessage,
} from './creem-config-api'

describe('Payment settings save errors', () => {
  test('keeps the API error message for user feedback', () => {
    assert.equal(
      getPaymentSettingsSaveErrorMessage(new Error('Invalid currency')),
      'Invalid currency'
    )
  })

  test('uses a fallback for non-error rejections', () => {
    assert.equal(
      getPaymentSettingsSaveErrorMessage(null, 'Save failed'),
      'Save failed'
    )
  })
})

describe('Payment settings request flow', () => {
  test('sends generic options', () => {
    const options = [{ key: 'MinTopUp', value: '12.5' }]
    assert.deepEqual(buildPaymentSettingsPayload(options), { options })
  })
})

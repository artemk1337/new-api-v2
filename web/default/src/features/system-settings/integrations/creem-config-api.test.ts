import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  buildPaymentSettingsPayload,
  getPaymentSettingsSaveErrorMessage,
  shouldUpdateCreemSecret,
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

describe('Creem secret updates', () => {
  test('omits an untouched masked secret', () => {
    assert.equal(shouldUpdateCreemSecret('********', '********', false), false)
    assert.equal(shouldUpdateCreemSecret('', '', false), false)
  })

  test('sends an explicitly cleared secret when the form starts empty', () => {
    assert.equal(shouldUpdateCreemSecret('', '', false, true), true)
  })

  test('sends a replacement secret only when the field is dirty', () => {
    assert.equal(shouldUpdateCreemSecret('new-key', '********', true), true)
    assert.equal(shouldUpdateCreemSecret('new-key', '********', false), false)
  })
})

describe('Payment settings request flow', () => {
  test('sends generic options and partial Creem config in one payload', () => {
    const options = [{ key: 'MinTopUp', value: '12.5' }]
    const creem = { api_key: 'new-api', test_mode: true }
    assert.deepEqual(buildPaymentSettingsPayload(options, creem), {
      options,
      creem,
    })
  })

  test('omits Creem section when there are no Creem changes', () => {
    assert.deepEqual(
      buildPaymentSettingsPayload([{ key: 'MinTopUp', value: '12.5' }]),
      { options: [{ key: 'MinTopUp', value: '12.5' }] }
    )
  })
})

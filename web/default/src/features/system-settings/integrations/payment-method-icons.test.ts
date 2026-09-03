import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  BUILT_IN_PAYMENT_ICONS,
  parseAvailablePaymentIcons,
  serializeAvailablePaymentIcons,
} from './payment-method-icons'

describe('payment method icon library', () => {
  test('falls back to the full curated catalog and ignores unknown icons', () => {
    assert.deepEqual(parseAvailablePaymentIcons(''), [...BUILT_IN_PAYMENT_ICONS])
    assert.deepEqual(parseAvailablePaymentIcons('["SiStripe","Unknown"]'), [
      'SiStripe',
    ])
    assert.deepEqual(parseAvailablePaymentIcons('not-json'), [
      ...BUILT_IN_PAYMENT_ICONS,
    ])
  })

  test('serializes only known icons in stable catalog order', () => {
    assert.equal(
      serializeAvailablePaymentIcons([
        'SiStripe',
        'LuExternalLink',
        'SiAlipay',
        'Unknown',
      ]),
      '["LuExternalLink","SiAlipay","SiStripe"]'
    )
  })
})

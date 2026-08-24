import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getPaymentMethodMinimumCurrency,
  getPaymentMethodMinimumForDisplay,
  hasEditablePaymentMethodMinimum,
} from './payment-method-minimum'

describe('payment method minimum currency', () => {
  test('uses settlement currencies for built-in gateways', () => {
    assert.equal(getPaymentMethodMinimumCurrency('stripe'), 'USD')
    assert.equal(getPaymentMethodMinimumCurrency('yookassa_sbp'), 'RUB')
    assert.equal(getPaymentMethodMinimumCurrency('nowpayments'), 'USDT')
    assert.equal(getPaymentMethodMinimumCurrency('waffo_pancake'), 'USD')
  })

  test('uses configured Waffo currency', () => {
    assert.equal(getPaymentMethodMinimumCurrency('waffo', 'rub'), 'RUB')
    assert.equal(getPaymentMethodMinimumCurrency('waffo', ''), 'USD')
  })

  test('keeps legacy EPay methods in USD', () => {
    assert.equal(getPaymentMethodMinimumCurrency('alipay'), 'USD')
    assert.equal(getPaymentMethodMinimumCurrency('custom1'), 'USD')
  })

  test('hides minimums owned by provider integration settings', () => {
    assert.equal(hasEditablePaymentMethodMinimum('stripe'), false)
    assert.equal(hasEditablePaymentMethodMinimum('waffo_pancake'), false)
    assert.equal(hasEditablePaymentMethodMinimum('yookassa_sbp'), false)
    assert.equal(hasEditablePaymentMethodMinimum('nowpayments'), false)
    assert.equal(hasEditablePaymentMethodMinimum('alipay'), true)
    assert.equal(
      getPaymentMethodMinimumForDisplay('stripe', '100'),
      undefined
    )
    assert.equal(getPaymentMethodMinimumForDisplay('alipay', '10'), '10')
  })
})

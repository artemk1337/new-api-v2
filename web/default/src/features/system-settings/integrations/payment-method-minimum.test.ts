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
    assert.equal(getPaymentMethodMinimumCurrency('usdt_trc20_direct'), 'USDT')
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

  test('allows a method-specific minimum for every payment type', () => {
    assert.equal(hasEditablePaymentMethodMinimum('stripe'), true)
    assert.equal(hasEditablePaymentMethodMinimum('waffo_pancake'), true)
    assert.equal(hasEditablePaymentMethodMinimum('yookassa_sbp'), true)
    assert.equal(hasEditablePaymentMethodMinimum('nowpayments'), true)
    assert.equal(hasEditablePaymentMethodMinimum('usdt_trc20_direct'), true)
    assert.equal(hasEditablePaymentMethodMinimum('alipay'), true)
    assert.equal(getPaymentMethodMinimumForDisplay('stripe', '100'), '100')
    assert.equal(
      getPaymentMethodMinimumForDisplay('usdt_trc20_direct', '10'),
      '10'
    )
    assert.equal(getPaymentMethodMinimumForDisplay('alipay', '10'), '10')
  })
})

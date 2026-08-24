/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { filterAvailablePaymentMethods } from './use-topup-info'

const allMethods = [
  { name: 'EPay', type: 'alipay' },
  { name: 'Stripe', type: 'stripe' },
  { name: 'Waffo', type: 'waffo' },
  { name: 'Pancake', type: 'waffo_pancake' },
  { name: 'SBP', type: 'yookassa_sbp' },
  { name: 'Crypto', type: 'nowpayments' },
]

describe('top-up payment method availability', () => {
  test('keeps only enabled gateways and makes the base Waffo method auto-selectable', () => {
    const methods = filterAvailablePaymentMethods(
      allMethods,
      {
        enable_online_topup: true,
        enable_stripe_topup: false,
        enable_waffo_topup: true,
        enable_waffo_pancake_topup: false,
        enable_yookassa_topup: true,
        enable_nowpayments_topup: false,
      },
      false
    )
    assert.deepEqual(
      methods.map((method) => method.type),
      ['alipay', 'waffo', 'yookassa_sbp']
    )
  })

  test('hides the base Waffo auto-selection when a visible Waffo sub-method exists', () => {
    const methods = filterAvailablePaymentMethods(
      allMethods,
      {
        enable_online_topup: false,
        enable_stripe_topup: false,
        enable_waffo_topup: true,
        enable_waffo_pancake_topup: false,
        enable_yookassa_topup: false,
        enable_nowpayments_topup: false,
      },
      true
    )
    assert.deepEqual(methods, [])
  })
})

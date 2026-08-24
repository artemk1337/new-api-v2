/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { isIncompleteSuccessfulPaymentResponse } from './use-payment'

describe('payment response redirects', () => {
  test('reports successful responses without a provider redirect', () => {
    const response = { success: true, data: {} }

    for (const paymentType of ['stripe', 'yookassa_sbp', 'nowpayments', 'alipay']) {
      assert.equal(
        isIncompleteSuccessfulPaymentResponse(response, paymentType),
        true
      )
    }
  })

  test('does not turn an API error into a duplicate redirect error', () => {
    assert.equal(
      isIncompleteSuccessfulPaymentResponse(
        { success: false, message: 'provider unavailable' },
        'stripe'
      ),
      false
    )
  })
})

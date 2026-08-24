/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  getSafeCreemCheckoutUrl,
  isIncompleteSuccessfulCreemPaymentResponse,
} from './use-creem-payment'

describe('Creem checkout redirect', () => {
  test('accepts an absolute HTTPS checkout URL', () => {
    assert.equal(
      getSafeCreemCheckoutUrl({
        checkout_url: ' https://checkout.creem.io/session/test ',
      }),
      'https://checkout.creem.io/session/test'
    )
  })

  test('rejects executable and data checkout URLs', () => {
    assert.equal(
      getSafeCreemCheckoutUrl({ checkout_url: 'javascript:alert(1)' }),
      null
    )
    assert.equal(
      getSafeCreemCheckoutUrl({
        checkout_url: 'data:text/html,<script>alert(1)</script>',
      }),
      null
    )
  })

  test('marks a successful response without checkout URL as incomplete', () => {
    assert.equal(
      isIncompleteSuccessfulCreemPaymentResponse({
        success: true,
        message: 'success',
        data: {},
      }),
      true
    )
    assert.equal(
      isIncompleteSuccessfulCreemPaymentResponse({ success: false }),
      false
    )
  })
})

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { isIncompleteSuccessfulWaffoPaymentResponse } from './use-waffo-payment'

describe('Waffo checkout redirect', () => {
  test('marks a successful response without payment URL as incomplete', () => {
    assert.equal(
      isIncompleteSuccessfulWaffoPaymentResponse({
        success: true,
        message: 'success',
        data: {},
      }),
      true
    )
    assert.equal(
      isIncompleteSuccessfulWaffoPaymentResponse({ success: false }),
      false
    )
  })
})

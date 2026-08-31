/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { getDirectCryptoPaymentEndpoint } from '../api'
import {
  getDirectCryptoInvoicePath,
  getDirectCryptoCheckoutSearch,
  isSafeDirectCryptoInvoiceUrl,
  parseDirectCryptoInvoiceUrl,
  prepareDirectCryptoPayment,
} from './direct-crypto-checkout'

test('does not prepare an invoice request before a server-advertised network is selected', () => {
  assert.equal(prepareDirectCryptoPayment(25, ['TRON'], null, true), null)
  assert.equal(prepareDirectCryptoPayment(25, ['TRON'], 'TON', true), null)
  assert.equal(prepareDirectCryptoPayment(25, ['TRON'], 'TRON', false), null)
})

test('keeps Crypto selection in the wallet until the main Pay action', () => {
  assert.equal(getDirectCryptoCheckoutSearch('crypto_direct', 0), null)
  assert.equal(getDirectCryptoCheckoutSearch('stripe', 25), null)
  assert.deepEqual(getDirectCryptoCheckoutSearch('crypto_direct', 25), {
    amount: 25,
  })
})

test('prepares the direct crypto API request only after the network choice', () => {
  assert.deepEqual(
    prepareDirectCryptoPayment(25, ['TRON', 'TON'], 'TON', true),
    {
      network: 'TON',
      request: { amount: 25, payment_method: 'crypto_direct' },
    }
  )
})

test('uses an immutable invoice route for the selected network only', () => {
  assert.equal(
    getDirectCryptoInvoicePath('TRON', 'trade-1'),
    '/crypto/tron/trade-1'
  )
  assert.equal(isSafeDirectCryptoInvoiceUrl('/crypto/ton/trade-1'), true)
  assert.equal(isSafeDirectCryptoInvoiceUrl('/usdt-trc20/trade-1'), false)
  assert.equal(
    isSafeDirectCryptoInvoiceUrl('/crypto/ton/trade-1?next=/wallet'),
    false
  )
  assert.deepEqual(parseDirectCryptoInvoiceUrl('/crypto/ton/trade-1'), {
    network: 'TON',
    tradeNo: 'trade-1',
  })
})

test('uses the generic direct crypto endpoint for every selected network', () => {
  assert.equal(
    getDirectCryptoPaymentEndpoint('TRON'),
    '/api/user/crypto/tron/pay'
  )
  assert.equal(
    getDirectCryptoPaymentEndpoint('TON'),
    '/api/user/crypto/ton/pay'
  )
})

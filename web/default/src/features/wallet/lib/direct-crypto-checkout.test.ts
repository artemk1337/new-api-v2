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
  formatDirectCryptoAmount,
  getDirectCryptoPaymentAddress,
  getDirectCryptoInvoicePath,
  getDirectCryptoHistoryNetwork,
  canOpenDirectCryptoHistoryPayment,
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

test('shows the immutable token destination for every network', () => {
  assert.equal(
    getDirectCryptoPaymentAddress(
      {
        receiving_address: 'ton-owner',
        destination_token_account: 'ton-destination',
      },
      'TON'
    ),
    'ton-destination'
  )
  assert.equal(
    getDirectCryptoPaymentAddress(
      { receiving_address: 'legacy-address' },
      'TRON'
    ),
    'legacy-address'
  )
  assert.equal(
    getDirectCryptoPaymentAddress(
      { receiving_address: 'solana-owner' },
      'SOLANA'
    ),
    ''
  )
})

test('trims API amount padding without losing exact decimals', () => {
  assert.equal(formatDirectCryptoAmount('10.010000'), '10.01')
  assert.equal(formatDirectCryptoAmount('10.000001'), '10.000001')
  assert.equal(formatDirectCryptoAmount('10.000000'), '10')
})

test('recognizes legacy direct provider IDs in billing history', () => {
  assert.equal(
    getDirectCryptoHistoryNetwork(
      'usdt_ton_direct',
      'usdt_ton_direct',
      'legacy-1'
    ),
    'TON'
  )
  assert.equal(
    getDirectCryptoHistoryNetwork(
      'usdt_solana_direct',
      'crypto_direct',
      'legacy-2'
    ),
    'SOLANA'
  )
  assert.equal(
    getDirectCryptoHistoryNetwork(
      'crypto_direct',
      'crypto_direct',
      'TONlegacy-3'
    ),
    'TON'
  )
  assert.equal(
    getDirectCryptoHistoryNetwork('stripe', 'stripe', 'legacy-4'),
    null
  )
})

test('only lets the invoice owner reopen a pending history payment', () => {
  const record = {
    status: 'pending' as const,
    user_id: 42,
    payment_provider: 'crypto_direct',
    payment_method: 'crypto_direct',
    trade_no: 'TONlegacy-5',
  }
  assert.equal(canOpenDirectCryptoHistoryPayment(record, 7), false)
  assert.equal(canOpenDirectCryptoHistoryPayment(record, 42), true)
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

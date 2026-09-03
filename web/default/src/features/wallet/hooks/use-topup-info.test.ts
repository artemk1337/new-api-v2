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
  filterAvailablePaymentMethods,
  parseCryptoNetworks,
  parsePaymentMethods,
} from './use-topup-info'

const allMethods = [
  { name: 'EPay', type: 'alipay' },
  { name: 'Stripe', type: 'stripe' },
  { name: 'Waffo', type: 'waffo' },
  { name: 'Pancake', type: 'waffo_pancake' },
  { name: 'SBP', type: 'yookassa_sbp' },
  { name: 'Crypto', type: 'nowpayments' },
  { name: 'Crypto', type: 'crypto_direct', min_topup: 10 },
]

describe('top-up payment method availability', () => {
  test('uses only server-advertised crypto networks and preserves their order', () => {
    assert.deepEqual(
      parseCryptoNetworks(['TON', 'SOLANA', 'TON', 'unsupported']),
      ['TON', 'SOLANA']
    )
    assert.deepEqual(parseCryptoNetworks([]), [])
  })

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
        crypto_networks: ['TRON', 'TON', 'SOLANA'],
      },
      false
    )
    assert.deepEqual(
      methods.map((method) => method.type),
      [
        'alipay',
        'waffo',
        'yookassa_sbp',
        'crypto_direct',
      ]
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
        crypto_networks: ['TRON'],
      },
      true
    )
    assert.deepEqual(
      methods.map((method) => method.type),
      ['crypto_direct']
    )
  })

  test('keeps direct USDT visible when legacy online top-up is disabled', () => {
    const methods = filterAvailablePaymentMethods(
      [{ name: 'Crypto', type: 'crypto_direct' }],
      {
        enable_online_topup: false,
        enable_stripe_topup: false,
        enable_waffo_topup: false,
        enable_waffo_pancake_topup: false,
        enable_yookassa_topup: false,
        enable_nowpayments_topup: false,
        crypto_networks: ['TRON'],
      },
      false
    )
    assert.deepEqual(
      methods.map((method) => method.type),
      ['crypto_direct']
    )
  })

  test('keeps manual transfer visible when legacy online top-up is disabled', () => {
    const methods = filterAvailablePaymentMethods(
      [{ name: 'Direct transfer', type: 'manual_transfer' }],
      {
        enable_online_topup: false,
        enable_stripe_topup: false,
        enable_waffo_topup: false,
        enable_waffo_pancake_topup: false,
        enable_yookassa_topup: false,
        enable_nowpayments_topup: false,
        crypto_networks: [],
      },
      false
    )
    assert.deepEqual(
      methods.map((method) => method.type),
      ['manual_transfer']
    )
  })

  test('preserves the direct transfer link and description from the server', () => {
    const [method] = parsePaymentMethods(
      [
        {
          name: 'Direct transfer',
          type: 'manual_transfer',
          contact_url: 'https://t.me/support',
          description: 'Contact the manager before paying.',
        },
      ],
      0,
      []
    )

    assert.deepEqual(method, {
      name: 'Direct transfer',
      type: 'manual_transfer',
      color: undefined,
      icon: undefined,
      description: 'Contact the manager before paying.',
      contact_url: 'https://t.me/support',
      admin_only: undefined,
      min_topup: 0,
      topup_ratio: 1,
      rate_to_usd: undefined,
      base_amount_multiplier: undefined,
      rounding_decimals: undefined,
      currency: undefined,
      payment_amount: undefined,
      currency_symbol: undefined,
      crypto_networks: undefined,
    })
  })
})

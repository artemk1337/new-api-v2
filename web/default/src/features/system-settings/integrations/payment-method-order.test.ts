import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { movePaymentMethodInJson } from './payment-method-order'

describe('payment method order', () => {
  test('moves a method up and keeps its persisted metadata', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp', icon: 'LuCreditCard' },
      { name: 'Crypto', type: 'crypto_direct', min_topup: '10' },
      { name: 'Stripe', type: 'stripe', min_topup: '20' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'Stripe', type: 'stripe' },
      'up'
    )

    assert.equal(
      moved,
      JSON.stringify(
        [
          { name: 'SBP', type: 'yookassa_sbp', icon: 'LuCreditCard' },
          { name: 'Stripe', type: 'stripe', min_topup: '20' },
          { name: 'Crypto', type: 'crypto_direct', min_topup: '10' },
        ],
        null,
        2
      )
    )
  })

  test('moves around non-method entries without deleting them', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp' },
      { comment: 'legacy metadata' },
      { name: 'Crypto', type: 'crypto_direct' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'Crypto', type: 'crypto_direct' },
      'up'
    )

    assert.deepEqual(JSON.parse(moved ?? ''), [
      { name: 'Crypto', type: 'crypto_direct' },
      { comment: 'legacy metadata' },
      { name: 'SBP', type: 'yookassa_sbp' },
    ])
  })

  test('moves a method down in the persisted order', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'Crypto', type: 'crypto_direct' },
      { name: 'Stripe', type: 'stripe' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'SBP', type: 'yookassa_sbp' },
      'down'
    )

    assert.deepEqual(JSON.parse(moved ?? ''), [
      { name: 'Crypto', type: 'crypto_direct' },
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'Stripe', type: 'stripe' },
    ])
  })

  test('treats legacy duplicate crypto entries as one visible method', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
      { name: 'Stripe', type: 'stripe' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'Crypto', type: 'crypto_direct' },
      'down'
    )

    assert.deepEqual(JSON.parse(moved ?? ''), [
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'Stripe', type: 'stripe' },
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
    ])
  })

  test('keeps legacy duplicate crypto entries in order when moved up', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
      { name: 'Stripe', type: 'stripe' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'Crypto', type: 'crypto_direct' },
      'up'
    )

    assert.deepEqual(JSON.parse(moved ?? ''), [
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'Stripe', type: 'stripe' },
    ])
  })

  test('keeps legacy duplicate crypto entries in order when another method moves up', () => {
    const value = JSON.stringify([
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
      { name: 'Stripe', type: 'stripe' },
    ])

    const moved = movePaymentMethodInJson(
      value,
      { name: 'Stripe', type: 'stripe' },
      'up'
    )

    assert.deepEqual(JSON.parse(moved ?? ''), [
      { name: 'SBP', type: 'yookassa_sbp' },
      { name: 'Stripe', type: 'stripe' },
      { name: 'USDT TRC20', type: 'crypto_direct', min_topup: '10' },
      { name: 'USDT TON', type: 'crypto_direct', min_topup: '10' },
    ])
  })

  test('does not change the value at the boundary or for an unknown method', () => {
    const value = JSON.stringify([{ name: 'SBP', type: 'yookassa_sbp' }])

    assert.equal(
      movePaymentMethodInJson(value, { name: 'SBP', type: 'yookassa_sbp' }, 'up'),
      null
    )
    assert.equal(
      movePaymentMethodInJson(value, { name: 'Stripe', type: 'stripe' }, 'down'),
      null
    )
  })
})

/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  formatTopupDisplayAmount,
  formatTopupPaymentDisplayAmount,
  getStatusConfig,
  getTopupStatusLabel,
  getTopupDisplayAmount,
  getTopupAmountToDisplay,
  getTopupPaymentDisplayAmount,
} from '../../lib/billing'

describe('billing history status', () => {
  test('renders failed payments as failed instead of pending', () => {
    assert.deepEqual(getStatusConfig('failed'), {
      variant: 'danger',
      label: 'Failed',
    })
  })

  test('translates every visible payment status at the UI boundary', () => {
    const ru: Record<string, string> = {
      Success: 'Успешно',
      Pending: 'Ожидает',
      Failed: 'Неудача',
      Expired: 'Просрочено',
    }
    const t = (key: string) => ru[key] ?? key

    assert.equal(getTopupStatusLabel('success', t), 'Успешно')
    assert.equal(getTopupStatusLabel('pending', t), 'Ожидает')
    assert.equal(getTopupStatusLabel('failed', t), 'Неудача')
    assert.equal(getTopupStatusLabel('expired', t), 'Просрочено')
  })
})

describe('billing history top-up amount', () => {
  test('shows the requested fractional amount when available', () => {
    assert.equal(getTopupAmountToDisplay(0, 0.1), 0.1)
  })

  test('falls back to legacy amount for absent or invalid values', () => {
    assert.equal(getTopupAmountToDisplay(5), 5)
    assert.equal(getTopupAmountToDisplay(5, 0), 5)
    assert.equal(getTopupAmountToDisplay(5, Number.NaN), 5)
  })

  test('falls back to money for legacy subscription records', () => {
    assert.equal(getTopupAmountToDisplay(0, 0, 12.5), 12.5)
  })

  test('prefers the immutable USD accounting amount', () => {
    assert.equal(getTopupAmountToDisplay(0, 12.5, 12.5, 8), 8)
    assert.equal(getTopupAmountToDisplay(0, 12.5, 12.5, 0), 0)
  })

  test('keeps provider currency for subscription history without USD snapshot', () => {
    const display = getTopupDisplayAmount({
      amount: 0,
      requestedAmount: 10,
      money: 10,
      paymentCurrency: 'EUR',
    })
    assert.deepEqual(display, { amount: 10, currency: 'EUR' })
    assert.equal(formatTopupDisplayAmount(display), '€10.00')
  })

  test('uses the provider amount for legacy NOWPayments token history', () => {
    const display = getTopupDisplayAmount({
      amount: 500000,
      requestedAmount: 500000,
      money: 10,
      paymentCurrency: 'USDT',
    })
    assert.deepEqual(display, { amount: 10, currency: 'USDT' })
    assert.equal(formatTopupDisplayAmount(display), '₮10.00')
  })

  test('uses the provider amount for legacy Waffo token history', () => {
    const display = getTopupDisplayAmount({
      amount: 500000,
      requestedAmount: 500000,
      money: 900,
      paymentCurrency: 'RUB',
    })
    assert.deepEqual(display, { amount: 900, currency: 'RUB' })
    assert.equal(formatTopupDisplayAmount(display), '₽900.00')
  })

  test('uses provider metadata for migrated NOWPayments rows with USD default', () => {
    const display = getTopupDisplayAmount({
      amount: 500000,
      requestedAmount: 500000,
      money: 10,
      paymentCurrency: 'USD',
      paymentProvider: 'nowpayments',
    })
    assert.deepEqual(display, { amount: 10, currency: 'USDT' })
    assert.equal(formatTopupDisplayAmount(display), '₮10.00')
  })

  test('uses provider amount for migrated Waffo rows with USD default', () => {
    const display = getTopupDisplayAmount({
      amount: 500000,
      requestedAmount: 500000,
      money: 900,
      paymentCurrency: 'USD',
      paymentProvider: 'waffo',
    })
    assert.deepEqual(display, { amount: 900, currency: 'USD' })
    assert.equal(formatTopupDisplayAmount(display), '$900')
  })

  test('prefers immutable wallet USD amount for non-USD provider payments', () => {
    const display = getTopupDisplayAmount({
      amount: 0,
      requestedAmount: 900,
      money: 900,
      paymentCurrency: 'RUB',
      paymentBaseAmount: 10,
    })
    assert.deepEqual(display, { amount: 10, currency: 'USD' })
  })

  test('shows the settled payment with its provider currency', () => {
    const payment = getTopupPaymentDisplayAmount({
      money: 927,
      paymentChargedAmount: 927,
      paymentCurrency: 'RUB',
      paymentProvider: 'yookassa',
    })
    assert.deepEqual(payment, { amount: 927, currency: 'RUB' })
    assert.equal(formatTopupPaymentDisplayAmount(payment), '₽927.00')
  })

  test('uses provider metadata for legacy payment currencies with USD defaults', () => {
    const payment = getTopupPaymentDisplayAmount({
      money: 10,
      paymentCurrency: 'USD',
      paymentProvider: 'nowpayments',
    })
    assert.deepEqual(payment, { amount: 10, currency: 'USDT' })
    assert.equal(formatTopupPaymentDisplayAmount(payment), '₮10.00')
  })

  test('does not invent a zero payment for promo and referral credits', () => {
    for (const source of ['promo_code', 'referral_income']) {
      const payment = getTopupPaymentDisplayAmount({ source })
      assert.equal(payment, null)
      assert.equal(formatTopupPaymentDisplayAmount(payment), '—')
    }
  })

  test('keeps a real zero-price payment distinguishable from a promo credit', () => {
    const payment = getTopupPaymentDisplayAmount({
      money: 0,
      paymentCurrency: 'USD',
      source: 'payment_method',
    })
    assert.deepEqual(payment, { amount: 0, currency: 'USD' })
    assert.equal(formatTopupPaymentDisplayAmount(payment), '$0.00')
  })
})

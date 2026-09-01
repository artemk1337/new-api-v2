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
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { getTopupAmountToDisplay, getTopupSourceLabel } from '../lib/billing'
import { getTopupHistoryTitleKey } from '../lib/topup-history-title'
import {
  formatPaymentSummaryAmount,
  canCheckYooKassaPayment,
  getPaymentSummaryValues,
  getSelectedPaymentMethodName,
  getSelectedPaymentMethodSubtitle,
  getTopupHistoryTotalPages,
  isWalletSummaryReady,
} from './wallet-sidebar-cards'

describe('wallet summary readiness', () => {
  test('requires both a positive amount and a payment method', () => {
    assert.equal(isWalletSummaryReady(0, false), false)
    assert.equal(isWalletSummaryReady(20, false), false)
    assert.equal(isWalletSummaryReady(0, true), false)
    assert.equal(isWalletSummaryReady(20, true), true)
  })
})

describe('top-up history source labels', () => {
  test('uses human-facing labels for provider and non-payment sources', () => {
    assert.equal(getTopupSourceLabel(undefined, 'yookassa_sbp'), 'СБП')
    assert.equal(
      getTopupSourceLabel(undefined, 'yookassa_sbp', undefined, 'СБП'),
      'СБП'
    )
    assert.equal(
      getTopupSourceLabel(undefined, 'custom1', undefined, 'FastPay'),
      'FastPay'
    )
    assert.equal(
      getTopupSourceLabel('promo_code', 'balance'),
      'Top-up by promo code'
    )
    assert.equal(
      getTopupSourceLabel('referral_income', 'balance'),
      'Referral top-up income'
    )
  })

  test('does not expose unknown source identifiers', () => {
    assert.equal(getTopupSourceLabel('user_12345', 'provider_6789'), 'Payment')
    assert.equal(getTopupSourceLabel('alice', 'provider_6789'), 'Payment')
  })
})

describe('top-up history amounts', () => {
  test('falls back from a zero requested amount to the legacy amount', () => {
    assert.equal(getTopupAmountToDisplay(11, 0), 11)
    assert.equal(getTopupAmountToDisplay(11, Number.NaN), 11)
    assert.equal(getTopupAmountToDisplay(0, 0, 12.5), 12.5)
  })
})

describe('top-up history pagination', () => {
  test('uses five records per page including empty and partial pages', () => {
    assert.equal(getTopupHistoryTotalPages(0), 1)
    assert.equal(getTopupHistoryTotalPages(5), 1)
    assert.equal(getTopupHistoryTotalPages(6), 2)
    assert.equal(getTopupHistoryTotalPages(10), 2)
    assert.equal(getTopupHistoryTotalPages(11), 3)
  })

  test('guards malformed totals and custom page sizes', () => {
    assert.equal(getTopupHistoryTotalPages(Number.NaN), 1)
    assert.equal(getTopupHistoryTotalPages(-10), 1)
    assert.equal(getTopupHistoryTotalPages(10, 10), 1)
  })
})

describe('YooKassa payment verification', () => {
  test('is available for pending or expired YooKassa payments', () => {
    assert.equal(
      canCheckYooKassaPayment({
        payment_provider: 'yookassa',
        status: 'pending',
      }),
      true
    )
    assert.equal(
      canCheckYooKassaPayment({
        payment_provider: 'yookassa',
        status: 'expired',
      }),
      true
    )
    assert.equal(
      canCheckYooKassaPayment({
        payment_provider: 'stripe',
        status: 'pending',
      }),
      false
    )
    assert.equal(
      canCheckYooKassaPayment({
        payment_provider: 'yookassa',
        status: 'success',
      }),
      false
    )
  })
})

describe('top-up history title', () => {
  test('uses the admin title only for administrators', () => {
    assert.equal(getTopupHistoryTitleKey(false), 'Top-up history')
    assert.equal(getTopupHistoryTitleKey(true), "Users' top-up history")
  })
})

describe('wallet summary currency values', () => {
  test('formats accounting values with the configured wallet currency', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency

    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CNY',
          usdExchangeRate: 7,
        },
      })
      assert.equal(formatPaymentSummaryAmount(1.2), '¥8.4')

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CUSTOM',
          customCurrencySymbol: '€',
          customCurrencyExchangeRate: 0.9,
        },
      })
      assert.equal(formatPaymentSummaryAmount(1.2), '€ 1.08')
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })

  test('shows gross charge, fee, and wallet credit separately', () => {
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 1,
        commissionUSD: 0.2,
        creditedAmountUSD: 1,
        chargedAmountUSD: 1.2,
        cashbackPercent: 0,
        cashbackAmountUSD: 0,
      }),
      {
        topup: 1.2,
        commission: 0.2,
        credited: 1,
        cashbackPercent: 0,
        cashbackAmount: 0,
      }
    )
  })

  test('uses the display snapshot fields for the summary', () => {
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 146,
        commissionUSD: 0,
        creditedAmountUSD: 146,
        chargedAmountUSD: 146,
        cashbackPercent: 0,
        cashbackAmountUSD: 0,
      }),
      {
        topup: 146,
        commission: 0,
        credited: 146,
        cashbackPercent: 0,
        cashbackAmount: 0,
      }
    )
    // The raw input can be 10 USD, while the provider quote's gross base is
    // 12.5 USD and therefore selects the server-side cashback tier.
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 100,
        commissionUSD: 10,
        creditedAmountUSD: 100,
        chargedAmountUSD: 110,
        cashbackPercent: 0,
        cashbackAmountUSD: 0,
      }),
      {
        topup: 110,
        commission: 10,
        credited: 100,
        cashbackPercent: 0,
        cashbackAmount: 0,
      }
    )
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 10,
        commissionUSD: 20,
        creditedAmountUSD: 10,
        chargedAmountUSD: 30,
        cashbackPercent: 0,
        cashbackAmountUSD: 0,
      }),
      {
        topup: 30,
        commission: 20,
        credited: 10,
        cashbackPercent: 0,
        cashbackAmount: 0,
      }
    )
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 12.5,
        commissionUSD: 0,
        creditedAmountUSD: 12.5,
        chargedAmountUSD: 12.5,
        cashbackPercent: 2,
        cashbackAmountUSD: 0.25,
      }),
      {
        topup: 12.5,
        commission: 0,
        credited: 12.5,
        cashbackPercent: 2,
        cashbackAmount: 0.25,
      }
    )
  })

  test('preserves the quote commission precision in the summary', () => {
    const values = getPaymentSummaryValues({
      baseAmountUSD: 0.1,
      commissionUSD: 0.013333333333333333,
      creditedAmountUSD: 0.1,
      chargedAmountUSD: 0.11333333333333333,
      cashbackPercent: 0,
      cashbackAmountUSD: 0,
    })

    assert.equal(values.commission, 0.013333333333333333)
  })

  test('returns empty values until a valid server quote is available', () => {
    assert.deepEqual(getPaymentSummaryValues(null), {
      topup: 0,
      commission: 0,
      credited: 0,
      cashbackPercent: 0,
      cashbackAmount: 0,
    })
    assert.deepEqual(
      getPaymentSummaryValues({
        baseAmountUSD: 0,
        commissionUSD: 0,
        creditedAmountUSD: 0,
        chargedAmountUSD: 0,
        cashbackPercent: 0,
        cashbackAmountUSD: 0,
      }),
      {
        topup: 0,
        commission: 0,
        credited: 0,
        cashbackPercent: 0,
        cashbackAmount: 0,
      }
    )
  })
})

describe('wallet summary payment method', () => {
  test('uses the configured display name instead of a raw payment type', () => {
    assert.equal(
      getSelectedPaymentMethodName({
        type: 'yookassa_sbp',
        name: 'СБП через ЮKassa',
      }),
      'СБП через ЮKassa'
    )
  })

  test('falls back to the existing payment method name mapping', () => {
    assert.equal(
      getSelectedPaymentMethodName({ type: 'stripe', name: ' ' }),
      'Stripe'
    )
  })

  test('adds a mapped subtitle without exposing an unknown raw id', () => {
    assert.equal(
      getSelectedPaymentMethodSubtitle({
        type: 'stripe',
        name: 'Банковская карта',
      }),
      'Stripe'
    )
    assert.equal(
      getSelectedPaymentMethodSubtitle({ type: 'yookassa_sbp', name: 'СБП' }),
      null
    )
  })
})

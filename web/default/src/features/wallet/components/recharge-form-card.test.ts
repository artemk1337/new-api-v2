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
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatCurrency } from '../lib/format'
import {
  getCashbackTierSummary,
  getPaymentMethodDisplayQuote,
  getMinimumAvailablePaymentMethodAmount,
  isPaymentMethodAmountEligible,
} from '../lib/payment'
import { getBackendTopupAmount } from '../lib/topup-input'
import type { CashbackThreshold, TopupInfo } from '../types'
import {
  getAvailableWaffoMethods,
  getCashbackTierPosition,
  getCashbackTierTranslate,
  getCashbackTierRangeLabel,
  getCashbackTierDisplayThresholds,
  getCashbackDisplayAmount,
  getRenderableCashbackTiers,
  hasPositiveCashbackTier,
  getWaffoPaymentMethod,
  hasPaymentMethodDisplayConfig,
  formatPaymentQuoteAmount,
  getMethodCommissionLabel,
  getPaymentQuoteDisplay,
  getRechargeValidationTarget,
  getRechargeStep,
  getTopupAmountErrorMessage,
  isTopupAmountValidationActive,
  shouldShowPaymentMethodQuote,
  canSelectPaymentMethod,
  formatWalletInputAmount,
  getManualTopupContact,
  isLegacyCustomMethod,
  isManualTopupAmountEligible,
  parseTopupAmount,
  sanitizeTopupAmount,
} from './recharge-form-card'

describe('custom top-up amount', () => {
  test('computes the minimum across all available methods', () => {
    const methods = [
      {
        name: 'Cheap',
        type: 'custom',
        min_topup: 5,
        currency: 'USD',
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
      },
      {
        name: 'Expensive',
        type: 'custom',
        min_topup: 20,
        currency: 'USD',
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
      },
    ]
    const info = { pay_methods: methods } as TopupInfo
    assert.equal(getMinimumAvailablePaymentMethodAmount(info), 5)
    assert.equal(isPaymentMethodAmountEligible(4, methods[0]), false)
    assert.equal(isPaymentMethodAmountEligible(5, methods[0]), true)
    assert.equal(isPaymentMethodAmountEligible(5, methods[1]), false)
    assert.equal(isPaymentMethodAmountEligible(20, methods[1]), true)
  })
  test('does not apply a USD minimum to converted wallet units', () => {
    // In TOKENS mode the value sent to the backend is quota units, not USD.
    // Provider-specific minimums (including direct USDT $10) belong to the
    // authoritative server quote.
    assert.equal(canSelectPaymentMethod(1), true)
  })
  test('accepts fractional values with either decimal separator', () => {
    assert.equal(parseTopupAmount('0.1'), 0.1)
    assert.equal(parseTopupAmount('0,1'), 0.1)
  })

  test('keeps partial values out of payment calculations', () => {
    assert.equal(parseTopupAmount('0,'), null)
    assert.equal(parseTopupAmount('0.'), null)
  })

  test('clears a previously valid amount when input becomes incomplete', () => {
    assert.equal(getBackendTopupAmount(parseTopupAmount('10')), 10)
    assert.equal(getBackendTopupAmount(parseTopupAmount('1.')), 0)
    assert.equal(getBackendTopupAmount(parseTopupAmount('.')), 0)
  })

  test('removes invalid characters while preserving a fractional value', () => {
    assert.equal(sanitizeTopupAmount('abc12x,3$'), '12.3')
    assert.equal(sanitizeTopupAmount('1.2.3'), '1.23')
    assert.equal(sanitizeTopupAmount('1e3'), '13')
  })
})

describe('manual top-up contact', () => {
  test('allows the Telegram link only at the server-calculated minimum', () => {
    const manualTopup = getManualTopupContact({
      manual_topup_enabled: true,
      manual_topup_min_amount: 5000,
      manual_topup_min_amount_backend: 50,
      manual_topup_contact_url: 'https://t.me/vibecode_support',
    } as TopupInfo)

    assert.notEqual(manualTopup, null)
    assert.equal(isManualTopupAmountEligible(49.99, manualTopup), false)
    assert.equal(isManualTopupAmountEligible(50, manualTopup), true)
    assert.equal(isManualTopupAmountEligible(60, manualTopup), true)
  })

  test('fails closed when the server does not provide a wallet minimum', () => {
    const manualTopup = getManualTopupContact({
      manual_topup_enabled: true,
      manual_topup_min_amount: 5000,
      manual_topup_contact_url: 'https://t.me/vibecode_support',
    } as TopupInfo)

    assert.equal(manualTopup, null)
    assert.equal(isManualTopupAmountEligible(100, manualTopup), false)
  })

  test('rejects a non-Telegram contact URL', () => {
    const manualTopup = getManualTopupContact({
      manual_topup_enabled: true,
      manual_topup_min_amount: 5000,
      manual_topup_min_amount_backend: 50,
      manual_topup_contact_url: 'https://example.com/contact',
    } as TopupInfo)

    assert.equal(manualTopup, null)
  })
})

describe('wallet display minimum', () => {
  test('shows the server minimum in the selected input unit', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency

    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CNY',
          usdExchangeRate: 7,
        },
      })
      assert.equal(formatWalletInputAmount(1), '7')

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CUSTOM',
          customCurrencySymbol: '€',
          customCurrencyExchangeRate: 0.8,
        },
      })
      assert.equal(formatWalletInputAmount(1.25), '1')

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      })
      assert.equal(formatWalletInputAmount(500000), '500000')
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })
})

describe('recharge steps', () => {
  test('keeps the flow on amount until a valid amount is entered', () => {
    assert.equal(getRechargeStep(0, 1, false), 1)
    assert.equal(getRechargeStep(0.5, 1, false), 1)
  })

  test('advances from payment method to ready only after selection', () => {
    assert.equal(getRechargeStep(20, 1, false), 2)
    assert.equal(getRechargeStep(20, 1, true), 3)
  })
})

describe('recharge validation guidance', () => {
  test('guides to the amount before payment selection', () => {
    assert.equal(getRechargeValidationTarget(0, 1, false), 'amount')
    assert.equal(getRechargeValidationTarget(0.5, 1, false), 'amount')
  })

  test('guides to payment method only after the amount is valid', () => {
    assert.equal(getRechargeValidationTarget(1, 1, false), 'payment-method')
    assert.equal(getRechargeValidationTarget(1, 1, true), null)
  })

  test('shows the formatted minimum amount when entered amount is too low', () => {
    const translate = (key: string, options?: Record<string, unknown>) =>
      key === 'Minimum topup amount: {{amount}}'
        ? `Минимальная сумма пополнения: ${String(options?.amount)}`
        : key

    assert.equal(
      getTopupAmountErrorMessage(4, 5, 'USD', translate),
      'Минимальная сумма пополнения: 5 USD'
    )
    assert.equal(getTopupAmountErrorMessage(0, 5, 'USD', translate), null)
  })

  test('uses the global minimum for realtime validation', () => {
    const methods = [
      {
        name: 'High minimum',
        type: 'custom',
        currency: 'USD',
        min_topup: 1,
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
      },
      {
        name: 'Low minimum',
        type: 'custom',
        currency: 'USD',
        min_topup: 0.09735123,
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
      },
    ]
    const globalMinimum = getMinimumAvailablePaymentMethodAmount({
      pay_methods: methods,
    } as TopupInfo)
    const translate = (key: string, options?: Record<string, unknown>) =>
      key === 'Minimum topup amount: {{amount}}'
        ? `Minimum topup amount: ${String(options?.amount)}`
        : key

    assert.equal(globalMinimum, 0.09735123)
    assert.equal(
      getTopupAmountErrorMessage(0.001, globalMinimum, 'USD', translate),
      'Minimum topup amount: 0.09735123 USD'
    )
    assert.equal(
      getTopupAmountErrorMessage(0.1, globalMinimum, 'USD', translate),
      null
    )
    assert.equal(
      isTopupAmountValidationActive(0.001, globalMinimum, null),
      true
    )
    assert.equal(isTopupAmountValidationActive(0.1, globalMinimum, null), false)
    assert.equal(isPaymentMethodAmountEligible(0.1, methods[0]), false)
    assert.equal(isPaymentMethodAmountEligible(0.1, methods[1]), true)
  })

  test('does not override global validation with a higher method minimum', () => {
    const sbp = {
      name: 'SBP',
      type: 'yookassa_sbp',
      currency: 'RUB',
      min_topup: 1000,
      rate_to_usd: 90,
      base_amount_multiplier: 1,
      topup_ratio: 1,
    }
    const epay = {
      name: 'EPay',
      type: 'epay',
      currency: 'USD',
      min_topup: 1,
      rate_to_usd: 1,
      base_amount_multiplier: 1,
      topup_ratio: 1,
    }
    const stripe = {
      name: 'Stripe',
      type: 'stripe',
      currency: 'USD',
      min_topup: 5,
      rate_to_usd: 1,
      base_amount_multiplier: 1,
      topup_ratio: 1,
    }
    const methods = [sbp, epay, stripe]
    const globalMinimum = getMinimumAvailablePaymentMethodAmount({
      pay_methods: methods,
    } as TopupInfo)

    assert.equal(globalMinimum, 1)
    assert.equal(isPaymentMethodAmountEligible(2, epay), true)
    assert.equal(isPaymentMethodAmountEligible(2, stripe), false)
    assert.equal(isPaymentMethodAmountEligible(2, sbp), false)
    assert.equal(
      getRechargeValidationTarget(2, globalMinimum, false),
      'payment-method'
    )

    const translate = (key: string, options?: Record<string, unknown>) =>
      key === 'Minimum topup amount: {{amount}}'
        ? `Minimum topup amount: ${String(options?.amount)}`
        : key
    assert.equal(
      getTopupAmountErrorMessage(2, globalMinimum, 'USD', translate),
      null
    )
  })
})

describe('payment method preview visibility', () => {
  test('hides the charged amount preview for unavailable methods', () => {
    const quote = {
      charged_amount: 5,
      charged_amount_usd: 5,
      currency: 'USD',
      rate_to_usd: 1,
      rounding_decimals: 2,
    }

    assert.equal(shouldShowPaymentMethodQuote(true, quote), false)
    assert.equal(shouldShowPaymentMethodQuote(false, quote), true)
    assert.equal(shouldShowPaymentMethodQuote(false, null), false)
  })
})

describe('provider minimums', () => {
  test('does not compare a settlement minimum with the USD wallet amount', () => {
    // A RUB provider minimum must be checked by the server quote. A $1 wallet
    // amount is still selectable so the quote can return the authoritative
    // RUB validation result.
    assert.equal(canSelectPaymentMethod(1), true)
    assert.equal(canSelectPaymentMethod(0), false)
  })
})

describe('cashback panel visibility', () => {
  test('hides the panel when cashback is absent, empty, or malformed', () => {
    assert.deepEqual(getRenderableCashbackTiers(undefined), [])
    assert.deepEqual(getRenderableCashbackTiers(null), [])
    assert.deepEqual(getRenderableCashbackTiers([]), [])
    assert.deepEqual(
      getRenderableCashbackTiers([{ min_amount: 0, cashback_percent: 0 }]),
      [{ min_amount: 0, cashback_percent: 0 }]
    )
    assert.equal(
      hasPositiveCashbackTier([{ min_amount: 0, cashback_percent: 0 }]),
      false
    )
    assert.deepEqual(
      getRenderableCashbackTiers([null] as unknown as CashbackThreshold[]),
      []
    )
    assert.deepEqual(
      getRenderableCashbackTiers([
        { min_amount: Number.NaN, cashback_percent: 1 },
        { min_amount: 10, cashback_percent: Number.POSITIVE_INFINITY },
        { min_amount: -1, cashback_percent: 1 },
        { min_amount: 20, cashback_percent: 101 },
        { min_amount: 30, cashback_percent: -1 },
      ]),
      []
    )
  })

  test('shows the panel when at least one valid cashback tier exists', () => {
    assert.deepEqual(
      getRenderableCashbackTiers([
        { min_amount: 'invalid' as unknown as number, cashback_percent: 1 },
        { min_amount: 50, cashback_percent: 2 },
      ]),
      [{ min_amount: 50, cashback_percent: 2 }]
    )
  })

  test('keeps zero-percent tiers when a positive tier enables the panel', () => {
    assert.deepEqual(
      getRenderableCashbackTiers([
        { min_amount: 10, cashback_percent: 1 },
        { min_amount: 20, cashback_percent: 0 },
      ]),
      [
        { min_amount: 10, cashback_percent: 1 },
        { min_amount: 20, cashback_percent: 0 },
      ]
    )
    assert.equal(
      hasPositiveCashbackTier([
        { min_amount: 10, cashback_percent: 1 },
        { min_amount: 20, cashback_percent: 0 },
      ]),
      true
    )
  })

  test('scales visible thresholds into entered amount units', () => {
    assert.deepEqual(
      getCashbackTierDisplayThresholds(
        [
          { min_amount: 10, cashback_percent: 1 },
          { min_amount: 20, cashback_percent: 0 },
        ],
        2
      ),
      [
        { min_amount: 5, cashback_percent: 1 },
        { min_amount: 10, cashback_percent: 0 },
      ]
    )
  })
})

describe('cashback tier positions', () => {
  test('formats inclusive tier ranges above the track', () => {
    const t = (key: string, options?: Record<string, unknown>) =>
      key === 'From {{amount}}' ? `From ${options?.amount}` : key

    assert.equal(
      getCashbackTierRangeLabel(
        { min_amount: 1, cashback_percent: 1 },
        { min_amount: 10, cashback_percent: 2 },
        '$',
        t
      ),
      '$1–$10'
    )
    assert.equal(
      getCashbackTierRangeLabel(
        { min_amount: 20, cashback_percent: 3 },
        undefined,
        '$',
        t
      ),
      'From $20'
    )
    assert.equal(
      getCashbackTierRangeLabel(
        { min_amount: 1, cashback_percent: 1 },
        { min_amount: 10, cashback_percent: 2 },
        '₽',
        t
      ),
      '₽1–₽10'
    )
  })

  test('anchors first and last tiers to the track boundaries', () => {
    assert.equal(getCashbackTierPosition(10, 2, 10, 90), 0)
    assert.equal(getCashbackTierPosition(100, 2, 10, 90), 100)
    assert.equal(getCashbackTierTranslate(0, 2), '0%')
    assert.equal(getCashbackTierTranslate(1, 2), '-100%')
  })

  test('centers a single tier and keeps middle tiers on their points', () => {
    assert.equal(getCashbackTierPosition(50, 1, 50, 1), 50)
    assert.equal(getCashbackTierTranslate(0, 1), '-50%')
    assert.equal(getCashbackTierPosition(55, 3, 10, 90), 50)
    assert.equal(getCashbackTierTranslate(1, 3), '-50%')
  })
})

describe('legacy payment methods', () => {
  test('hides custom1 aliases from the wallet method list', () => {
    assert.deepEqual(
      isLegacyCustomMethod({ type: 'custom1', name: 'Legacy' }),
      true
    )
    assert.equal(
      isLegacyCustomMethod({
        payMethodType: 'card',
        payMethodName: 'Visa',
        name: 'Visa',
      }),
      false
    )
  })

  test('preserves provider indexes after hiding custom1 methods', () => {
    const methods = getAvailableWaffoMethods([
      { name: 'Legacy', payMethodType: 'custom1' },
      { name: 'Card', payMethodType: 'card' },
    ])
    assert.equal(methods.length, 1)
    assert.equal(methods[0]?.index, 1)
  })
})

describe('payment price formatting', () => {
  test('supports fixed two-decimal payment prices', () => {
    assert.equal(formatCurrency(20, 2), '20.00')
  })
})

describe('payment method currency quotes', () => {
  const translate = (key: string, options?: Record<string, unknown>) =>
    key === 'Commission {{percent}}%'
      ? `Commission ${options?.percent}%`
      : 'Commission 0%'

  test('keeps method cards on the configured commission rate', () => {
    assert.equal(getMethodCommissionLabel(1.2, translate), 'Commission 20%')
    assert.equal(getMethodCommissionLabel(1, translate), 'Commission 0%')
    assert.equal(
      getMethodCommissionLabel(undefined, translate),
      'Commission 0%'
    )
  })

  test('shows non-USD payment prices as a compact approximation', () => {
    assert.equal(
      formatPaymentQuoteAmount({
        charged_amount: 1_850,
        charged_amount_usd: 20,
        currency: 'RUB',
        rate_to_usd: 92.5,
        rounding_decimals: 2,
      }),
      '~ ₽1,850.00'
    )
  })

  test('keeps USD payment prices unchanged', () => {
    assert.equal(
      formatPaymentQuoteAmount({
        charged_amount: 20,
        charged_amount_usd: 20,
        currency: 'USD',
        rate_to_usd: 1,
        rounding_decimals: 2,
      }),
      '$20.00'
    )
  })

  test('calculates a configured RUB display quote immediately', () => {
    assert.deepEqual(
      getPaymentQuoteDisplay(
        {
          type: 'yookassa_sbp',
          name: 'СБП',
          currency: 'RUB',
          rate_to_usd: 92.5,
          base_amount_multiplier: 1,
          topup_ratio: 1,
          rounding_decimals: 2,
        },
        20
      ),
      {
        charged_amount: 1_850,
        charged_amount_usd: 20,
        currency: 'RUB',
        rate_to_usd: 92.5,
        rounding_decimals: 2,
      }
    )
  })

  test('does not invent a non-USD amount without display config', () => {
    assert.equal(
      getPaymentQuoteDisplay(
        { type: 'yookassa_sbp', name: 'СБП', currency: 'RUB' },
        20
      ),
      null
    )
  })

  test('keeps RUB display local to the current amount', () => {
    assert.deepEqual(
      getPaymentQuoteDisplay(
        {
          type: 'yookassa_sbp',
          name: 'СБП',
          currency: 'RUB',
          rate_to_usd: 92,
          base_amount_multiplier: 1,
          topup_ratio: 1,
          rounding_decimals: 2,
        },
        20
      ),
      {
        charged_amount: 1_840,
        charged_amount_usd: 20,
        currency: 'RUB',
        rate_to_usd: 92,
        rounding_decimals: 2,
      }
    )
  })

  test('honors a provider zero-decimal settlement currency', () => {
    const quote = getPaymentQuoteDisplay(
      {
        type: 'waffo',
        name: 'Waffo JPY',
        currency: 'JPY',
        rate_to_usd: 150,
        base_amount_multiplier: 1,
        topup_ratio: 1,
        rounding_decimals: 0,
      },
      12.345
    )
    assert.deepEqual(quote, {
      charged_amount: 1852,
      charged_amount_usd: 1852 / 150,
      currency: 'JPY',
      rate_to_usd: 150,
      rounding_decimals: 0,
    })
    assert.ok(quote)
    assert.equal(formatPaymentQuoteAmount(quote), '~ ¥1,852')
  })

  test('rounds display amounts up to provider precision', () => {
    const quote = getPaymentQuoteDisplay(
      {
        type: 'stripe',
        name: 'Card',
        currency: 'USD',
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
        rounding_decimals: 2,
      },
      1.001
    )
    assert.equal(quote?.charged_amount, 1.01)
    assert.equal(quote?.charged_amount_usd, 1.01)
    assert.ok(quote)
    assert.equal(formatPaymentQuoteAmount(quote), '$1.01')
  })

  test('uses preloaded Waffo display config for a non-USD quote', () => {
    const method = getWaffoPaymentMethod({
      name: 'Waffo JPY',
      currency: 'JPY',
      rate_to_usd: 150,
      base_amount_multiplier: 2,
      topup_ratio: 1.1,
      rounding_decimals: 0,
    })

    assert.equal(hasPaymentMethodDisplayConfig(method), true)
    assert.deepEqual(getPaymentQuoteDisplay(method, 10), {
      charged_amount: 3300,
      charged_amount_usd: 22,
      currency: 'JPY',
      rate_to_usd: 150,
      rounding_decimals: 0,
    })
    const quote = getPaymentMethodDisplayQuote(10, method)
    assert.equal(quote?.commissionUSD, 2)
    assert.equal(quote?.creditedAmountUSD, 20)
  })

  test('rejects payment methods without usable preloaded display config', () => {
    assert.equal(
      hasPaymentMethodDisplayConfig({ type: 'stripe', name: 'Card' }),
      false
    )
    assert.equal(
      hasPaymentMethodDisplayConfig({
        type: 'stripe',
        name: 'Card',
        rate_to_usd: 1,
        base_amount_multiplier: 1,
        topup_ratio: 1,
        rounding_decimals: 2,
      }),
      false,
      'a missing settlement currency must not default to USD'
    )
    assert.equal(
      hasPaymentMethodDisplayConfig({
        type: 'stripe',
        name: 'Card',
        currency: 'USD',
        rate_to_usd: 0,
        base_amount_multiplier: 1,
        topup_ratio: 1,
        rounding_decimals: 2,
      }),
      false
    )
  })

  test('uses payment base amount for cashback tiers when multiplier is configured', () => {
    const cashback = [{ min_amount: 10, cashback_percent: 2 }]
    const amount = getCashbackDisplayAmount(
      5,
      {
        type: 'stripe',
        name: 'Card',
        currency: 'USD',
        rate_to_usd: 1,
        base_amount_multiplier: 2,
        topup_ratio: 1,
        rounding_decimals: 2,
      },
      cashback
    )

    assert.equal(amount, 10)
    assert.deepEqual(
      getCashbackTierSummary(amount, cashback).current,
      cashback[0]
    )
  })
})

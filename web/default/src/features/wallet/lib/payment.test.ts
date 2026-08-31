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
  backendAmountToWalletDisplay,
  walletDisplayAmountToBackend,
} from '@/lib/currency'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import type { TopupInfo } from '../types'
import {
  calculateCashbackAmount,
  calculatePresetPricing,
  formatCashbackCredit,
  getPaymentCurrencyLabel,
} from './format'
import {
  generatePresetAmounts,
  getCashbackPercentForAmount,
  getCashbackTierSummary,
  getMinTopupAmount,
  getPaymentMethodMinimumAmount,
  getMinimumAvailablePaymentMethodAmount,
  isPaymentMethodAmountEligible,
  getPaymentMethodDisplayQuote,
  getPaymentCheckoutKind,
  getPaymentErrorMessage,
  getUSDTTrc20DisplayStatus,
  isWaffoPayment,
  isSafeHttpRedirectUrl,
  redirectToPaymentPage,
  isSafeInternalUSDTTrc20Url,
  isSafeInternalDirectUSDTUrl,
  isUSDTTrc20TerminalStatus,
  isDirectUSDTPayment,
  getDirectUSDTNetwork,
  normalizeCashbackTiers,
  submitPaymentForm,
} from './payment'

describe('payment redirect URL validation', () => {
  test('allows only internal USDT TRC20 payment routes', () => {
    assert.equal(isSafeInternalUSDTTrc20Url('/usdt-trc20/NOWabc123'), true)
    assert.equal(
      isSafeInternalUSDTTrc20Url('https://evil.test/usdt-trc20/x'),
      false
    )
    assert.equal(isSafeInternalUSDTTrc20Url('//evil.test/usdt-trc20/x'), false)
    assert.equal(
      isSafeInternalUSDTTrc20Url('/usdt-trc20/x?redirect=https://evil.test'),
      false
    )
    assert.equal(isSafeInternalUSDTTrc20Url('/usdt-trc20/x%2Fy'), false)
  })
  test('allows absolute HTTPS provider redirects', () => {
    assert.equal(
      isSafeHttpRedirectUrl(' https://checkout.example.test/pay?id=123 '),
      true
    )
  })

  test('rejects executable and data URLs', () => {
    assert.equal(isSafeHttpRedirectUrl('javascript:alert(1)'), false)
    assert.equal(
      isSafeHttpRedirectUrl('data:text/html,<script>alert(1)</script>'),
      false
    )
  })

  test('navigates safe provider redirects in the current tab only', () => {
    let navigatedTo = ''
    const navigate = (url: string) => {
      navigatedTo = url
    }

    assert.equal(
      redirectToPaymentPage(
        ' https://checkout.example.test/pay?id=123 ',
        navigate
      ),
      true
    )
    assert.equal(navigatedTo, 'https://checkout.example.test/pay?id=123')

    assert.equal(redirectToPaymentPage('javascript:alert(1)', navigate), false)
    assert.equal(navigatedTo, 'https://checkout.example.test/pay?id=123')
  })

  test('submits EPay form in the current tab', () => {
    const previousDocument = globalThis.document
    let submitted = false
    const form = {
      action: '',
      method: '',
      target: '',
      appendChild: () => null,
      submit: () => {
        submitted = true
      },
    }
    const fakeDocument = {
      createElement: (tagName: string) =>
        tagName === 'form' ? form : { type: '', name: '', value: '' },
      body: {
        appendChild: () => null,
        removeChild: () => null,
      },
    }

    Object.defineProperty(globalThis, 'document', {
      configurable: true,
      value: fakeDocument,
    })
    try {
      submitPaymentForm('https://epay.example.test/pay', { order: '123' })
      assert.equal(form.action, 'https://epay.example.test/pay')
      assert.equal(form.method, 'POST')
      assert.equal(form.target, '')
      assert.equal(submitted, true)
    } finally {
      Object.defineProperty(globalThis, 'document', {
        configurable: true,
        value: previousDocument,
      })
    }
  })
})

describe('direct USDT payment statuses', () => {
  test('maps direct crypto method ids to allowlisted networks', () => {
    assert.equal(isDirectUSDTPayment('crypto_direct'), true)
    assert.equal(isDirectUSDTPayment('usdt_trc20_direct'), true)
    assert.equal(isDirectUSDTPayment('usdt_ton_direct'), true)
    assert.equal(getDirectUSDTNetwork('usdt_solana_direct'), 'SOLANA')
    assert.equal(isDirectUSDTPayment('usdt_bep20_direct'), false)
  })
  test('allows only allowlisted multichain checkout routes', () => {
    assert.equal(isSafeInternalDirectUSDTUrl('/crypto/tron/trade-1'), true)
    assert.equal(isSafeInternalDirectUSDTUrl('/crypto/ton/trade-1'), true)
    assert.equal(isSafeInternalDirectUSDTUrl('/crypto/solana/trade-1'), true)
    assert.equal(isSafeInternalDirectUSDTUrl('/crypto/bitcoin/trade-1'), false)
    assert.equal(isSafeInternalDirectUSDTUrl('/crypto/ton/trade-1?x=1'), false)
    assert.equal(isSafeInternalDirectUSDTUrl('/usdt-trc20/trade-1'), true)
  })
  test('treats paid as terminal and displays it as confirmed', () => {
    assert.equal(isUSDTTrc20TerminalStatus('paid'), true)
    assert.equal(isUSDTTrc20TerminalStatus('pending'), false)
    assert.equal(getUSDTTrc20DisplayStatus('paid', false), 'success')
    assert.equal(getUSDTTrc20DisplayStatus('pending', true), 'expired')
  })
})

describe('payment checkout dispatch', () => {
  test('routes Waffo methods to the dedicated checkout instead of EPay', () => {
    assert.equal(isWaffoPayment('waffo'), true)
    assert.equal(isWaffoPayment('alipay'), false)
    assert.equal(getPaymentCheckoutKind('waffo'), 'waffo')
    assert.equal(getPaymentCheckoutKind('waffo_pancake'), 'waffo-pancake')
    assert.equal(getPaymentCheckoutKind('alipay'), 'generic')
  })
})

describe('payment error messages', () => {
  test('uses safe endpoint details instead of the legacy error literal', () => {
    assert.equal(
      getPaymentErrorMessage(
        { message: 'error', data: 'Payment method is unavailable' },
        'Payment request failed'
      ),
      'Payment method is unavailable'
    )
  })

  test('falls back for generic, HTML, and credential-bearing provider details', () => {
    for (const response of [
      { message: 'error' },
      { message: 'error', data: '<strong>provider error</strong>' },
      { message: 'error', data: 'secret token: value' },
    ]) {
      assert.equal(
        getPaymentErrorMessage(response, 'Payment request failed'),
        'Payment request failed'
      )
    }
  })
})

describe('wallet cashback', () => {
  test('uses the highest matching minimum amount threshold', () => {
    const cashback = [
      { min_amount: 0.1, cashback_percent: 0.5 },
      { min_amount: 100, cashback_percent: 1 },
      { min_amount: 150, cashback_percent: 2 },
      { min_amount: 300, cashback_percent: 3 },
    ]

    assert.equal(getCashbackPercentForAmount(0.1, cashback), 0.5)
    assert.equal(getCashbackPercentForAmount(0.09, cashback), 0)
    assert.equal(getCashbackPercentForAmount(100, cashback), 1)
    assert.equal(getCashbackPercentForAmount(149, cashback), 1)
    assert.equal(getCashbackPercentForAmount(150, cashback), 2)
    assert.equal(getCashbackPercentForAmount(500, cashback), 3)
  })

  test('allows a higher zero-percent tier to disable cashback', () => {
    const cashback = [
      { min_amount: 10, cashback_percent: 1 },
      { min_amount: 20, cashback_percent: 0 },
    ]

    assert.equal(getCashbackPercentForAmount(19.99, cashback), 1)
    assert.equal(getCashbackPercentForAmount(20, cashback), 0)
    assert.equal(getCashbackPercentForAmount(100, cashback), 0)
  })

  test('keeps zero-percent tier behavior in the payment preview', () => {
    const method = {
      type: 'test',
      name: 'Test',
      currency: 'USD',
      rate_to_usd: 1,
      base_amount_multiplier: 1,
      topup_ratio: 1,
      rounding_decimals: 2,
    }
    const cashback = [
      { min_amount: 10, cashback_percent: 1 },
      { min_amount: 20, cashback_percent: 0 },
    ]

    assert.equal(
      getPaymentMethodDisplayQuote(15, method, cashback)?.cashbackPercent,
      1
    )
    assert.equal(
      getPaymentMethodDisplayQuote(25, method, cashback)?.cashbackPercent,
      0
    )
    assert.equal(
      getPaymentMethodDisplayQuote(25, method, cashback)?.cashbackAmountUSD,
      0
    )
  })

  test('ignores invalid legacy tiers while retaining valid zero tiers', () => {
    const cashback = normalizeCashbackTiers([
      { min_amount: -1, cashback_percent: 5 },
      { min_amount: 10, cashback_percent: 1 },
      { min_amount: 20, cashback_percent: 0 },
      { min_amount: 30, cashback_percent: 101 },
      { min_amount: 40, cashback_percent: Number.NaN },
    ])

    assert.deepEqual(cashback, [
      { min_amount: 10, cashback_percent: 1 },
      { min_amount: 20, cashback_percent: 0 },
    ])
    assert.equal(getCashbackPercentForAmount(25, cashback), 0)
  })

  test('exposes current and next cashback tiers with bounded progress', () => {
    const cashback = [
      { min_amount: 5, cashback_percent: 1 },
      { min_amount: 20, cashback_percent: 2 },
      { min_amount: 50, cashback_percent: 4 },
    ]

    assert.deepEqual(getCashbackTierSummary(10, cashback), {
      current: cashback[0],
      next: cashback[1],
      progress: (5 / 15) * 100,
    })
    assert.equal(getCashbackTierSummary(100, cashback).progress, 100)
    assert.equal(getCashbackTierSummary(1, cashback).current, null)
  })

  test('calculates the balance credit without changing the payment amount', () => {
    assert.equal(calculateCashbackAmount(100, 1), 1)
    assert.equal(calculateCashbackAmount(100, 2.5), 2.5)
    assert.equal(calculateCashbackAmount(100, 0), 0)

    const preset = calculatePresetPricing(100, 7)
    assert.equal(preset.actualPrice, 700)
    assert.equal(calculateCashbackAmount(100, 1), 1)
  })

  test('formats cashback in the configured balance unit', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency

    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CNY',
          usdExchangeRate: 7,
        },
      })
      assert.equal(formatCashbackCredit(1), '¥7')

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CUSTOM',
          customCurrencySymbol: '€',
          customCurrencyExchangeRate: 0.9,
        },
      })
      assert.equal(formatCashbackCredit(1), '€ 0.9')

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      })
      const tokenCashback = calculateCashbackAmount(100000, 1)
      assert.equal(tokenCashback, 1000)
      assert.equal(formatCashbackCredit(tokenCashback), '1000')
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })
})

describe('payment method display quote', () => {
  test('matches the decimal backend formula for fractional amounts', () => {
    const quote = getPaymentMethodDisplayQuote(0.1, {
      type: 'waffo',
      name: 'Waffo',
      currency: 'RUB',
      rate_to_usd: 90,
      base_amount_multiplier: 1.1,
      topup_ratio: 1,
      rounding_decimals: 2,
    })

    assert.deepEqual(quote, {
      currency: 'RUB',
      baseAmountUSD: 0.11,
      commissionUSD: 0,
      creditedAmountUSD: 0.11,
      cashbackPercent: 0,
      cashbackAmountUSD: 0,
      chargedAmountUSD: 0.11,
      chargedAmount: 9.9,
    })
  })

  test('keeps the decimal commission after provider rounding', () => {
    const quote = getPaymentMethodDisplayQuote(0.1, {
      type: 'yookassa_sbp',
      name: 'СБП',
      currency: 'RUB',
      rate_to_usd: 90,
      base_amount_multiplier: 1,
      topup_ratio: 1.1,
      rounding_decimals: 2,
    })

    assert.equal(quote?.baseAmountUSD, 0.1)
    assert.equal(quote?.commissionUSD, 0.01)
    assert.equal(quote?.chargedAmountUSD, 0.11)
    assert.equal(quote?.chargedAmount, 9.9)
  })

  test('normalizes a coefficient below one to a fee-free gross amount', () => {
    const method = {
      type: 'test',
      name: 'Test',
      currency: 'USD',
      rate_to_usd: 1,
      base_amount_multiplier: 1,
      topup_ratio: 0.8,
      rounding_decimals: 2,
    }
    assert.deepEqual(getPaymentMethodDisplayQuote(10, method), {
      currency: 'USD',
      baseAmountUSD: 10,
      commissionUSD: 0,
      creditedAmountUSD: 10,
      cashbackPercent: 0,
      cashbackAmountUSD: 0,
      chargedAmountUSD: 10,
      chargedAmount: 10,
    })
  })
})

describe('payment currency display', () => {
  test('uses configured symbols and falls back to the currency code', () => {
    assert.equal(getPaymentCurrencyLabel('RUB'), '₽')
    assert.equal(getPaymentCurrencyLabel('eur', '€'), '€')
    assert.equal(getPaymentCurrencyLabel('xyz'), 'XYZ')
    assert.equal(getPaymentCurrencyLabel(undefined), '$')
  })

  test('converts wallet display input to backend accounting units', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency

    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'CNY',
          usdExchangeRate: 7,
        },
      })
      assert.equal(walletDisplayAmountToBackend(70), 10)
      assert.equal(backendAmountToWalletDisplay(10), 70)

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      })
      assert.equal(walletDisplayAmountToBackend(500000), 500000)
      assert.equal(backendAmountToWalletDisplay(500000), 500000)
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })
})

describe('wallet minimum top-up', () => {
  test('converts settlement minimums to wallet USD units for mixed methods', () => {
    const rubMethod = {
      type: 'yookassa_sbp',
      name: 'SBP',
      currency: 'RUB',
      min_topup: 90,
      rate_to_usd: 90,
      base_amount_multiplier: 1,
      topup_ratio: 1,
    }
    const usdMethod = {
      type: 'stripe',
      name: 'Stripe',
      currency: 'USD',
      min_topup: 5,
      rate_to_usd: 1,
      base_amount_multiplier: 1,
      topup_ratio: 1,
    }
    const info = {
      min_topup: 1,
      pay_methods: [rubMethod, usdMethod],
    } as TopupInfo

    assert.equal(getPaymentMethodMinimumAmount(rubMethod), 1)
    assert.equal(getPaymentMethodMinimumAmount(usdMethod), 5)
    assert.equal(getMinimumAvailablePaymentMethodAmount(info), 1)
    assert.equal(isPaymentMethodAmountEligible(1, rubMethod), true)
    assert.equal(isPaymentMethodAmountEligible(1, usdMethod), false)
    assert.equal(isPaymentMethodAmountEligible(5, usdMethod), true)
  })

  test('fails closed when a configured minimum lacks quote conversion', () => {
    const method = { type: 'stripe', name: 'Stripe', min_topup: 5 }
    assert.equal(getPaymentMethodMinimumAmount(method), null)
    assert.equal(isPaymentMethodAmountEligible(100, method), false)
  })

  test('keeps fractional online minimum amount', () => {
    const minimum = getMinTopupAmount({
      enable_online_topup: true,
      min_topup: 0.1,
    } as TopupInfo)

    assert.equal(minimum, 0.1)
    assert.deepEqual(
      generatePresetAmounts(minimum)
        .slice(0, 2)
        .map(({ value }) => value),
      [0.1, 0.5]
    )
  })

  test('enforces the direct USDT minimum in USD', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency
    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'USD',
        },
      })
      const method = {
        type: 'usdt_trc20_direct',
        name: 'USDT TRC20',
        min_topup: 10,
      }
      const topupInfo = {
        enable_online_topup: true,
        min_topup: 1,
        pay_methods: [method],
      } as TopupInfo

      assert.equal(getPaymentMethodMinimumAmount(method), 10)
      assert.equal(getMinTopupAmount(topupInfo, method), 10)
      assert.equal(getMinTopupAmount(topupInfo), 10)
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })

  test('keeps the generic USD minimum for non-direct provider methods', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency
    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'USD',
        },
      })
      const method = {
        type: 'yookassa_sbp',
        name: 'SBP',
        currency: 'RUB',
        min_topup: 100,
      }
      const topupInfo = {
        enable_online_topup: true,
        min_topup: 1,
        pay_methods: [method],
      } as TopupInfo

      assert.equal(getPaymentMethodMinimumAmount(method), null)
      assert.equal(getMinTopupAmount(topupInfo, method), 1)
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })

  test('converts direct USDT minimum into token display units', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency
    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      })
      const method = {
        type: 'usdt_trc20_direct',
        name: 'USDT TRC20',
        min_topup: 10,
      }
      assert.equal(getPaymentMethodMinimumAmount(method), 5_000_000)
      assert.equal(
        getMinTopupAmount({ pay_methods: [method] } as TopupInfo),
        5_000_000
      )
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })

  test('renders direct USDT quote in USD and token input units', () => {
    const originalCurrency = useSystemConfigStore.getState().config.currency
    const method = {
      type: 'usdt_trc20_direct',
      name: 'USDT TRC20',
      min_topup: 10,
    }
    try {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'USD',
        },
      })
      assert.equal(getPaymentMethodDisplayQuote(12, method)?.chargedAmount, 12)

      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: 'TOKENS',
          quotaPerUnit: 500000,
        },
      })
      assert.equal(
        getPaymentMethodDisplayQuote(6_000_000, method)?.chargedAmount,
        12
      )
    } finally {
      useSystemConfigStore.getState().setConfig({ currency: originalCurrency })
    }
  })
})

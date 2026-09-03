import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PlatformCurrency } from '../general/platform-currencies-api'
import {
  getPreferredPaymentCurrency,
  getSupportedPaymentCurrencies,
  normalizePaymentMethodCurrency,
  usesFixedPaymentCurrency,
} from './payment-method-currency'
import type { TopupGroupOption } from './payment-method-dialog'
import { isConfiguredTopupGroup } from './payment-method-group'
import { getPaymentTypeOptions } from './payment-method-options'
import {
  getPaymentMethodPendingTtl,
  isPaymentMethodData,
} from './payment-methods-visual-editor'

const currencies: PlatformCurrency[] = [
  {
    code: 'USD',
    name: 'US Dollar',
    symbol: '$',
    enabled: true,
    sync_enabled: false,
    sync_provider: '',
    manual_rate_to_usd: 1,
    rate_to_usd: 1,
  },
  {
    code: 'RUB',
    name: 'Russian Ruble',
    symbol: '₽',
    enabled: true,
    sync_enabled: true,
    sync_provider: 'cbr',
    manual_rate_to_usd: 90,
    rate_to_usd: 90,
  },
  {
    code: 'EUR',
    name: 'Euro',
    symbol: '€',
    enabled: true,
    sync_enabled: true,
    sync_provider: 'cbr',
    manual_rate_to_usd: 0.92,
    rate_to_usd: 0.92,
  },
  {
    code: 'USDT',
    name: 'Tether',
    symbol: '₮',
    enabled: true,
    sync_enabled: true,
    sync_provider: 'bybit_p2p',
    manual_rate_to_usd: 1,
    rate_to_usd: 1,
  },
]

describe('payment method currency capabilities', () => {
  test('offers YooKassa SBP with its RUB default', () => {
    const yooKassa = getPaymentTypeOptions((key) => key).find(
      (option) => option.value === 'yookassa_sbp'
    )

    assert.deepEqual(yooKassa, {
      iconName: 'LuCreditCard',
      label: 'СБП / YooKassa',
      name: 'СБП / YooKassa',
      value: 'yookassa_sbp',
    })
    assert.equal(getPreferredPaymentCurrency('yookassa_sbp'), 'RUB')
  })

  test('offers every built-in gateway including the Crypto catalog method', () => {
    const types = getPaymentTypeOptions((key) => key).map(
      (option) => option.value
    )
    assert.deepEqual(types, [
      'yookassa_sbp',
      'alipay',
      'wxpay',
      'stripe',
      'waffo_pancake',
      'waffo',
      'nowpayments',
      'manual_transfer',
      'crypto_direct',
    ])
    const crypto = getPaymentTypeOptions((key) => key).find(
      (option) => option.value === 'crypto_direct'
    )
    assert.equal(crypto?.label, 'Crypto')
    assert.equal(crypto?.name, 'Crypto')
    assert.equal(getPreferredPaymentCurrency('crypto_direct'), 'USDT')
    assert.equal(usesFixedPaymentCurrency('crypto_direct'), true)
  })

  test('keeps an auto-created YooKassa SBP method editable', () => {
    const method = {
      name: 'СБП / YooKassa',
      type: 'yookassa_sbp',
      currency: 'RUB',
      topup_group: 'sbp-commission',
    }

    assert.equal(isPaymentMethodData(method), true)
    assert.equal(method.currency, 'RUB')
    assert.equal(method.topup_group, 'sbp-commission')
  })

  test('accepts the administrator-only visibility flag', () => {
    assert.equal(
      isPaymentMethodData({
        name: 'Internal',
        type: 'custom',
        admin_only: true,
      }),
      true
    )
    assert.equal(
      isPaymentMethodData({
        name: 'Persisted internal',
        type: 'custom',
        admin_only: 'true',
      }),
      true
    )
    assert.equal(
      isPaymentMethodData({
        name: 'Invalid internal',
        type: 'custom',
        admin_only: 'yes',
      }),
      false
    )
  })

  test('requires a configured top-up coefficient group', () => {
    const groups: TopupGroupOption[] = [
      { name: 'default', ratio: 1 },
      { name: 'premium', ratio: 1.2 },
    ]

    assert.equal(isConfiguredTopupGroup('default', groups), true)
    assert.equal(isConfiguredTopupGroup(' premium ', groups), true)
    assert.equal(isConfiguredTopupGroup('unknown', groups), false)
    assert.equal(isConfiguredTopupGroup('', groups), false)
    assert.equal(isConfiguredTopupGroup('default', []), false)
  })

  test('resolves individual and provider payment TTL defaults', () => {
    assert.equal(
      getPaymentMethodPendingTtl({ name: 'Card', type: 'stripe' }, 1440),
      1440
    )
    assert.equal(
      getPaymentMethodPendingTtl(
        { name: 'Card', type: 'stripe', pending_ttl_minutes: '30' },
        1440
      ),
      30
    )
    assert.equal(
      getPaymentMethodPendingTtl({ name: 'SBP', type: 'yookassa_sbp' }, 1440),
      15
    )
  })

  test('keeps fixed gateway currencies selectable but constrained', () => {
    assert.deepEqual(
      getSupportedPaymentCurrencies('alipay', currencies).map(
        (item) => item.code
      ),
      ['USD']
    )
    assert.deepEqual(
      getSupportedPaymentCurrencies('stripe', currencies).map(
        (item) => item.code
      ),
      ['USD']
    )
    assert.deepEqual(
      getSupportedPaymentCurrencies('yookassa_sbp', currencies).map(
        (item) => item.code
      ),
      ['RUB']
    )
    assert.deepEqual(
      getSupportedPaymentCurrencies('wxpay', currencies).map(
        (item) => item.code
      ),
      ['USD']
    )
    assert.deepEqual(
      getSupportedPaymentCurrencies('waffo_pancake', currencies).map(
        (item) => item.code
      ),
      ['USD']
    )
  })

  test('normalizes every fixed provider to its settlement currency', () => {
    assert.deepEqual(
      getSupportedPaymentCurrencies('nowpayments', currencies, 'EUR').map(
        (item) => item.code
      ),
      ['USDT']
    )
    assert.equal(getPreferredPaymentCurrency('nowpayments'), 'USDT')
    assert.equal(usesFixedPaymentCurrency('nowpayments'), true)
    assert.equal(normalizePaymentMethodCurrency('nowpayments', 'EUR'), 'USDT')
    assert.equal(usesFixedPaymentCurrency('yookassa_sbp'), true)
    assert.equal(usesFixedPaymentCurrency('stripe'), true)
    assert.equal(usesFixedPaymentCurrency('alipay'), true)
    assert.equal(usesFixedPaymentCurrency('waffo'), true)
    assert.equal(normalizePaymentMethodCurrency('yookassa_sbp', 'USD'), 'RUB')
    assert.equal(normalizePaymentMethodCurrency('stripe', 'RUB'), 'USD')
    assert.equal(normalizePaymentMethodCurrency('waffo', 'USD', 'EUR'), 'EUR')
    assert.deepEqual(
      getSupportedPaymentCurrencies('waffo', currencies, 'EUR').map(
        (item) => item.code
      ),
      ['EUR']
    )
  })
})

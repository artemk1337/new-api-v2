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

import type { TopupInfo } from '../types'
import {
  calculateCashbackAmount,
  calculatePresetPricing,
  formatCashbackCredit,
} from './format'
import {
  generatePresetAmounts,
  getCashbackPercentForAmount,
  getMinTopupAmount,
} from './payment'

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

describe('wallet minimum top-up', () => {
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
})

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
import { formatCurrencyFromUSD } from '@/lib/currency'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'
// ============================================================================
// Wallet-specific Formatting Functions
// ============================================================================

/**
 * Format Creem price with currency symbol (USD/EUR)
 */
export function formatCreemPrice(
  price: number,
  currency: 'USD' | 'EUR'
): string {
  const symbol = currency === 'EUR' ? '€' : '$'
  return `${symbol}${price.toFixed(2)}`
}

/**
 * Format large quota numbers with K/M suffix
 */
export function formatQuotaShort(quota: number): string {
  if (quota >= 1000000) {
    return `${(quota / 1000000).toFixed(1)}M`
  }
  if (quota >= 1000) {
    return `${(quota / 1000).toFixed(1)}K`
  }
  return quota.toString()
}

/**
 * Format currency amount that is already in local currency.
 * This is used for payment amounts that have been calculated via priceRatio.
 */
export function formatCurrency(amount: number | string): string {
  const numeric =
    typeof amount === 'number' ? amount : Number.parseFloat(String(amount))
  if (!Number.isFinite(numeric)) return '-'

  return new Intl.NumberFormat(undefined, {
    minimumFractionDigits: 0,
    maximumFractionDigits: Math.abs(numeric) >= 1 ? 2 : 4,
  }).format(numeric)
}

/**
 */
export function calculatePresetPricing(
  presetValue: number,
  priceRatio: number,
  usdExchangeRate: number = 1
) {
  const originalPrice = presetValue * priceRatio
  const displayValue = presetValue * usdExchangeRate

  return {
    displayValue,
    originalPrice,
    actualPrice: originalPrice,
  }
}

export function calculateCashbackAmount(
  amount: number,
  cashbackPercent: number
): number {
  if (!Number.isFinite(amount) || !Number.isFinite(cashbackPercent)) {
    return 0
  }
  return Math.max(0, (amount * cashbackPercent) / 100)
}

export function formatCashbackCredit(amount: number): string {
  const currency = useSystemConfigStore.getState().config.currency
  const amountUSD =
    currency.quotaDisplayType === 'TOKENS'
      ? amount /
        (currency.quotaPerUnit > 0
          ? currency.quotaPerUnit
          : DEFAULT_CURRENCY_CONFIG.quotaPerUnit)
      : amount

  return formatCurrencyFromUSD(amountUSD, {
    digitsLarge: 2,
    digitsSmall: 4,
    abbreviate: false,
  })
}

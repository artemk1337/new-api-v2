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
import { formatBillingCurrencyFromUSD } from '@/lib/currency'

import { FILTER_ALL, TOKEN_UNIT_DIVISORS } from '../constants'
import type {
  PricingGroupPrice,
  PricingModel,
  PricingRates,
  PricingTier,
  TokenUnit,
} from '../types'

export type PricingRateKey = keyof PricingRates

export const PRICING_RATE_LABELS: { key: PricingRateKey; label: string }[] = [
  { key: 'input_usd', label: 'Input' },
  { key: 'output_usd', label: 'Output' },
  { key: 'cache_read_usd', label: 'Cached input' },
  { key: 'cache_write_usd', label: 'Cache write' },
  { key: 'cache_write_1h_usd', label: 'Cache Write (1h)' },
  { key: 'image_usd', label: 'Image input' },
  { key: 'image_output_usd', label: 'Image output' },
  { key: 'audio_input_usd', label: 'Audio input' },
  { key: 'audio_output_usd', label: 'Audio output' },
  { key: 'reasoning_output_usd', label: 'Reasoning output' },
  { key: 'price_usd', label: 'Price' },
]

export type RetailPriceOptions = {
  tokenUnit: TokenUnit
  showRechargePrice?: boolean
  priceRate?: number
  usdExchangeRate?: number
}

export function hasRetailRate(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0
}

export function isTokenGroupPrice(price: PricingGroupPrice): boolean {
  return price.price_unit === 'million_tokens'
}

export function getGroupPriceAmount(price: PricingGroupPrice): number | null {
  const value = isTokenGroupPrice(price) ? price.input_usd : price.price_usd
  return hasRetailRate(value) && !price.unavailable ? value : null
}

export function compareGroupPrices(
  a: PricingGroupPrice,
  b: PricingGroupPrice
): number {
  // Amounts with different billing units cannot be compared as one price.
  const unitDifference = a.price_unit.localeCompare(b.price_unit)
  if (unitDifference) return unitDifference
  const aAmount = getGroupPriceAmount(a)
  const bAmount = getGroupPriceAmount(b)
  if (aAmount === null && bAmount !== null) return 1
  if (aAmount !== null && bAmount === null) return -1
  if (aAmount !== null && bAmount !== null && aAmount !== bAmount) {
    return aAmount - bAmount
  }
  if (hasRetailRate(a.output_usd) && hasRetailRate(b.output_usd)) {
    const outputDifference = a.output_usd - b.output_usd
    if (outputDifference) return outputDifference
  }
  return a.group.localeCompare(b.group, undefined, { numeric: true })
}

export function getDisplayGroupPrice(
  model: PricingModel,
  groupFilter?: string
): PricingGroupPrice | undefined {
  const prices = model.group_prices ?? []
  if (groupFilter && groupFilter !== FILTER_ALL) {
    return prices.find((price) => price.group === groupFilter)
  }
  return [...prices]
    .sort(compareGroupPrices)
    .find((price) => !price.unavailable)
}

export function getPricingRateFields(
  rows: PricingRates[]
): typeof PRICING_RATE_LABELS {
  return PRICING_RATE_LABELS.filter((field) =>
    rows.some((row) => hasRetailRate(row[field.key]))
  )
}

export function hasTwoContextTiers(tiers: PricingTier[] | undefined): boolean {
  if (!tiers || tiers.length !== 2) return false
  const [short, long] = tiers
  return (
    (short.min_input_tokens ?? 0) === 0 &&
    typeof short.max_input_tokens === 'number' &&
    long.min_input_tokens === short.max_input_tokens + 1
  )
}

export function formatRetailPrice(
  value: number | null | undefined,
  options: RetailPriceOptions,
  tokenBased = true
): string {
  if (!hasRetailRate(value)) return '—'
  let amount = value
  if (options.showRechargePrice) {
    amount *= (options.priceRate ?? 1) / (options.usdExchangeRate ?? 1)
  }
  if (tokenBased) amount /= TOKEN_UNIT_DIVISORS[options.tokenUnit]
  return formatBillingCurrencyFromUSD(amount, {
    digitsLarge: 4,
    digitsSmall: 6,
    abbreviate: false,
  })
}

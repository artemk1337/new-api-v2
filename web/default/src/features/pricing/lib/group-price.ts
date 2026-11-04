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

import { FILTER_ALL, QUOTA_TYPES, TOKEN_UNIT_DIVISORS } from '../constants'
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
  if (price.unavailable) return null
  if (isTokenGroupPrice(price)) return getTokenPairAmount(price)
  return hasRetailRate(price.price_usd) ? price.price_usd : null
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

function getTokenPairAmount(price: PricingRates): number | null {
  if (!hasRetailRate(price.input_usd) || !hasRetailRate(price.output_usd)) {
    return null
  }
  const amount = price.input_usd + price.output_usd
  return Number.isFinite(amount) ? amount : null
}

export function getDisplayGroupPrice(
  model: PricingModel,
  groupFilter?: string,
  quotaTypeFilter: string = QUOTA_TYPES.ALL
): PricingGroupPrice | undefined {
  const selectedGroup = Boolean(groupFilter && groupFilter !== FILTER_ALL)
  let cheapest: PricingGroupPrice | undefined
  for (const price of model.group_prices ?? []) {
    if (selectedGroup && price.group !== groupFilter) continue
    if (
      quotaTypeFilter !== QUOTA_TYPES.ALL &&
      isTokenGroupPrice(price) !== (quotaTypeFilter === QUOTA_TYPES.TOKEN)
    ) {
      continue
    }
    if (selectedGroup) return price
    if (
      !price.unavailable &&
      (!cheapest || compareGroupPrices(price, cheapest) < 0)
    ) {
      cheapest = price
    }
  }
  return cheapest
}

export function getComparisonReference(
  model: PricingModel
): { input_usd: number; output_usd: number } | null {
  const references = [
    model.official_price?.comparison_enabled ? model.official_price : null,
    model.base_reference,
  ]
  for (const reference of references) {
    if (
      reference?.price_unit === 'million_tokens' &&
      hasRetailRate(reference.input_usd) &&
      hasRetailRate(reference.output_usd) &&
      (getTokenPairAmount(reference) ?? 0) > 0
    ) {
      return {
        input_usd: reference.input_usd,
        output_usd: reference.output_usd,
      }
    }
  }
  return null
}

export function getGroupPriceBenefit(
  model: PricingModel,
  price: PricingGroupPrice | undefined
): number | null {
  const reference = getComparisonReference(model)
  if (!reference || !price || price.unavailable || !isTokenGroupPrice(price)) {
    return null
  }
  const base = reference.input_usd + reference.output_usd
  const amount = getTokenPairAmount(price)
  return amount === null ? null : Math.max(0, (1 - amount / base) * 100)
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

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
import type { RatioType } from '../types'
import type { ModelPricingSnapshot } from './model-pricing-snapshots'
import {
  RATIO_SYNC_FIELDS,
  type RatioDifferenceEntry,
} from './upstream-ratio-sync-helpers'

type PricingValues = Partial<Record<RatioType, number | string | null>>
type BillingMode = 'per-token' | 'per-request' | 'tiered_expr'

const snapshotFields: Partial<Record<RatioType, keyof ModelPricingSnapshot>> = {
  model_price: 'price',
  model_ratio: 'ratio',
  completion_ratio: 'completionRatio',
  cache_ratio: 'cacheRatio',
  create_cache_ratio: 'createCacheRatio',
  image_ratio: 'imageRatio',
  audio_ratio: 'audioRatio',
  audio_completion_ratio: 'audioCompletionRatio',
}

function numberValue(value: number | string | null | undefined): number | null {
  if (value === null || value === undefined || value === '') return null
  const number = Number(value)
  return Number.isFinite(number) && number >= 0 ? number : null
}

function priceForField(
  values: PricingValues,
  field: RatioType,
  mode: BillingMode,
  quotaPerUnit: number
): number | null {
  if (field === 'model_price') {
    return mode === 'per-request' ? numberValue(values.model_price) : null
  }
  if (mode !== 'per-token' || !RATIO_SYNC_FIELDS.includes(field)) return null
  if (!Number.isFinite(quotaPerUnit) || quotaPerUnit <= 0) return null
  const base = numberValue(values.model_ratio)
  if (base === null) return null
  const inputPrice = (base * 1_000_000) / quotaPerUnit
  if (field === 'model_ratio') {
    return Number.isFinite(inputPrice) ? inputPrice : null
  }

  const defaultedFields: RatioType[] = [
    'image_ratio',
    'audio_ratio',
    'audio_completion_ratio',
  ]
  const ratio = numberValue(
    values[field] ?? (defaultedFields.includes(field) ? 1 : null)
  )
  if (ratio === null) return null
  let price = inputPrice * ratio
  if (field === 'audio_completion_ratio') {
    const inputAudio = numberValue(values.audio_ratio ?? 1)
    if (inputAudio === null) return null
    price *= inputAudio
  }
  return Number.isFinite(price) ? price : null
}

export function getPricingSyncPricePreview(
  snapshot: ModelPricingSnapshot | undefined,
  differences: Partial<Record<RatioType, RatioDifferenceEntry>>,
  resolutions: Record<string, number | string>,
  field: RatioType,
  sourceName: string | null,
  quotaPerUnit = 500_000
): { current: number | null; proposed: number | null } {
  const current: PricingValues = {}
  for (const [key, snapshotKey] of Object.entries(snapshotFields)) {
    const ratioType = key as RatioType
    const value = snapshot?.[snapshotKey]
    if (typeof value === 'string' && value !== '') current[ratioType] = value
    const difference = differences[ratioType]
    if (difference) current[ratioType] = difference.current
  }
  let currentMode: BillingMode = 'per-token'
  if (snapshot?.billingMode === 'per-request') currentMode = 'per-request'
  if (
    snapshot?.billingMode === 'tiered_expr' ||
    differences.billing_mode?.current === 'tiered_expr'
  ) {
    currentMode = 'tiered_expr'
  }
  const currentPrice = priceForField(current, field, currentMode, quotaPerUnit)
  const upstreamValue =
    sourceName === null
      ? resolutions[field]
      : differences[field]?.upstreams[sourceName]
  if (upstreamValue === null || upstreamValue === undefined) {
    return { current: currentPrice, proposed: null }
  }

  const changes: PricingValues = { ...resolutions }
  changes[field] = upstreamValue === 'same' ? current[field] : upstreamValue
  const proposed = { ...current, ...changes }
  const categoryChanges = upstreamValue === 'same' ? resolutions : changes
  let proposedMode = currentMode
  if ('model_price' in categoryChanges) proposedMode = 'per-request'
  else if (RATIO_SYNC_FIELDS.some((key) => key in categoryChanges)) {
    proposedMode = 'per-token'
  } else if (
    'billing_expr' in categoryChanges ||
    categoryChanges.billing_mode === 'tiered_expr'
  ) {
    proposedMode = 'tiered_expr'
  }

  return {
    current: currentPrice,
    proposed: priceForField(proposed, field, proposedMode, quotaPerUnit),
  }
}

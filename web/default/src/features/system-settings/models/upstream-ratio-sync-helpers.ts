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
import type { PricingSyncPatch, RatioType } from '../types'
import { RATIO_TYPE_OPTIONS } from './constants'

export type RatioDifferenceEntry = {
  current: number | string | null
  upstreams: Record<string, number | string | 'same'>
  confidence: Record<string, boolean>
}

export type ModelRow = {
  key: string
  model: string
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>
  billingConflict: boolean
}

export type ResolutionsMap = Record<string, Record<string, number | string>>

export const RATIO_SYNC_FIELDS: RatioType[] = [
  'model_ratio',
  'completion_ratio',
  'cache_ratio',
  'create_cache_ratio',
  'image_ratio',
  'audio_ratio',
  'audio_completion_ratio',
]

export const SYNC_FIELD_ORDER: RatioType[] = [
  ...RATIO_SYNC_FIELDS,
  'model_price',
  'billing_mode',
  'billing_expr',
]

export const NUMERIC_SYNC_FIELDS = new Set<string>([
  ...RATIO_SYNC_FIELDS,
  'model_price',
])

const RATIO_OPTION_KEYS = [
  'ModelRatio',
  'CompletionRatio',
  'CacheRatio',
  'CreateCacheRatio',
  'ImageRatio',
  'AudioRatio',
  'AudioCompletionRatio',
]

const TIERED_OPTION_KEYS = [
  'billing_setting.billing_mode',
  'billing_setting.billing_expr',
]

const PRICING_FORM_OPTION_KEYS: Record<string, string> = {
  ModelPrice: 'ModelPrice',
  ModelRatio: 'ModelRatio',
  CacheRatio: 'CacheRatio',
  CreateCacheRatio: 'CreateCacheRatio',
  CompletionRatio: 'CompletionRatio',
  ImageRatio: 'ImageRatio',
  AudioRatio: 'AudioRatio',
  AudioCompletionRatio: 'AudioCompletionRatio',
  BillingMode: 'billing_setting.billing_mode',
  BillingExpr: 'billing_setting.billing_expr',
}

export function buildPricingMapDiffPatches(
  before: Record<string, string>,
  after: Record<string, string>
): Record<string, PricingSyncPatch> {
  const patches: Record<string, PricingSyncPatch> = {}

  for (const [formKey, optionKey] of Object.entries(PRICING_FORM_OPTION_KEYS)) {
    const previous = JSON.parse(before[formKey] || '{}') as Record<
      string,
      number | string
    >
    const next = JSON.parse(after[formKey] || '{}') as Record<
      string,
      number | string
    >
    const set: Record<string, number | string> = {}
    const deleted: string[] = []

    for (const model of Object.keys(previous)) {
      if (!(model in next)) deleted.push(model)
    }
    for (const [model, value] of Object.entries(next)) {
      if (!(model in previous) || previous[model] !== value) set[model] = value
    }
    if (Object.keys(set).length > 0 || deleted.length > 0) {
      patches[optionKey] = {
        ...(Object.keys(set).length > 0 ? { set } : {}),
        ...(deleted.length > 0 ? { delete: deleted } : {}),
      }
    }
  }

  return patches
}

function optionKeyBySyncField(ratioType: string): string {
  const explicit: Record<string, string> = {
    billing_mode: 'billing_setting.billing_mode',
    billing_expr: 'billing_setting.billing_expr',
  }
  if (explicit[ratioType]) return explicit[ratioType]
  return ratioType
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join('')
}

export function buildPricingSyncPatches(
  resolutions: ResolutionsMap
): Record<string, PricingSyncPatch> {
  const patches: Record<string, PricingSyncPatch> = {}
  const deleteModel = (optionKey: string, model: string) => {
    const patch = patches[optionKey] ?? {}
    patch.delete = [...(patch.delete ?? []), model]
    patches[optionKey] = patch
  }
  const setModel = (
    optionKey: string,
    model: string,
    value: number | string
  ) => {
    const patch = patches[optionKey] ?? {}
    patch.set = { ...patch.set, [model]: value }
    patches[optionKey] = patch
  }

  Object.entries(resolutions).forEach(([model, ratios]) => {
    const selectedTypes = Object.keys(ratios)
    const hasPrice = selectedTypes.includes('model_price')
    const hasRatio = selectedTypes.some((field) =>
      RATIO_SYNC_FIELDS.includes(field as RatioType)
    )

    if (hasPrice) {
      for (const key of [...RATIO_OPTION_KEYS, ...TIERED_OPTION_KEYS]) {
        deleteModel(key, model)
      }
    } else if (hasRatio) {
      deleteModel('ModelPrice', model)
      for (const key of TIERED_OPTION_KEYS) deleteModel(key, model)
    } else {
      deleteModel('ModelPrice', model)
      for (const key of RATIO_OPTION_KEYS) deleteModel(key, model)
    }

    Object.entries(ratios).forEach(([ratioType, value]) => {
      setModel(
        optionKeyBySyncField(ratioType),
        model,
        NUMERIC_SYNC_FIELDS.has(ratioType) ? Number(value) : value
      )
    })
  })

  return patches
}

export function getSyncFieldLabel(
  ratioType: string,
  t: (key: string) => string
): string {
  const opt = RATIO_TYPE_OPTIONS.find((o) => o.value === ratioType)
  if (opt) return t(opt.label)
  return ratioType
}

export function getOrderedRatioTypes(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  filter?: string
): RatioType[] {
  const keys = Object.keys(ratioTypes) as RatioType[]
  const ordered = [
    ...SYNC_FIELD_ORDER.filter((f) => keys.includes(f)),
    ...keys.filter((f) => !SYNC_FIELD_ORDER.includes(f)),
  ]
  if (!filter || filter === '__all__') return ordered
  return ordered.filter((f) => f === filter)
}

// billing_mode is derived from billing_expr and is not a separate price for
// an administrator to compare or select.
export function getDisplaySyncFields(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  filter?: string
): RatioType[] {
  const fields = getOrderedRatioTypes(ratioTypes, filter)
  if (!fields.includes('billing_expr')) return fields
  return fields.filter((field) => field !== 'billing_mode')
}

export function getPreferredSyncField(
  ratioTypes: Partial<Record<RatioType, RatioDifferenceEntry>>,
  ratioType: RatioType,
  sourceName: string
): RatioType {
  const exprValue = ratioTypes.billing_expr?.upstreams?.[sourceName]
  if (
    ratioType !== 'billing_expr' &&
    exprValue !== null &&
    exprValue !== undefined &&
    exprValue !== 'same'
  ) {
    return 'billing_expr'
  }
  return ratioType
}

export function isSelectableUpstreamValue(
  value: number | string | 'same' | null | undefined
): boolean {
  return value !== null && value !== undefined && value !== 'same'
}

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
import type { TFunction } from 'i18next'

import type { TokenUnit } from './types'

// ----------------------------------------------------------------------------
// Pricing Constants
// ----------------------------------------------------------------------------

/** Sort options for pricing models */
export const SORT_OPTIONS = {
  NAME: 'name',
  PRICE_LOW: 'price-low',
  PRICE_HIGH: 'price-high',
  BENEFIT: 'benefit',
} as const

export type SortOption = (typeof SORT_OPTIONS)[keyof typeof SORT_OPTIONS]
export const DEFAULT_SORT_OPTION = SORT_OPTIONS.BENEFIT

export function getSortLabels(t: TFunction): Record<SortOption, string> {
  return {
    [SORT_OPTIONS.BENEFIT]: t('Benefit'),
    [SORT_OPTIONS.NAME]: t('Name'),
    [SORT_OPTIONS.PRICE_LOW]: t('Price: Low to High'),
    [SORT_OPTIONS.PRICE_HIGH]: t('Price: High to Low'),
  }
}

/** Filter values */
export const FILTER_ALL = 'all'

/** Quota type options */
export const QUOTA_TYPES = {
  ALL: 'all',
  TOKEN: 'token',
  REQUEST: 'request',
} as const

export type QuotaTypeOption = (typeof QUOTA_TYPES)[keyof typeof QUOTA_TYPES]

/** Quota type labels */
export function getQuotaTypeLabels(
  t: TFunction
): Record<QuotaTypeOption, string> {
  return {
    [QUOTA_TYPES.ALL]: t('All Models'),
    [QUOTA_TYPES.TOKEN]: t('Token-based'),
    [QUOTA_TYPES.REQUEST]: t('Per Request'),
  }
}

/** Model task categories used by the catalog filter. */
export const MODEL_TYPES = {
  ALL: 'all',
  TEXT: 'text',
  IMAGE: 'image',
  AUDIO: 'audio',
  VIDEO: 'video',
  EMBEDDING: 'embedding',
  RERANK: 'rerank',
  OTHER: 'other',
} as const

export type ModelType = Exclude<
  (typeof MODEL_TYPES)[keyof typeof MODEL_TYPES],
  'all'
>

export function getModelTypeLabels(
  t: TFunction
): Record<ModelType | typeof FILTER_ALL, string> {
  return {
    [MODEL_TYPES.ALL]: t('All Types'),
    [MODEL_TYPES.TEXT]: t('Text'),
    [MODEL_TYPES.IMAGE]: t('Images'),
    [MODEL_TYPES.AUDIO]: t('Audio'),
    [MODEL_TYPES.VIDEO]: t('Video'),
    [MODEL_TYPES.EMBEDDING]: t('Embedding models'),
    [MODEL_TYPES.RERANK]: t('Reranking'),
    [MODEL_TYPES.OTHER]: t('Other'),
  }
}

/** Filter section keys */
export const FILTER_SECTIONS = {
  PRICING_TYPE: 'pricingType',
  MODEL_TYPE: 'modelType',
  VENDOR: 'vendor',
  GROUP: 'group',
  TAG: 'tag',
} as const

/** Maximum number of tags to display in model row */
export const MAX_TAGS_DISPLAY = 5

/** Maximum number of filter items to display before showing "More..." */
export const MAX_FILTER_ITEMS = 5

/** Sidebar width */
export const SIDEBAR_WIDTH = 'w-64'

/** Excluded groups */
export const EXCLUDED_GROUPS = ['', 'auto']

/** Quota type values */
export const QUOTA_TYPE_VALUES = {
  TOKEN: 0,
  REQUEST: 1,
} as const

/** Token unit divisors */
export const TOKEN_UNIT_DIVISORS = {
  M: 1,
  K: 1000,
} as const

/** Default token unit for pricing display */
export const DEFAULT_TOKEN_UNIT: TokenUnit = 'M'

/** View mode options */
export const VIEW_MODES = {
  CARD: 'card',
  TABLE: 'table',
} as const

export type ViewMode = (typeof VIEW_MODES)[keyof typeof VIEW_MODES]

/** Default page size for pricing table */
export const DEFAULT_PRICING_PAGE_SIZE = 20

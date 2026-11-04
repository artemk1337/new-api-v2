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
import {
  SORT_OPTIONS,
  FILTER_ALL,
  QUOTA_TYPES,
  QUOTA_TYPE_VALUES,
  MODEL_TYPES,
} from '../constants'
import type { PricingModel } from '../types'
import {
  getDisplayGroupPrice,
  getGroupPriceAmount,
  isTokenGroupPrice,
} from './group-price'
import { getModelType } from './model-type'

export type PricingFilterValues = {
  search: string
  vendor: string
  group: string
  quotaType: string
  modelType: string
  tag: string
}

export type PricingFacet =
  | 'vendor'
  | 'group'
  | 'quotaType'
  | 'modelType'
  | 'tag'

// ----------------------------------------------------------------------------
// Filter Utilities
// ----------------------------------------------------------------------------

/**
 * Filter models by search query
 */
export function filterBySearch(
  models: PricingModel[],
  query: string
): PricingModel[] {
  if (!query) return models

  const lowerQuery = query.toLowerCase()
  return models.filter(
    (m) =>
      m.model_name?.toLowerCase().includes(lowerQuery) ||
      m.description?.toLowerCase().includes(lowerQuery) ||
      m.tags?.toLowerCase().includes(lowerQuery) ||
      m.vendor_name?.toLowerCase().includes(lowerQuery)
  )
}

/**
 * Filter models by vendor
 */
export function filterByVendor(
  models: PricingModel[],
  vendor: string
): PricingModel[] {
  if (vendor === FILTER_ALL) return models
  return models.filter((m) => m.vendor_name === vendor)
}

/**
 * Filter models by group
 */
export function filterByGroup(
  models: PricingModel[],
  group: string
): PricingModel[] {
  if (group === FILTER_ALL) return models
  return models.filter((m) => m.enable_groups?.includes(group))
}

/**
 * Filter models by quota type
 */
export function filterByQuotaType(
  models: PricingModel[],
  quotaType: string,
  group = FILTER_ALL
): PricingModel[] {
  if (quotaType === QUOTA_TYPES.ALL) return models
  const targetType =
    quotaType === QUOTA_TYPES.TOKEN
      ? QUOTA_TYPE_VALUES.TOKEN
      : QUOTA_TYPE_VALUES.REQUEST
  return models.filter((model) =>
    (model.group_prices ?? []).some(
      (price) =>
        (group === FILTER_ALL || price.group === group) &&
        Number(!isTokenGroupPrice(price)) === targetType
    )
  )
}

/**
 * Filter models by task category
 */
export function filterByModelType(
  models: PricingModel[],
  modelType: string
): PricingModel[] {
  if (modelType === MODEL_TYPES.ALL) return models
  return models.filter((model) => getModelType(model) === modelType)
}

/**
 * Get model price for sorting
 */
function compareModelPrices(
  a: PricingModel,
  b: PricingModel,
  group: string,
  descending: boolean
): number {
  const aPrice = getDisplayGroupPrice(a, group)
  const bPrice = getDisplayGroupPrice(b, group)
  const aAmount = aPrice ? getGroupPriceAmount(aPrice) : null
  const bAmount = bPrice ? getGroupPriceAmount(bPrice) : null
  if (aAmount === null && bAmount !== null) return 1
  if (aAmount !== null && bAmount === null) return -1
  if (aPrice && bPrice && aPrice.price_unit !== bPrice.price_unit) {
    return aPrice.price_unit.localeCompare(bPrice.price_unit)
  }
  if (aAmount !== null && bAmount !== null && aAmount !== bAmount) {
    return descending ? bAmount - aAmount : aAmount - bAmount
  }
  return a.model_name.localeCompare(b.model_name)
}

/**
 * Sort models by specified option
 */
export function sortModels(
  models: PricingModel[],
  sortBy: string,
  groupFilter = FILTER_ALL
): PricingModel[] {
  const sorted = [...models]

  switch (sortBy) {
    case SORT_OPTIONS.NAME:
      sorted.sort((a, b) =>
        (a.model_name || '').localeCompare(b.model_name || '')
      )
      break
    case SORT_OPTIONS.PRICE_LOW:
    case SORT_OPTIONS.BENEFIT:
      sorted.sort((a, b) => compareModelPrices(a, b, groupFilter, false))
      break
    case SORT_OPTIONS.PRICE_HIGH:
      sorted.sort((a, b) => compareModelPrices(a, b, groupFilter, true))
      break
  }

  return sorted
}

/**
 * Apply all filters and sorting to models
 */
export function filterAndSortModels(
  models: PricingModel[],
  filters: PricingFilterValues & { sortBy: string }
): PricingModel[] {
  let result = filterModelsForFacet(models, filters)
  result = sortModels(result, filters.sortBy, filters.group)

  return result
}

export function filterModelsForFacet(
  models: PricingModel[],
  filters: PricingFilterValues,
  skipFacet?: PricingFacet
): PricingModel[] {
  let result = filterBySearch(models, filters.search)
  if (skipFacet !== 'vendor') {
    result = filterByVendor(result, filters.vendor)
  }
  if (skipFacet !== 'group') {
    result = filterByGroup(result, filters.group)
  }
  if (skipFacet !== 'quotaType') {
    result = filterByQuotaType(result, filters.quotaType, filters.group)
  }
  if (skipFacet !== 'modelType') {
    result = filterByModelType(result, filters.modelType)
  }
  if (skipFacet !== 'tag') {
    result = filterByTag(result, filters.tag)
  }
  return result
}

/**
 * Parse tags from comma-separated string
 */
export function parseTags(tagsString?: string): string[] {
  if (!tagsString) return []
  return tagsString
    .split(/[,;|\s]+/)
    .map((t) => t.trim())
    .filter(Boolean)
}

/**
 * Extract all unique tags from models
 */
export function extractAllTags(models: PricingModel[]): string[] {
  const tagSet = new Set<string>()

  models.forEach((model) => {
    if (model.tags) {
      const tags = parseTags(model.tags)
      tags.forEach((tag) => {
        tagSet.add(tag.toLowerCase())
      })
    }
  })

  return [...tagSet].sort((a, b) => a.localeCompare(b))
}

/**
 * Filter models by tag
 */
export function filterByTag(
  models: PricingModel[],
  tag: string
): PricingModel[] {
  if (tag === FILTER_ALL) return models

  const tagLower = tag.toLowerCase()
  return models.filter((m) => {
    if (!m.tags) return false
    const modelTags = parseTags(m.tags).map((t) => t.toLowerCase())
    return modelTags.includes(tagLower)
  })
}

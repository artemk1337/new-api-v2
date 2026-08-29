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
import { z } from 'zod'
import type { TFunction } from 'i18next'

import { parseQuotaFromDollars, quotaUnitsToDollars } from '@/lib/format'

import type { ApiKeyFormData, ApiKey } from '../types'

// ============================================================================
// Form Schema
// ============================================================================

export function getApiKeyFormSchema(t: TFunction) {
  return z
    .object({
      name: z.string().min(1, t('Please enter a name')),
      remain_quota_dollars: z.number().optional(),
      expired_time: z.date().optional(),
      unlimited_quota: z.boolean(),
      model_limits: z.array(z.string()),
      model_mapping: z.string(),
      allow_ips: z.string().optional(),
      group: z.string().min(1, t('Please select a group')),
      auto_group_mode: z.enum(['all', 'specific']),
      auto_group_candidates: z.array(z.string()),
      tokenCount: z.number().min(1).optional(),
    })
    .superRefine((data, ctx) => {
      if (
        !data.unlimited_quota &&
        (data.remain_quota_dollars === undefined ||
          data.remain_quota_dollars < 0)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['remain_quota_dollars'],
          message: t('Quota must be zero or greater'),
        })
      }

      if (
        data.group === 'auto' &&
        data.auto_group_mode === 'specific' &&
        data.auto_group_candidates.length === 0
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['auto_group_candidates'],
          message: t('Select at least one group for Auto'),
        })
      }
    })
}

export type ApiKeyFormValues = z.infer<ReturnType<typeof getApiKeyFormSchema>>

export function resolveAutoGroupCandidates(
  availableGroups: string[],
  autoGroups: string[]
): string[] {
  const available = new Set(
    availableGroups.filter((group) => group && group !== 'auto')
  )
  const resolved: string[] = []
  const seen = new Set<string>()

  for (const group of autoGroups) {
    if (!available.has(group) || seen.has(group)) continue
    seen.add(group)
    resolved.push(group)
  }

  return resolved
}

// ============================================================================
// Form Defaults
// ============================================================================

export const API_KEY_FORM_DEFAULT_VALUES: ApiKeyFormValues = {
  name: '',
  remain_quota_dollars: 10,
  expired_time: undefined,
  unlimited_quota: true,
  model_limits: [],
  model_mapping: '',
  allow_ips: '',
  group: 'auto',
  auto_group_mode: 'all',
  auto_group_candidates: [],
  tokenCount: 1,
}

export function getApiKeyFormDefaultValues(): ApiKeyFormValues {
  return {
    ...API_KEY_FORM_DEFAULT_VALUES,
    model_limits: [],
    model_mapping: '',
    auto_group_candidates: [],
  }
}

// ============================================================================
// Form Data Transformation
// ============================================================================

/**
 * Transform form data to API payload
 */
export function transformFormDataToPayload(
  data: ApiKeyFormValues
): ApiKeyFormData {
  return {
    name: data.name,
    remain_quota: data.unlimited_quota
      ? 0
      : parseQuotaFromDollars(data.remain_quota_dollars || 0),
    expired_time: data.expired_time
      ? Math.floor(data.expired_time.getTime() / 1000)
      : -1,
    unlimited_quota: data.unlimited_quota,
    model_limits_enabled: data.model_limits.length > 0,
    model_limits: data.model_limits.join(','),
    model_mapping: data.model_mapping,
    allow_ips: data.allow_ips || '',
    group: data.group || 'auto',
    auto_group_candidates:
      data.group === 'auto' && data.auto_group_mode === 'specific'
        ? data.auto_group_candidates.filter((group) => group !== 'auto')
        : [],
  }
}

/**
 * Transform API key data to form defaults
 */
export function transformApiKeyToFormDefaults(
  apiKey: ApiKey
): ApiKeyFormValues {
  const autoGroupCandidates = apiKey.auto_group_candidates || []
  return {
    name: apiKey.name,
    remain_quota_dollars: apiKey.unlimited_quota
      ? 0
      : quotaUnitsToDollars(apiKey.remain_quota),
    expired_time:
      apiKey.expired_time > 0
        ? new Date(apiKey.expired_time * 1000)
        : undefined,
    unlimited_quota: apiKey.unlimited_quota,
    model_limits: apiKey.model_limits
      ? apiKey.model_limits.split(',').filter(Boolean)
      : [],
    model_mapping: apiKey.model_mapping || '',
    allow_ips: apiKey.allow_ips || '',
    group: apiKey.group || 'auto',
    auto_group_mode: autoGroupCandidates.length > 0 ? 'specific' : 'all',
    auto_group_candidates: autoGroupCandidates.filter(
      (group) => group !== 'auto'
    ),
    tokenCount: 1,
  }
}

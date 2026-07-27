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
import { api } from '@/lib/api'

import type {
  ConfirmPaymentComplianceResponse,
  FetchUpstreamRatiosRequest,
  LogCleanupTask,
  SystemUpdateCheckResponse,
  SystemUpdateJobStatusResponse,
  SystemUpdateTask,
  SystemOptionsResponse,
  SystemTaskListResponse,
  SystemTaskResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
  PricingSyncConfig,
  PricingSyncConfigResponse,
  PricingSyncModelState,
  PricingSyncModelPreference,
  PricingSyncModelStateResponse,
  PricingSyncPatch,
} from './types'

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request, {
    skipBusinessError: true,
  })
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to update setting')
  }
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function startLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<SystemTaskResponse<LogCleanupTask>>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function getCurrentLogCleanupTask() {
  const res = await api.get<SystemTaskResponse<LogCleanupTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'log_cleanup' },
    }
  )
  return res.data
}

export async function getSystemTask<TTask = LogCleanupTask>(taskId: string) {
  const res = await api.get<SystemTaskResponse<TTask>>(
    `/api/system-task/${taskId}`
  )
  return res.data
}

export async function listSystemTasks(limit = 20) {
  const res = await api.get<SystemTaskListResponse>('/api/system-task/list', {
    params: { limit },
  })
  return res.data
}

export async function checkSystemUpdate() {
  const res = await api.get<SystemUpdateCheckResponse>(
    '/api/system-update/check'
  )
  return res.data
}

export async function startSystemUpdate(version: string) {
  const res = await api.post<SystemTaskResponse<SystemUpdateTask>>(
    '/api/system-update/apply',
    { version }
  )
  return res.data
}

export async function getCurrentSystemUpdateTask() {
  const res = await api.get<SystemTaskResponse<SystemUpdateTask | null>>(
    '/api/system-task/current',
    {
      params: { type: 'system_update' },
    }
  )
  return res.data
}

export async function getSystemUpdateJob(jobId: string) {
  const res = await api.get<SystemUpdateJobStatusResponse>(
    `/api/system-update/job/${encodeURIComponent(jobId)}`
  )
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}

export async function getPricingSyncConfig() {
  const res = await api.get<PricingSyncConfigResponse>('/api/ratio_sync/config')
  return res.data
}

export async function updatePricingSyncConfig(config: PricingSyncConfig) {
  const { version, ...payload } = config
  const res = await api.put<UpdateOptionResponse>(
    '/api/ratio_sync/config',
    { ...payload, expected_version: version },
    { skipBusinessError: true }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to save settings')
  }
  return res.data
}

export async function getPricingSyncModelState(model: string) {
  const res = await api.get<PricingSyncModelStateResponse>(
    '/api/ratio_sync/model-preference',
    { params: { model } }
  )
  return res.data
}

export async function updatePricingSyncModelState(
  state: Pick<PricingSyncModelState, 'model_name' | 'mode' | 'channel_id'>
) {
  const res = await api.put<UpdateOptionResponse>(
    '/api/ratio_sync/model-preference',
    state,
    { skipBusinessError: true }
  )
  if (!res.data.success) {
    throw new Error(
      res.data.message || 'Failed to save price synchronization source'
    )
  }
  return res.data
}

export async function applyPricingSyncPatches(
  patches: Record<string, PricingSyncPatch>,
  preferences: PricingSyncModelPreference[] = []
) {
  const res = await api.post<UpdateOptionResponse>(
    '/api/ratio_sync/apply',
    { patches, preferences },
    { skipBusinessError: true }
  )
  if (!res.data.success) {
    throw new Error(res.data.message || 'Failed to apply pricing changes')
  }
  return res.data
}

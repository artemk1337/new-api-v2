import { api } from '@/lib/api'

export type PlatformCurrency = {
  code: string
  name: string
  symbol: string
  enabled: boolean
  sync_enabled: boolean
  sync_provider: string
  manual_rate_to_usd: number
  rate_to_usd: number
  last_sync_at?: string
  last_sync_error?: string
}

type CurrencyResponse = {
  success: boolean
  data?: PlatformCurrency[]
  message?: string
}
type SingleCurrencyResponse = {
  success: boolean
  data?: PlatformCurrency
  message?: string
}

export type CurrencySyncConfig = {
  update_interval: string
  allowed_intervals: string[]
}

type CurrencySyncConfigResponse = {
  success: boolean
  data?: CurrencySyncConfig
  message?: string
}

export async function getPlatformCurrencies(admin = false) {
  const response = await api.get<CurrencyResponse>(
    admin ? '/api/currencies/admin' : '/api/currencies'
  )
  return response.data
}

export type PlatformCurrencyPayload = Omit<
  PlatformCurrency,
  'rate_to_usd' | 'last_sync_at' | 'last_sync_error'
>

export async function createPlatformCurrency(payload: PlatformCurrencyPayload) {
  const response = await api.post<SingleCurrencyResponse>(
    '/api/currencies/admin',
    payload
  )
  return response.data
}

export async function updatePlatformCurrency(
  code: string,
  payload: Partial<PlatformCurrencyPayload>
) {
  const response = await api.put<SingleCurrencyResponse>(
    `/api/currencies/admin/${encodeURIComponent(code)}`,
    payload
  )
  return response.data
}

export async function deletePlatformCurrency(code: string) {
  const response = await api.delete(
    `/api/currencies/admin/${encodeURIComponent(code)}`
  )
  return response.data as { success: boolean; message?: string }
}

export async function syncPlatformCurrency(code: string) {
  const response = await api.post<SingleCurrencyResponse>(
    `/api/currencies/admin/${encodeURIComponent(code)}/sync`
  )
  return response.data
}

export async function getCurrencySyncConfig() {
  const response = await api.get<CurrencySyncConfigResponse>(
    '/api/currencies/admin/config'
  )
  return response.data
}

export async function updateCurrencySyncConfig(updateInterval: string) {
  const response = await api.put<CurrencySyncConfigResponse>(
    '/api/currencies/admin/config',
    { update_interval: updateInterval }
  )
  return response.data
}

export async function syncAllPlatformCurrencies() {
  const response = await api.post<CurrencyResponse>(
    '/api/currencies/admin/sync'
  )
  return response.data
}

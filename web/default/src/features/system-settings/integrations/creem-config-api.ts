import { api } from '@/lib/api'

export type CreemConfigUpdate = {
  api_key?: string
  webhook_secret?: string
  test_mode?: boolean
  products?: string
}

export type PaymentOptionUpdate = {
  key: string
  value: string | number | boolean
}

export function getPaymentSettingsSaveErrorMessage(
  error: unknown,
  fallback = 'Failed to save payment settings'
) {
  return error instanceof Error && error.message ? error.message : fallback
}

export function buildPaymentSettingsPayload(
  options: PaymentOptionUpdate[],
  creem?: CreemConfigUpdate
) {
  return {
    options,
    ...(creem ? { creem } : {}),
  }
}

// Secret fields are returned masked by the API. An untouched masked value must
// be omitted so a partial save preserves the stored secret; clearing a dirty
// field is an explicit request and must be sent as an empty string.
export function shouldUpdateCreemSecret(
  currentValue: string,
  initialValue: string,
  isDirty: boolean,
  clearRequested = false
) {
  return clearRequested || (isDirty && currentValue !== initialValue)
}

export async function saveCreemConfig(update: CreemConfigUpdate) {
  const response = await api.put<{ success: boolean; message?: string }>(
    '/api/option/creem/save',
    update,
    { skipBusinessError: true }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to save Creem settings')
  }
  return response.data
}

export async function savePaymentSettings(update: {
  options?: PaymentOptionUpdate[]
  creem?: CreemConfigUpdate
}) {
  const response = await api.post<{ success: boolean; message?: string }>(
    '/api/option/payment-settings/save',
    update,
    { skipBusinessError: true }
  )
  if (!response.data.success) {
    throw new Error(response.data.message || 'Failed to save payment settings')
  }
  return response.data
}

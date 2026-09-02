import { api } from '@/lib/api'

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

export function buildPaymentSettingsPayload(options: PaymentOptionUpdate[]) {
  return { options }
}

export async function savePaymentSettings(update: { options?: PaymentOptionUpdate[] }) {
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

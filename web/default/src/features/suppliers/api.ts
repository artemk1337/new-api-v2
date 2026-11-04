import { api } from '@/lib/api'

import type {
  ChannelSupplierBinding,
  GroupSupplierBinding,
  Supplier,
  SupplierChoice,
  SupplierModelTariff,
  SupplierPricingProfile,
  SupplierTariffs,
} from './types'

interface Response<T> {
  success: boolean
  message?: string
  data: T
}

function result<T>(response: Response<T>): T {
  if (!response.success) throw new Error(response.message || 'Request failed')
  return response.data
}

export const supplierQueryKeys = {
  all: ['suppliers'] as const,
  choices: ['suppliers', 'choices'] as const,
  bindings: ['suppliers', 'group-bindings'] as const,
  profiles: (id: number) => ['suppliers', id, 'profiles'] as const,
  tariffs: (id: number) => ['suppliers', 'profiles', id, 'tariffs'] as const,
}

export async function getSuppliers() {
  return result((await api.get<Response<Supplier[]>>('/api/suppliers')).data)
}

export async function getSupplierMigrationPreview() {
  return result(
    (
      await api.get<Response<Record<string, unknown>>>(
        '/api/suppliers/migration-preview'
      )
    ).data
  )
}

export async function getSupplierChoices() {
  return result(
    (await api.get<Response<SupplierChoice[]>>('/api/suppliers/choices')).data
  )
}

export async function saveSupplier(value: Omit<Supplier, 'id'>, id?: number) {
  const path = id ? `/api/suppliers/${id}` : '/api/suppliers'
  const response = id
    ? await api.put<Response<Supplier>>(path, value)
    : await api.post<Response<Supplier>>(path, value)
  return result(response.data)
}

export async function deleteSupplier(id: number) {
  return result((await api.delete<Response<void>>(`/api/suppliers/${id}`)).data)
}

export async function getSupplierProfiles(id: number) {
  return result(
    (
      await api.get<Response<SupplierPricingProfile[]>>(
        `/api/suppliers/${id}/profiles`
      )
    ).data
  )
}

export async function saveSupplierProfile(
  value: Omit<
    SupplierPricingProfile,
    'id' | 'last_attempt_at' | 'last_success_at' | 'last_error'
  >,
  id?: number
) {
  const response = id
    ? await api.put<Response<SupplierPricingProfile>>(
        `/api/suppliers/profiles/${id}`,
        value
      )
    : await api.post<Response<SupplierPricingProfile>>(
        `/api/suppliers/${value.supplier_id}/profiles`,
        value
      )
  return result(response.data)
}

export async function deleteSupplierProfile(id: number) {
  return result(
    (await api.delete<Response<void>>(`/api/suppliers/profiles/${id}`)).data
  )
}

export async function getGroupSupplierBindings() {
  return result(
    (
      await api.get<Response<GroupSupplierBinding[]>>(
        '/api/suppliers/group-bindings'
      )
    ).data
  )
}

export async function saveSupplierBindings(
  bindings: GroupSupplierBinding[],
  channels: ChannelSupplierBinding[] = []
) {
  return result(
    (
      await api.put<Response<void>>('/api/suppliers/group-bindings', {
        bindings,
        channels,
      })
    ).data
  )
}

export async function getSupplierTariffs(id: number) {
  return result(
    (
      await api.get<Response<SupplierTariffs>>(
        `/api/suppliers/profiles/${id}/tariffs`
      )
    ).data
  )
}

export async function publishSupplierTariffs(
  id: number,
  expected: SupplierTariffs,
  tariffs: SupplierModelTariff[]
) {
  return result(
    (
      await api.post<Response<{ revision: number }>>(
        `/api/suppliers/profiles/${id}/tariffs`,
        {
          expected_config_revision: expected.config_revision,
          expected_revision: expected.revision,
          tariffs,
        }
      )
    ).data
  )
}

export async function syncSupplierTariffs(
  id: number,
  expected: SupplierTariffs
) {
  return result(
    (
      await api.post<
        Response<{
          revision: number
          pending_confirmation: boolean
          models: number
        }>
      >(`/api/suppliers/profiles/${id}/sync`, {
        expected_config_revision: expected.config_revision,
        expected_revision: expected.revision,
      })
    ).data
  )
}

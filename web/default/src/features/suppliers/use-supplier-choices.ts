import { useQuery } from '@tanstack/react-query'

import {
  ADMIN_PERMISSION_ACTIONS,
  ADMIN_PERMISSION_RESOURCES,
  hasPermission,
} from '@/lib/admin-permissions'
import { useAuthStore } from '@/stores/auth-store'

import { getSupplierChoices, supplierQueryKeys } from './api'

export function useSupplierChoices() {
  const user = useAuthStore((state) => state.auth.user)
  const enabled = hasPermission(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL,
    ADMIN_PERMISSION_ACTIONS.SENSITIVE_WRITE
  )
  return useQuery({
    queryKey: supplierQueryKeys.choices,
    queryFn: getSupplierChoices,
    enabled,
    staleTime: 60_000,
  })
}

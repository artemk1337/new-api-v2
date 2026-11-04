import type { GroupSupplierBinding, SupplierBinding } from './types'

export function groupMatchesSupplier(
  groupId: number,
  bindings: GroupSupplierBinding[],
  selected: SupplierBinding
): boolean {
  const binding = bindings.find((item) => item.group_id === groupId)
  return (
    (binding?.supplier_id ?? null) === selected.supplier_id &&
    (binding?.profile_id ?? null) === selected.profile_id
  )
}

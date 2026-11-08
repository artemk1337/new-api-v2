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
export type GroupProviderCatalog = {
  success: boolean
  data: { vendor_id?: number; enable_groups: string[] }[]
  vendors: { id: number; name: string; icon?: string }[]
}

export function getGroupProviders(
  options: { value: string }[],
  catalog?: GroupProviderCatalog
) {
  if (!catalog?.success) return []
  const available = new Set(options.map((option) => option.value))
  const groupsByProvider = new Map<number, Set<string>>()
  for (const model of catalog.data) {
    if (model.vendor_id === undefined) continue
    for (const group of model.enable_groups ?? []) {
      if (!available.has(group) || group === 'auto') continue
      let groups = groupsByProvider.get(model.vendor_id)
      if (!groups) {
        groups = new Set<string>()
        groupsByProvider.set(model.vendor_id, groups)
      }
      groups.add(group)
    }
  }
  return catalog.vendors.flatMap((vendor) => {
    const groups = groupsByProvider.get(vendor.id)
    return groups?.size ? [{ ...vendor, groups: [...groups] }] : []
  })
}

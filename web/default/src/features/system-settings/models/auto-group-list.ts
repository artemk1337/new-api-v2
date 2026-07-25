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
export function normalizeAutoGroupList(value: unknown): string[] {
  if (!Array.isArray(value)) return []

  const groups: string[] = []
  const seen = new Set<string>()
  for (const item of value) {
    if (typeof item !== 'string') continue
    const group = item.trim()
    if (!group || seen.has(group)) continue
    seen.add(group)
    groups.push(group)
  }
  return groups
}

export function addAutoGroup(groups: string[], value: string): string[] {
  return normalizeAutoGroupList([...groups, value])
}

export function removeAutoGroup(groups: string[], index: number): string[] {
  const normalized = normalizeAutoGroupList(groups)
  if (normalized.length <= 1) return normalized
  return normalized.filter((_, itemIndex) => itemIndex !== index)
}

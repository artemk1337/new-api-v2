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
import type { ParsedTier } from './billing-expr'

export function getPricingTierLabel(
  tier: ParsedTier,
  tierCount: number,
  t: (key: string) => string
): string {
  if (tier.conditions.length === 0) {
    return tierCount === 1 ? t('All requests') : t('Other requests')
  }
  const labels = { p: 'Input', c: 'Output', len: 'Context' }
  const operators = { '<': '<', '<=': '≤', '>': '>', '>=': '≥' }
  return tier.conditions
    .map((condition) => {
      const value = condition.value
      let count = String(value)
      if (value >= 1_000_000 && value % 1_000_000 === 0) {
        count = `${value / 1_000_000}M`
      } else if (value >= 1000 && value % 1000 === 0) {
        count = `${value / 1000}K`
      }
      return `${t(labels[condition.var])} ${operators[condition.op]} ${count}`
    })
    .join(' · ')
}

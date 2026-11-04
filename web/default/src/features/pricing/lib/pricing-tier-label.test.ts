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
import { expect, test } from 'bun:test'

import { getPricingTierLabel } from './pricing-tier-label'

const t = (key: string) => key

test('shows all conditions with exact boundaries instead of technical tier names', () => {
  expect(
    getPricingTierLabel(
      {
        label: 'step_1',
        conditions: [
          { var: 'len', op: '<=', value: 272000 },
          { var: 'c', op: '<', value: 1001 },
        ],
      },
      2,
      t
    )
  ).toBe('Context ≤ 272K · Output < 1001')
})

test('distinguishes an unconditional tariff from the fallback in tiered pricing', () => {
  const tier = { label: 'step_2', conditions: [] }
  expect(getPricingTierLabel(tier, 1, t)).toBe('All requests')
  expect(getPricingTierLabel(tier, 2, t)).toBe('Other requests')
})

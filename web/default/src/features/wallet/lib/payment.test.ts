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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { TopupInfo } from '../types'
import {
  generatePresetAmounts,
  getDiscountForAmount,
  getMinTopupAmount,
} from './payment'

describe('wallet payment discounts', () => {
  test('treats amount discount map as minimum amount thresholds', () => {
    const discounts = {
      100: 0.95,
      150: 0.94,
      200: 0.93,
      300: 0.91,
      400: 0.9,
    }

    assert.equal(getDiscountForAmount(99, discounts), 1.0)
    assert.equal(getDiscountForAmount(100, discounts), 0.95)
    assert.equal(getDiscountForAmount(149, discounts), 0.95)
    assert.equal(getDiscountForAmount(150, discounts), 0.94)
    assert.equal(getDiscountForAmount(500, discounts), 0.9)
    assert.equal(getDiscountForAmount(1000, discounts), 0.9)
  })
})

describe('wallet minimum top-up', () => {
  test('keeps fractional online minimum amount', () => {
    const minimum = getMinTopupAmount({
      enable_online_topup: true,
      min_topup: 0.1,
    } as TopupInfo)

    assert.equal(minimum, 0.1)
    assert.deepEqual(
      generatePresetAmounts(minimum).slice(0, 2).map(({ value }) => value),
      [0.1, 0.5]
    )
  })
})

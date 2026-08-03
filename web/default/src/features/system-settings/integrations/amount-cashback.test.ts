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

import {
  formatAmountCashbackThreshold,
  isValidAmountCashbackConfig,
  normalizeAmountCashbackConfig,
} from './amount-cashback'

describe('amount cashback settings validation', () => {
  test('accepts numeric fractional thresholds and boundary percentages', () => {
    assert.equal(
      isValidAmountCashbackConfig([
        { min_amount: 0.1, cashback_percent: 0 },
        { min_amount: 20, cashback_percent: 100 },
      ]),
      true
    )
  })

  test('rejects numeric strings and duplicate thresholds', () => {
    assert.equal(
      isValidAmountCashbackConfig([{ min_amount: '10', cashback_percent: 1 }]),
      false
    )
    assert.equal(
      isValidAmountCashbackConfig([{ min_amount: 10, cashback_percent: '1' }]),
      false
    )
    assert.equal(
      isValidAmountCashbackConfig([
        { min_amount: 10, cashback_percent: 1 },
        { min_amount: 10, cashback_percent: 2 },
      ]),
      false
    )
  })

  test('normalizes a blank raw editor value to the empty array contract', () => {
    assert.equal(normalizeAmountCashbackConfig(''), '[]')
    assert.equal(normalizeAmountCashbackConfig('  \n '), '[]')
    assert.equal(
      normalizeAmountCashbackConfig(' [{"min_amount":1}] '),
      '[{"min_amount":1}]'
    )
  })

  test('formats threshold amounts in their configured request unit', () => {
    assert.equal(
      formatAmountCashbackThreshold(100000, true, 'en-US'),
      '100,000'
    )
    assert.equal(formatAmountCashbackThreshold(100.5, false, 'en-US'), '$100.5')
  })
})

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
  addAutoGroup,
  normalizeAutoGroupList,
  removeAutoGroup,
} from './auto-group-list.ts'

describe('Auto group allowlist', () => {
  test('trims values and removes empty and duplicate groups', () => {
    assert.deepEqual(
      normalizeAutoGroupList([' cheap ', '', 'premium', 'cheap', null]),
      ['cheap', 'premium']
    )
  })

  test('does not add a duplicate group', () => {
    assert.deepEqual(addAutoGroup(['cheap', 'premium'], ' cheap '), [
      'cheap',
      'premium',
    ])
  })

  test('does not remove the last group', () => {
    assert.deepEqual(removeAutoGroup(['cheap'], 0), ['cheap'])
    assert.deepEqual(removeAutoGroup(['cheap', 'premium'], 0), ['premium'])
  })
})

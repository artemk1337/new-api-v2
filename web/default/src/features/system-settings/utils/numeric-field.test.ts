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

import { normalizeDecimalInput, parseDecimalValue } from './numeric-field'

describe('decimal settings input', () => {
  test('accepts dot and comma decimal separators', () => {
    assert.equal(parseDecimalValue('0.1'), 0.1)
    assert.equal(parseDecimalValue('0,1'), 0.1)
  })

  test('keeps partial and invalid values out of form state', () => {
    assert.equal(parseDecimalValue(''), null)
    assert.equal(parseDecimalValue('0.'), null)
    assert.equal(parseDecimalValue('0,'), null)
    assert.equal(parseDecimalValue('0,1.2'), null)
  })

  test('allows a hundredth while preserving in-progress decimal text', () => {
    assert.equal(normalizeDecimalInput('0.01'), '0.01')
    assert.equal(normalizeDecimalInput('0,'), '0.')
    assert.equal(normalizeDecimalInput('0.001'), null)
  })
})

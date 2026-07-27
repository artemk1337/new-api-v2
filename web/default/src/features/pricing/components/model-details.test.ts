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

import { getAutoGroupChain } from '../lib/auto-group-chain'

describe('getAutoGroupChain', () => {
  test('orders model-available groups by effective price', () => {
    const chain = getAutoGroupChain(['1', '2', '3'], ['1', '2', '3'], {
      '1': 1,
      '2': 0.8,
      '3': 0.3,
    })

    assert.deepEqual(chain, ['3', '2', '1'])
  })

  test('keeps the order deterministic when prices match', () => {
    const chain = getAutoGroupChain(['2', '1', '3'], ['1', '2'], {
      '1': 1,
      '2': 1,
    })

    assert.deepEqual(chain, ['1', '2'])
  })
})

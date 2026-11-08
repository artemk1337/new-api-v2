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
import { describe, expect, it } from 'bun:test'

import { getGroupProviders } from './group-providers'

describe('getGroupProviders', () => {
  it('shows each available group once per provider and excludes inaccessible groups', () => {
    expect(
      getGroupProviders([{ value: '1' }, { value: '2' }, { value: 'auto' }], {
        success: true,
        vendors: [
          { id: 1, name: 'OpenAI' },
          { id: 2, name: 'Anthropic' },
          { id: 3, name: 'Other' },
        ],
        data: [
          { vendor_id: 1, enable_groups: ['1', '2', 'private', 'auto'] },
          { vendor_id: 1, enable_groups: ['1'] },
          { vendor_id: 2, enable_groups: ['2'] },
          { vendor_id: 3, enable_groups: ['private'] },
        ],
      })
    ).toEqual([
      { id: 1, name: 'OpenAI', groups: ['1', '2'] },
      { id: 2, name: 'Anthropic', groups: ['2'] },
    ])
  })

  it('keeps the unfiltered selector available when the catalog is unavailable', () => {
    expect(getGroupProviders([{ value: '1' }])).toEqual([])
    expect(
      getGroupProviders([], { success: false, data: [], vendors: [] })
    ).toEqual([])
  })
})

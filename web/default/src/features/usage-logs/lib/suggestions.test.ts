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
import { describe, expect, test } from 'bun:test'

import { buildSearchParams } from './filter'
import { logSuggestionScope, parseProxySupplierId } from './suggestions'
import { buildApiParams } from './utils'

describe('admin supplier log filter contract', () => {
  test('accepts unknown provenance zero and exact supplier IDs, rejecting malformed URL values', () => {
    expect(parseProxySupplierId(0)).toBe('0')
    expect(parseProxySupplierId('15')).toBe('15')
    for (const invalid of [
      '',
      '-1',
      '01',
      '1.5',
      'NaN',
      '1e2',
      true,
      null,
      Number.MAX_SAFE_INTEGER + 1,
    ]) {
      expect(parseProxySupplierId(invalid)).toBeUndefined()
    }
  })

  test('preserves manual text and includes supplier selection only for admin requests', () => {
    const search = buildSearchParams(
      { model: ' gpt-% ', token: '%', proxySupplierId: '0' },
      'common'
    )
    expect(search).toEqual({
      model: ' gpt-% ',
      token: '%',
      proxySupplierId: '0',
    })
    const config = { page: 1, pageSize: 20, searchParams: search }
    const admin = buildApiParams({ ...config, isAdmin: true })
    expect(admin.proxy_supplier_id).toBe(0)
    expect(admin.model_name).toBe(' gpt-% ')
    expect(
      buildApiParams({ ...config, isAdmin: false }).proxy_supplier_id
    ).toBeUndefined()
    expect(buildSearchParams({}, 'common')).not.toHaveProperty(
      'proxySupplierId'
    )
  })

  test('suggestions exclude their own field and pagination while retaining the other draft filters', () => {
    const scope = {
      p: 2,
      page_size: 10,
      model_name: 'gpt-%',
      token_name: 'key',
      proxy_supplier_id: 0,
    }
    expect(logSuggestionScope(scope, 'model_name')).toEqual({
      token_name: 'key',
      proxy_supplier_id: 0,
    })
    expect(logSuggestionScope(scope, 'proxy_supplier_id')).toEqual({
      model_name: 'gpt-%',
      token_name: 'key',
    })
    expect(scope.proxy_supplier_id).toBe(0)
  })
})

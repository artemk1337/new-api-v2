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

import { resolveDocumentationLink } from './documentation-link'

describe('documentation link', () => {
  test('opens the migrated Google guide inside the application', () => {
    assert.deepEqual(
      resolveDocumentationLink(
        'https://docs.google.com/document/d/1qhl3njTP5zASOOBg7XoGGwZiw5Bd0MAFYKjEOGWkCoE/edit?tab=t.0'
      ),
      { external: false, href: '/docs' }
    )
    assert.deepEqual(resolveDocumentationLink('/docs'), {
      external: false,
      href: '/docs',
    })
  })

  test('preserves other configured documentation sources', () => {
    assert.deepEqual(resolveDocumentationLink('https://docs.newapi.pro'), {
      external: true,
      href: 'https://docs.newapi.pro',
    })
  })
})

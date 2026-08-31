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
import { test } from 'node:test'

import type { TFunction } from 'i18next'

import { getSidebarData } from '../../../hooks/use-sidebar-data'
import { checkIsActive } from './url-utils'

const translate = ((key: string) => key) as TFunction

test('keeps the dashboard navigation active for related dashboard sections', () => {
  const dashboardItem = getSidebarData(translate)
    .navGroups.find((group) => group.id === 'general')
    ?.items.find((item) => item.url === '/dashboard/models')

  assert.ok(dashboardItem)
  assert.equal(checkIsActive('/dashboard/models', dashboardItem), true)
  assert.equal(checkIsActive('/dashboard/flow', dashboardItem), true)
  assert.equal(checkIsActive('/dashboard/users', dashboardItem), true)
})

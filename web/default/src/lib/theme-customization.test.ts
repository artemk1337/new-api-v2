/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

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
  PRESET_DEFAULT_FONT,
  resolveThemeFont,
  THEME_FONT_VALUES,
} from './theme-customization'

describe('theme font selection', () => {
  test('keeps explicit font options available', () => {
    for (const font of ['system', 'sans', 'inter', 'serif'] as const) {
      assert.equal(THEME_FONT_VALUES.has(font), true)
    }
  })

  test('uses the system UI font as the default preset font', () => {
    assert.equal(resolveThemeFont('default', 'default'), 'system')
    assert.equal(PRESET_DEFAULT_FONT.default, 'system')
  })

  test('keeps preset-specific typography for Auto', () => {
    assert.equal(resolveThemeFont('default', 'anthropic'), 'serif')
    assert.equal(resolveThemeFont('inter', 'anthropic'), 'inter')
  })
})

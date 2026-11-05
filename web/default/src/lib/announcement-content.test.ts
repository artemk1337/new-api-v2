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

import { splitAnnouncementContent } from './announcement-content'

describe('announcement title and preview', () => {
  test('keeps a Markdown heading outside the collapsible body', () => {
    expect(
      splitAnnouncementContent('## Model updates\n\nFirst paragraph\n- A\n- B')
    ).toEqual({ title: 'Model updates', body: 'First paragraph\n- A\n- B' })
  })
  test('supports the existing bold title format and Windows newlines', () => {
    expect(
      splitAnnouncementContent('  **Platform updates**\r\n\r\nDetails')
    ).toEqual({ title: 'Platform updates', body: 'Details' })
  })
  test('preserves content without an explicit title', () => {
    for (const body of [
      '',
      'Normal text\nMore text',
      '**Bold** text continues',
      '- First item\n- Second item',
    ]) {
      expect(splitAnnouncementContent(body)).toEqual({ title: '', body })
    }
  })
})

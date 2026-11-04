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

import { isTelegramPostUrl } from './announcement-telegram'

test('allows public Telegram post links', () => {
  assert.equal(isTelegramPostUrl('https://t.me/platform_news/123'), true)
})

test('does not render unsafe or unsupported announcement links', () => {
  for (const url of [
    undefined,
    '',
    'javascript:alert(1)',
    'http://t.me/platform_news/123',
    'https://t.me.example.com/platform_news/123',
    'https://example.com@t.me/platform_news/123',
    'https://t.me/c/123456/1',
    'https://t.me/platform_news',
    'https://t.me/platform_news/0',
    'https://t.me/platform_news/01',
    'https://t.me/platform_news/1?extra=value',
    'https://t.me/platform_news/1#fragment',
    'https://t.me/platform_news/1/',
    'https://t.me/platform_news/1\n',
  ]) {
    assert.equal(isTelegramPostUrl(url), false, url)
  }
})

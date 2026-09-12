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
import { readFileSync } from 'node:fs'
import { describe, test } from 'node:test'
import { fileURLToPath } from 'node:url'

import { DOCS_SECTIONS, DOCS_URLS } from './docs-sections'

const guide = readFileSync(
  fileURLToPath(new URL('./content/api-keys.md', import.meta.url)),
  'utf8'
)

describe('API-key guide', () => {
  test('has exactly one explicit anchor for every menu section', () => {
    const anchors = [...guide.matchAll(/<h2 id="([^"]+)"/g)].map(
      (match) => match[1]
    )

    assert.deepEqual(
      anchors,
      DOCS_SECTIONS.map((section) => section.id)
    )
  })

  test('keeps Claude and OpenAI-compatible URL rules explicit', () => {
    assert.equal(new URL(DOCS_URLS.gateway).pathname, '/')
    assert.equal(new URL(DOCS_URLS.openAI).pathname, '/v1')
    assert.equal(new URL(DOCS_URLS.models).pathname, '/v1/models')
    assert.match(guide, /ANTHROPIC_BASE_URL="https:\/\/vibecode-api\.online"/)
    assert.match(guide, /apiBase: https:\/\/vibecode-api\.online\/v1/)
    assert.match(guide, /https:\/\/vibecode-api\.online\/v1\/models/)
  })

  test('warns before exposing the dangerous permissions bypass', () => {
    const warningIndex = guide.indexOf('> **Опасный режим.')
    const commandIndex = guide.indexOf('claude --dangerously-skip-permissions')

    assert.notEqual(warningIndex, -1)
    assert.notEqual(commandIndex, -1)
    assert.ok(warningIndex < commandIndex)
  })

  test('documents the current native installer and npm requirements', () => {
    assert.match(guide, /curl -fsSL https:\/\/claude\.ai\/install\.sh \| bash/)
    assert.match(guide, /winget install Anthropic\.ClaudeCode/)
    assert.match(guide, /Node\.js 22/)
    assert.doesNotMatch(guide, /Node\.js 18/)
  })

  test('keeps diagnostics safe and distinguishes API auth formats', () => {
    assert.match(guide, /-H "Authorization: Bearer \$\{VIBECODE_API_KEY\}"/)
    assert.match(guide, /-H "x-api-key: \$\{VIBECODE_API_KEY\}"/)
    assert.match(guide, /anthropic-version: 2023-06-01/)
    assert.doesNotMatch(guide, /alias claudecc=/)
    assert.doesNotMatch(guide, /claude-(?:sonnet|opus)-\d+\.\d+/)
  })

  test('describes continue and resume without swapping their behavior', () => {
    assert.match(
      guide,
      /`claude --continue` \(`-c`\)\s*\|\s*Продолжить последнюю сессию в текущей папке/
    )
    assert.match(
      guide,
      /`claude --resume` \(`-r`\)\s*\|\s*Выбрать сессию из списка или продолжить её по ID\/имени/
    )
  })

  test('requires one successful CLI request before opening Claude Code integrations', () => {
    const setupIndex = guide.indexOf(
      '**Для Claude Code обязательно сначала настройте CLI:**'
    )
    const requestIndex = guide.indexOf(
      'отправьте в CLI минимум один запрос',
      setupIndex
    )
    const integrationIndex = guide.indexOf(
      'Только после первого успешного запроса в CLI открывайте приложение',
      requestIndex
    )

    assert.notEqual(setupIndex, -1)
    assert.ok(requestIndex > setupIndex)
    assert.ok(integrationIndex > requestIndex)
    assert.match(guide, /Убедитесь, что модель успешно ответила/)
  })
})

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
    assert.match(
      guide,
      /"ANTHROPIC_BASE_URL": "https:\/\/vibecode-api\.online"/
    )
    assert.match(guide, /apiBase: https:\/\/vibecode-api\.online\/v1/)
    assert.match(guide, /https:\/\/vibecode-api\.online\/v1\/models/)
  })

  test('documents Codex custom provider configuration', () => {
    assert.match(guide, /model_provider = "custom"/)
    assert.match(guide, /\[model_providers\.custom\]/)
    assert.match(guide, /experimental_bearer_token = "sk-xxxxxxxx"/)
    assert.match(guide, /wire_api = "responses"/)
    assert.doesNotMatch(guide, /export VIBECODE_API_KEY/)
    assert.doesNotMatch(guide, /env_key = "VIBECODE_API_KEY"/)
    assert.match(
      guide,
      /https:\/\/developers\.openai\.com\/codex\/config-file\/config-advanced#custom-model-providers/
    )
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

  test('provides persistent CLI settings and a connection check', () => {
    const connection = guide
      .split('<h2 id="claude-code"')[1]
      .split('<h2 id="openai-compatible"')[0]
    const example = connection.match(/```json\n([\s\S]*?)```/)
    assert.ok(example)
    assert.deepEqual(JSON.parse(example[1]), {
      env: {
        ANTHROPIC_AUTH_TOKEN: 'sk-xxxxxxxx',
        ANTHROPIC_BASE_URL: DOCS_URLS.gateway,
      },
    })
    assert.match(connection, /~\/\.claude\/settings\.json/)
    assert.ok(connection.includes('%USERPROFILE%'))
    assert.match(connection, /Отправьте любой запрос и дождитесь ответа модели/)
    assert.match(connection, /#claude-code-vscode/)
  })
})

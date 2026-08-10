/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
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
  buildApiKeySnippets,
  maskApiKey,
  normalizeApiKey,
  normalizeServerAddress,
  resolveServerAddress,
  resolveTokenKey,
} from './api-key-created-dialog-utils'

describe('api key created dialog helpers', () => {
  test('masks the middle of a key while preserving a recognizable prefix and suffix', () => {
    assert.equal(maskApiKey('sk-1234567890abcdef'), 'sk-12**********cdef')
    assert.equal(maskApiKey('short'), '*****')
  })

  test('normalizes masked and full keys for display', () => {
    assert.equal(normalizeApiKey('abc********xyz'), 'sk-abc********xyz')
    assert.equal(normalizeApiKey('sk-abc********xyz'), 'sk-abc********xyz')
  })

  test('normalizes server addresses before appending the API version', () => {
    assert.equal(
      normalizeServerAddress('https://api.example.com///'),
      'https://api.example.com'
    )
    assert.equal(
      normalizeServerAddress('https://api.example.com/v1/'),
      'https://api.example.com'
    )
  })

  test('resolves the current server address from reactive status data', () => {
    assert.equal(
      resolveServerAddress(
        { data: { server_address: 'https://status.example.com/v1/' } },
        'https://fallback.example.com'
      ),
      'https://status.example.com'
    )
    assert.equal(
      resolveServerAddress(null, 'https://fallback.example.com/'),
      'https://fallback.example.com'
    )
  })

  test('builds copyable snippets for each supported language with the real credentials', () => {
    const snippets = buildApiKeySnippets(
      'sk-test-secret',
      'https://api.example.com/v1'
    )

    for (const snippet of Object.values(snippets)) {
      assert.ok(snippet.includes('sk-test-secret'))
      assert.ok(snippet.includes('https://api.example.com/v1'))
    }
    assert.ok(snippets.curl.includes('/chat/completions'))
    assert.ok(snippets.python.includes('OpenAI'))
    assert.ok(snippets.node.includes('baseURL'))
    assert.ok(snippets.go.includes('http.NewRequest'))
  })

  test('allows a failed key resolution to be retried after creation succeeds', async () => {
    let failed = true
    const fetcher = async () => {
      if (failed) throw new Error('temporary failure')
      return { success: true, data: { key: 'retry-secret' } }
    }

    assert.equal(await resolveTokenKey(42, fetcher), null)
    failed = false
    assert.equal(await resolveTokenKey(42, fetcher), 'sk-retry-secret')
  })
})

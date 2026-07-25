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

import { apiKeySchema } from '../types'
import {
  getApiKeyFormDefaultValues,
  resolveAutoGroupCandidates,
  transformFormDataToPayload,
} from './api-key-form'

describe('API key Auto group form', () => {
  test('defaults new keys to Auto with all groups', () => {
    const values = getApiKeyFormDefaultValues()

    assert.equal(values.group, 'auto')
    assert.equal(values.auto_group_mode, 'all')
    assert.deepEqual(values.auto_group_candidates, [])
  })

  test('normalizes legacy empty group and CSV candidates from the API', () => {
    const apiKey = apiKeySchema.parse({
      id: 1,
      name: 'legacy',
      key: 'key',
      status: 1,
      remain_quota: 0,
      used_quota: 0,
      unlimited_quota: true,
      expired_time: -1,
      created_time: 1,
      accessed_time: 0,
      group: '',
      auto_group_candidates: 'cheap, premium',
      model_limits_enabled: false,
      model_limits: '',
      allow_ips: '',
    })

    assert.equal(apiKey.group, 'auto')
    assert.deepEqual(apiKey.auto_group_candidates, ['cheap', 'premium'])
  })

  test('clears Auto candidates for a fixed group', () => {
    const payload = transformFormDataToPayload({
      ...getApiKeyFormDefaultValues(),
      name: 'fixed',
      group: 'cheap',
      auto_group_mode: 'specific',
      auto_group_candidates: ['cheap', 'premium'],
    })

    assert.deepEqual(payload.auto_group_candidates, [])
  })

  test('keeps selected concrete candidates for Auto and excludes Auto itself', () => {
    const payload = transformFormDataToPayload({
      ...getApiKeyFormDefaultValues(),
      name: 'auto',
      auto_group_mode: 'specific',
      auto_group_candidates: ['cheap', 'auto', 'premium'],
    })

    assert.deepEqual(payload.auto_group_candidates, ['cheap', 'premium'])
  })

  test('keeps the server Auto allowlist order and removes unavailable groups', () => {
    const candidates = resolveAutoGroupCandidates(
      ['cheap', 'standard', 'premium'],
      ['premium', 'hidden', 'cheap']
    )

    assert.deepEqual(candidates, ['premium', 'cheap'])
  })

  test('deduplicates the server Auto allowlist', () => {
    const candidates = resolveAutoGroupCandidates(
      ['cheap', 'premium'],
      ['cheap', 'cheap', 'auto', 'premium']
    )

    assert.deepEqual(candidates, ['cheap', 'premium'])
  })
})

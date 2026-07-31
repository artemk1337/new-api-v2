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
*/
import assert from 'node:assert/strict'
import { test } from 'node:test'

import type { SecuritySettings } from '../types'
import { resolveSecuritySettings } from './security-settings'

const legacySettings = {
  ModelRequestRateLimitDuration: '1m',
  ModelRequestRateLimitDurationActivationAt: 0,
  ModelRequestRateLimitDurationActive: false,
  ModelRequestRateLimitDurationActivated: false,
  ModelRequestRateLimitDurationStaged: false,
  ModelRequestRateLimitDurationMinutes: 15,
} as SecuritySettings

test('uses the new rate limit duration when it is returned by the API', () => {
  assert.equal(
    resolveSecuritySettings(
      { ...legacySettings, ModelRequestRateLimitDuration: '10s' },
      [{ key: 'ModelRequestRateLimitDuration', value: '10s' }]
    ).ModelRequestRateLimitDuration,
    '10s'
  )
})

test('falls back to the legacy minutes option for older servers', () => {
  assert.equal(
    resolveSecuritySettings(legacySettings, [
      { key: 'ModelRequestRateLimitDurationMinutes', value: '15' },
    ]).ModelRequestRateLimitDuration,
    '15m'
  )
})

test('parses activation and falls back to inactive when the option is absent', () => {
  assert.equal(
    resolveSecuritySettings(legacySettings, [
      { key: 'ModelRequestRateLimitDurationActivated', value: 'true' },
    ]).ModelRequestRateLimitDurationActivated,
    true
  )
  assert.equal(
    resolveSecuritySettings(
      { ...legacySettings, ModelRequestRateLimitDurationActivated: true },
      [{ key: 'ModelRequestRateLimitDurationMinutes', value: '15' }]
    ).ModelRequestRateLimitDurationActivated,
    false
  )
})

test('only marks the duration as staged when backend metadata says so', () => {
  assert.equal(
    resolveSecuritySettings(legacySettings, [
      { key: 'ModelRequestRateLimitDurationStaged', value: 'true' },
    ]).ModelRequestRateLimitDurationStaged,
    true
  )
  assert.equal(
    resolveSecuritySettings(
      { ...legacySettings, ModelRequestRateLimitDurationStaged: true },
      [{ key: 'ModelRequestRateLimitDuration', value: '1m' }]
    ).ModelRequestRateLimitDurationStaged,
    false
  )
})

test('resolves scheduled and actual activation state independently', () => {
  const pending = resolveSecuritySettings(legacySettings, [
    { key: 'ModelRequestRateLimitDurationActivated', value: 'true' },
    { key: 'ModelRequestRateLimitDurationActivationAt', value: '1785434400' },
    { key: 'ModelRequestRateLimitDurationActive', value: 'false' },
  ])
  assert.equal(pending.ModelRequestRateLimitDurationActivated, true)
  assert.equal(pending.ModelRequestRateLimitDurationActivationAt, 1785434400)
  assert.equal(pending.ModelRequestRateLimitDurationActive, false)

  const active = resolveSecuritySettings(legacySettings, [
    { key: 'ModelRequestRateLimitDurationActivated', value: 'true' },
    { key: 'ModelRequestRateLimitDurationActive', value: 'true' },
  ])
  assert.equal(active.ModelRequestRateLimitDurationActivated, true)
  assert.equal(active.ModelRequestRateLimitDurationActive, true)
})

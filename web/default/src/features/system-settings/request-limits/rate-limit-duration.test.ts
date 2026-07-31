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

import {
  formatRateLimitDuration,
  getRateLimitActivationRefreshDelay,
  getRateLimitDuration,
  getRateLimitDurationUnit,
  getRateLimitPeriodState,
  parseRateLimitDuration,
  rateLimitDurationActivationUpdate,
  shouldSaveRateLimitDuration,
} from './rate-limit-duration'

describe('rate limit duration', () => {
  test('parses supported units and formats the selected unit unchanged', () => {
    assert.deepEqual(parseRateLimitDuration('10s'), { value: 10, unit: 's' })
    assert.deepEqual(parseRateLimitDuration('5m'), { value: 5, unit: 'm' })
    assert.deepEqual(parseRateLimitDuration('1h'), { value: 1, unit: 'h' })
    assert.equal(formatRateLimitDuration({ value: 10, unit: 's' }), '10s')
  })

  test('rejects invalid durations', () => {
    for (const duration of ['0s', '-1m', '1.5h', '1h30m', '10d']) {
      assert.equal(parseRateLimitDuration(duration), null)
    }
  })

  test('uses a valid duration before the legacy minutes fallback', () => {
    assert.deepEqual(getRateLimitDuration('10s', 15), { value: 10, unit: 's' })
    assert.deepEqual(getRateLimitDuration('', 15), { value: 15, unit: 'm' })
    assert.deepEqual(getRateLimitDuration('', 0), { value: 1, unit: 'm' })
  })

  test('keeps the selected unit while the input is temporarily invalid', () => {
    assert.equal(getRateLimitDurationUnit('0h', 'm'), 'h')
    assert.equal(getRateLimitDurationUnit('', 's'), 's')
  })

  test('schedules the options refresh at cutover or immediately when overdue', () => {
    assert.equal(
      getRateLimitActivationRefreshDelay(1_785_434_400, 1_785_434_390_000),
      10_000
    )
    assert.equal(
      getRateLimitActivationRefreshDelay(1_785_434_400, 1_785_434_401_000),
      0
    )
    assert.equal(getRateLimitActivationRefreshDelay(0, 0), null)
  })

  test('retries after the first refresh while activation is still pending', () => {
    assert.equal(
      getRateLimitActivationRefreshDelay(
        1_785_434_400,
        1_785_434_401_000,
        true
      ),
      5_000
    )
  })

  test('saves a synthetic fallback to create the canonical staged option', () => {
    assert.equal(shouldSaveRateLimitDuration('5m', '5m', false), true)
    assert.equal(shouldSaveRateLimitDuration('5m', '5m', true), false)
    assert.equal(shouldSaveRateLimitDuration('10s', '5m', true), true)
  })

  test('shows legacy period as effective until the staged period is activated', () => {
    assert.deepEqual(
      getRateLimitPeriodState('10s', 15, false, false, true, '10s'),
      {
        effectiveDuration: '15m',
        stagedDuration: '10s',
        isDraft: false,
        activationPending: false,
        canActivate: true,
      }
    )
    assert.deepEqual(
      getRateLimitPeriodState('10s', 15, true, true, true, '10s'),
      {
        effectiveDuration: '10s',
        stagedDuration: '10s',
        isDraft: false,
        activationPending: false,
        canActivate: false,
      }
    )
  })

  test('keeps the legacy period effective while activation is pending', () => {
    assert.deepEqual(
      getRateLimitPeriodState('10s', 15, true, false, true, '10s'),
      {
        effectiveDuration: '15m',
        stagedDuration: '10s',
        isDraft: false,
        activationPending: true,
        canActivate: false,
      }
    )
  })

  test('keeps the saved period effective and blocks activation for a draft', () => {
    assert.deepEqual(
      getRateLimitPeriodState('10s', 15, false, false, true, '1h'),
      {
        effectiveDuration: '15m',
        stagedDuration: '10s',
        isDraft: true,
        activationPending: false,
        canActivate: false,
      }
    )
  })

  test('blocks activation until the staged duration was persisted', () => {
    assert.deepEqual(
      getRateLimitPeriodState('1m', 1, false, false, false, '1m'),
      {
        effectiveDuration: '1m',
        stagedDuration: '1m',
        isDraft: false,
        activationPending: false,
        canActivate: false,
      }
    )
  })

  test('activation is a separate one-way option update', () => {
    assert.deepEqual(rateLimitDurationActivationUpdate, {
      key: 'ModelRequestRateLimitDurationActivated',
      value: true,
    })
  })
})

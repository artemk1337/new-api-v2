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
  createBotProtectionSchema,
  getBotProtectionOptionUpdates,
  type BotProtectionFormValues,
} from './bot-protection-options'

const defaultValues: BotProtectionFormValues = {
  RegistrationRateLimitEnabled: true,
  RegistrationRateLimitAttempts: 10,
  RegistrationRateLimitSuccesses: 3,
  RegistrationRateLimitWindowMinutes: 1440,
  TurnstileCheckEnabled: false,
  TurnstileSiteKey: '',
  TurnstileSecretKey: '',
}

describe('bot protection options', () => {
  test('saves Turnstile credentials before enabling protection', () => {
    const updates = getBotProtectionOptionUpdates(
      {
        ...defaultValues,
        TurnstileCheckEnabled: true,
        TurnstileSiteKey: 'site-key',
        TurnstileSecretKey: 'secret-key',
      },
      defaultValues
    )

    assert.deepEqual(updates, [
      { key: 'TurnstileSiteKey', value: 'site-key' },
      { key: 'TurnstileSecretKey', value: 'secret-key' },
      { key: 'TurnstileCheckEnabled', value: true },
    ])
  })

  test('does not overwrite a stored Turnstile secret with an empty value', () => {
    const updates = getBotProtectionOptionUpdates(defaultValues, {
      ...defaultValues,
      TurnstileSecretKey: 'stored-secret',
    })

    assert.deepEqual(updates, [])
  })

  test('rejects a successful registration limit above the attempt limit', () => {
    const schema = createBotProtectionSchema((key) => key)
    const result = schema.safeParse({
      ...defaultValues,
      RegistrationRateLimitAttempts: 2,
      RegistrationRateLimitSuccesses: 3,
    })

    assert.equal(result.success, false)
    if (result.success) return

    assert.ok(
      result.error.issues.some(
        (issue) =>
          issue.message ===
            'Successful registration limit cannot exceed attempt limit' &&
          issue.path.length === 1 &&
          issue.path[0] === 'RegistrationRateLimitSuccesses'
      )
    )
  })
})

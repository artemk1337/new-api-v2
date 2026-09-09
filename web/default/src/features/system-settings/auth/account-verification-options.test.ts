/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { parseAccountVerificationProviders } from './account-verification-options'

describe('account verification options', () => {
  test('normalizes and de-duplicates provider values', () => {
    assert.deepEqual(
      parseAccountVerificationProviders(' Telegram, email,telegram,unknown '),
      ['telegram', 'email']
    )
  })

  test('falls back to email when the stored value is empty or invalid', () => {
    assert.deepEqual(parseAccountVerificationProviders(''), ['email'])
    assert.deepEqual(parseAccountVerificationProviders('sms'), ['email'])
  })
})

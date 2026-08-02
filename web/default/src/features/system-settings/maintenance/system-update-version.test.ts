/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { compareStableSystemUpdateVersions } from './system-update-version'

describe('system update version comparison', () => {
  test('compares stable versions numerically', () => {
    assert.equal(compareStableSystemUpdateVersions('v1.1.101', 'v1.1.98'), 1)
    assert.equal(compareStableSystemUpdateVersions('v1.1.98', 'v1.1.98'), 0)
    assert.equal(compareStableSystemUpdateVersions('v1.1.97', 'v1.1.98'), -1)
  })

  test('returns unknown for an unparsable current version', () => {
    assert.equal(
      compareStableSystemUpdateVersions('v1.1.101', 'development'),
      null
    )
  })
})

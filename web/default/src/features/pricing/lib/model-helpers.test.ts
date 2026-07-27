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
import { describe, test } from 'node:test'

import type { PricingModel } from '../types'
import { getPricingGroupDisplayName } from './model-helpers'

const model = {
  enable_groups: ['1'],
  enable_group_refs: [{ id: 1, name: 'Premium' }],
} as PricingModel

describe('getPricingGroupDisplayName', () => {
  test('uses the group name for a numeric group ID', () => {
    assert.equal(getPricingGroupDisplayName(model, '1'), 'Premium')
  })

  test('keeps the group value when its reference is unavailable', () => {
    assert.equal(getPricingGroupDisplayName(model, '3'), '3')
  })
})

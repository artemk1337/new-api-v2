/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { SORT_OPTIONS } from '../constants'
import type { PricingModel } from '../types'
import { filterAndSortModels } from './filters'

function model(
  model_name: string,
  enable_groups: string[],
  group_ratio: Record<string, number>
): PricingModel {
  return {
    id: model_name.length,
    model_name,
    quota_type: 0,
    model_ratio: 1,
    completion_ratio: 1,
    enable_groups,
    group_ratio,
  }
}

const noFilters = {
  search: '',
  vendor: 'all',
  group: 'all',
  quotaType: 'all',
  endpointType: 'all',
  tag: 'all',
}

describe('filterAndSortModels benefit sorting', () => {
  test('sorts by descending benefit and uses the selected group ratio', () => {
    const models = [
      model('minimum-group', ['standard', 'sale'], { standard: 1, sale: 0.5 }),
      model('selected-group', ['standard', 'sale'], {
        standard: 0.2,
        sale: 0.9,
      }),
    ]

    const sorted = filterAndSortModels(models, {
      ...noFilters,
      group: 'standard',
      sortBy: SORT_OPTIONS.BENEFIT,
    })

    assert.deepEqual(
      sorted.map((item) => item.model_name),
      ['selected-group', 'minimum-group']
    )
  })

  test('uses the minimum enabled group ratio and model name as a tie-breaker', () => {
    const models = [
      model('zeta', ['standard', 'sale'], { standard: 1, sale: 0.4 }),
      model('alpha', ['standard', 'sale'], { standard: 0.6, sale: 1 }),
      model('surcharge', ['standard'], { standard: 1.2 }),
    ]

    const sorted = filterAndSortModels(models, {
      ...noFilters,
      sortBy: SORT_OPTIONS.BENEFIT,
    })

    assert.deepEqual(
      sorted.map((item) => item.model_name),
      ['zeta', 'alpha', 'surcharge']
    )
  })
})

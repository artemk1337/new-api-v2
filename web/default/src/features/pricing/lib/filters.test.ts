/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { describe, expect, test } from 'bun:test'

import { SORT_OPTIONS } from '../constants'
import type { PricingGroupPrice, PricingModel } from '../types'
import { filterAndSortModels } from './filters'

function model(name: string, prices: PricingGroupPrice[]): PricingModel {
  return {
    id: name.length,
    model_name: name,
    quota_type: 0,
    model_ratio: 100,
    completion_ratio: 10,
    enable_groups: prices.map((price) => price.group),
    group_prices: prices,
  }
}

function price(
  group: string,
  input: number,
  unit = 'million_tokens'
): PricingGroupPrice {
  return {
    group_id: Number(group),
    group,
    group_name: `Group ${group}`,
    billing_mode: unit === 'million_tokens' ? 'ratio' : 'fixed',
    price_unit: unit,
    ...(unit === 'million_tokens'
      ? { input_usd: input, output_usd: input * 2 }
      : { price_usd: input }),
  }
}

const noFilters = {
  search: '',
  vendor: 'all',
  group: 'all',
  quotaType: 'all',
  modelType: 'all',
  tag: 'all',
}

function sortedNames(
  models: PricingModel[],
  sortBy: string,
  group = 'all'
): string[] {
  return filterAndSortModels(models, { ...noFilters, sortBy, group }).map(
    (item) => item.model_name
  )
}

describe('final group price sorting', () => {
  test('price sorting follows one complete input/output pair and skips unavailable groups', () => {
    const cheapest: PricingGroupPrice = {
      ...price('2', 0.02),
      output_usd: 0.03,
    }
    const models = [
      model('cheaper-pair', [{ ...price('1', 0.01), output_usd: 2 }, cheapest]),
      model('smaller-input', [{ ...price('1', 0.015), output_usd: 0.04 }]),
    ]
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'cheaper-pair',
      'smaller-input',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_HIGH)).toEqual([
      'smaller-input',
      'cheaper-pair',
    ])
    cheapest.unavailable = true
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'smaller-input',
      'cheaper-pair',
    ])
  })
  test('sorts benefit by savings on both components rather than absolute price', () => {
    const models: PricingModel[] = [
      {
        ...model('small-saving', [price('1', 0.01), price('2', 0.001)]),
        base_reference: {
          input_usd: 0.02,
          output_usd: 0.04,
          price_unit: 'million_tokens',
        },
      },
      {
        ...model('large-saving', [price('1', 1), price('2', 10)]),
        base_reference: {
          input_usd: 10,
          output_usd: 20,
          price_unit: 'million_tokens',
        },
      },
      model('no-reference', [price('1', 0)]),
    ]
    expect(sortedNames(models, SORT_OPTIONS.BENEFIT, '1')).toEqual([
      'large-saving',
      'small-saving',
      'no-reference',
    ])
    expect(sortedNames(models, SORT_OPTIONS.BENEFIT, '2')).toEqual([
      'small-saving',
      'large-saving',
    ])
    expect(
      filterAndSortModels(models, noFilters).map((item) => item.model_name)
    ).toEqual(['small-saving', 'large-saving', 'no-reference'])
    expect(sortedNames(models, SORT_OPTIONS.NAME)).toEqual([
      'large-saving',
      'no-reference',
      'small-saving',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'no-reference',
      'small-saving',
      'large-saving',
    ])
  })
  test('sorts the final price for the selected group', () => {
    const models = [
      model('first', [price('1', 5), price('2', 1)]),
      model('second', [price('1', 2), price('2', 10)]),
    ]
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'first',
      'second',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW, '1')).toEqual([
      'second',
      'first',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_HIGH, '1')).toEqual([
      'first',
      'second',
    ])
  })

  test('keeps missing prices last and free prices first', () => {
    const models = [
      model('unknown', []),
      model('paid', [price('1', 1)]),
      model('free', [price('1', 0)]),
    ]
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'free',
      'paid',
      'unknown',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_HIGH)).toEqual([
      'paid',
      'free',
      'unknown',
    ])
  })

  test('does not compare request prices with token prices', () => {
    const models = [
      model('request', [price('1', 0.01, 'request')]),
      model('token', [price('1', 100)]),
    ]
    expect(sortedNames(models, SORT_OPTIONS.PRICE_LOW)).toEqual([
      'token',
      'request',
    ])
    expect(sortedNames(models, SORT_OPTIONS.PRICE_HIGH)).toEqual([
      'token',
      'request',
    ])
  })

  test('quota filter follows the selected group billing unit', () => {
    const models = [model('mixed', [price('1', 1), price('2', 0.1, 'request')])]
    expect(
      filterAndSortModels(models, {
        ...noFilters,
        group: '2',
        quotaType: 'token',
        sortBy: SORT_OPTIONS.NAME,
      })
    ).toEqual([])
    expect(
      filterAndSortModels(models, {
        ...noFilters,
        group: '2',
        quotaType: 'request',
        sortBy: SORT_OPTIONS.NAME,
      })
    ).toHaveLength(1)
  })
})

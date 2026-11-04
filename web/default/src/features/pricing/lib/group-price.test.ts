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
import { describe, expect, test } from 'bun:test'

import type { PricingGroupPrice, PricingModel } from '../types'
import {
  formatRetailPrice,
  getDisplayGroupPrice,
  getPricingRateFields,
  hasTwoContextTiers,
} from './group-price'

const quote = (
  group: string,
  rates: Partial<PricingGroupPrice> = {}
): PricingGroupPrice => ({
  group_id: Number(group),
  group,
  group_name: `Group ${group}`,
  billing_mode: 'ratio',
  price_unit: 'million_tokens',
  ...rates,
})

const model = (group_prices: PricingGroupPrice[]): PricingModel => ({
  id: 1,
  model_name: 'test-model',
  quota_type: 0,
  model_ratio: 999,
  completion_ratio: 999,
  enable_groups: ['1', '2'],
  group_ratio: { '1': 0.00001, '2': 0.5 },
  group_prices,
})

describe('retail group prices', () => {
  test('short and long labels require exactly two continuous input ranges', () => {
    expect(
      hasTwoContextTiers([
        { max_input_tokens: 100000 },
        { min_input_tokens: 100001 },
      ])
    ).toBe(true)
    expect(
      hasTwoContextTiers([
        { max_input_tokens: 100000 },
        { min_input_tokens: 200001 },
      ])
    ).toBe(false)
    expect(hasTwoContextTiers([{ label: 'step_1' }, { label: 'step_2' }])).toBe(
      false
    )
    expect(hasTwoContextTiers([{ max_input_tokens: 100000 }])).toBe(false)
  })
  test('keeps input, output and cache from the same group', () => {
    const item = model([
      quote('1', { input_usd: 1, output_usd: 20, cache_read_usd: 0.3 }),
      quote('2', { input_usd: 2, output_usd: 5, cache_read_usd: 0.1 }),
    ])
    const selected = getDisplayGroupPrice(item)
    expect(selected?.group).toBe('1')
    expect(selected?.output_usd).toBe(20)
    expect(selected?.cache_read_usd).toBe(0.3)
    expect(getDisplayGroupPrice(item, '2')?.input_usd).toBe(2)
  })

  test('does not fall back to another group or legacy global pricing', () => {
    const item = model([quote('1', { input_usd: 1, output_usd: 2 })])
    expect(getDisplayGroupPrice(item, '2')).toBeUndefined()
    expect(getDisplayGroupPrice(model([]))).toBeUndefined()
    expect(
      getDisplayGroupPrice(model([quote('1', { unavailable: true })]))
    ).toBeUndefined()
  })

  test('preserves free rates and keeps missing components unknown', () => {
    const rates = quote('1', {
      input_usd: 0,
      output_usd: 0,
      cache_read_usd: null,
    })
    expect(getDisplayGroupPrice(model([rates]))?.input_usd).toBe(0)
    expect(getPricingRateFields([rates]).map((field) => field.key)).toEqual([
      'input_usd',
      'output_usd',
    ])
    expect(formatRetailPrice(null, { tokenUnit: 'M' })).toBe('—')
    expect(formatRetailPrice(undefined, { tokenUnit: 'M' })).toBe('—')
    expect(formatRetailPrice(Number.NaN, { tokenUnit: 'M' })).toBe('—')
    expect(formatRetailPrice(0, { tokenUnit: 'M' })).not.toBe('—')
  })

  test('formats final retail rates once and only rescales token units', () => {
    expect(formatRetailPrice(2, { tokenUnit: 'M' })).toBe('$2')
    expect(formatRetailPrice(2, { tokenUnit: 'K' })).toBe('$0.002')
    expect(formatRetailPrice(2, { tokenUnit: 'K' }, false)).toBe('$2')
  })

  test('retains each group cache write and hourly cache prices', () => {
    const prices = [
      quote('1', {
        input_usd: 1,
        cache_write_usd: 1.25,
        cache_write_1h_usd: 2,
      }),
      quote('2', { input_usd: 2, cache_write_usd: 4, cache_write_1h_usd: 8 }),
    ]
    expect(getDisplayGroupPrice(model(prices), '2')?.cache_write_1h_usd).toBe(8)
    expect(getPricingRateFields(prices).map((field) => field.key)).toEqual([
      'input_usd',
      'cache_write_usd',
      'cache_write_1h_usd',
    ])
  })
})

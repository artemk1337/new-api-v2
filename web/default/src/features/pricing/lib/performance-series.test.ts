/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { PerformanceGroup } from '@/features/performance-metrics/types'

import type { PricingModel } from '../types'
import { buildPerformanceChartSeries } from './performance-series'

const model = {
  model_name: 'test-model',
  enable_group_refs: [
    { id: 1, name: 'default' },
    { id: 2, name: 'vip' },
  ],
} as PricingModel

const groups: PerformanceGroup[] = [
  {
    group: '1',
    avg_ttft_ms: 120,
    avg_latency_ms: 945,
    success_rate: 98,
    avg_tps: 52.9,
    series: [
      {
        ts: 100,
        avg_ttft_ms: 100,
        avg_latency_ms: 900,
        success_rate: 99,
        avg_tps: 50,
      },
      {
        ts: 200,
        avg_ttft_ms: 120,
        avg_latency_ms: 950,
        success_rate: 98,
        avg_tps: 55,
      },
    ],
  },
  {
    group: '2',
    avg_ttft_ms: 700,
    avg_latency_ms: 5000,
    success_rate: 95,
    avg_tps: 33.3,
    series: [
      {
        ts: 100,
        avg_ttft_ms: 700,
        avg_latency_ms: 5000,
        success_rate: 92,
        avg_tps: 33,
      },
    ],
  },
]

describe('buildPerformanceChartSeries', () => {
  test('keeps latency and uptime points separated by group', () => {
    const result = buildPerformanceChartSeries(groups, model)

    assert.deepEqual(
      result.latency.map((point) => [point.group, point.ttft_ms]),
      [
        ['default', 100],
        ['default', 120],
        ['vip', 700],
      ]
    )
    assert.deepEqual(
      result.uptime.map((point) => [point.group, point.uptime_pct]),
      [
        ['default', 99],
        ['default', 98],
        ['vip', 92],
      ]
    )
    assert.notEqual(result.colors.default, result.colors.vip)
  })

  test('returns empty series when no performance groups exist', () => {
    assert.deepEqual(buildPerformanceChartSeries([], model), {
      latency: [],
      uptime: [],
      colors: {},
    })
  })

  test('preserves a group with a single data point', () => {
    const result = buildPerformanceChartSeries([groups[1]], model)

    assert.equal(result.latency.length, 1)
    assert.equal(result.uptime.length, 1)
    assert.equal(result.uptime[0]?.group, 'vip')
  })

  test('keeps sparse points and normalizes an invalid uptime value', () => {
    const result = buildPerformanceChartSeries(
      [
        {
          ...groups[0],
          series: [
            {
              ...groups[0].series[0]!,
              avg_ttft_ms: 0,
              success_rate: Number.NaN,
            },
          ],
        },
      ],
      model
    )

    assert.deepEqual(result.latency, [])
    assert.deepEqual(result.uptime[0], {
      date: new Date(100 * 1000).toISOString(),
      group: 'default',
      uptime_pct: 0,
      incidents: 1,
      outage_minutes: 0,
    })
  })
})

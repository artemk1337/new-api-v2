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
import { getGroupChartColor } from '@/lib/colors'

import type { PerformanceGroup } from '@/features/performance-metrics/types'
import type { PricingModel } from '../types'
import type { LatencyTimePoint, UptimeDayPoint } from './mock-stats'
import { getPricingGroupDisplayName } from './model-helpers'

function toUptimePercent(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.round(Math.min(100, Math.max(0, value)) * 100) / 100
}

export type PerformanceChartSeries = {
  latency: LatencyTimePoint[]
  uptime: UptimeDayPoint[]
  colors: Record<string, string>
}

export function getPerformanceGroupLabel(
  model: PricingModel,
  group: PerformanceGroup
): string {
  return group.group_ref?.name ?? getPricingGroupDisplayName(model, group.group)
}

export function buildPerformanceChartSeries(
  groups: PerformanceGroup[],
  model: PricingModel
): PerformanceChartSeries {
  const latency: LatencyTimePoint[] = []
  const uptime: UptimeDayPoint[] = []
  const colors: Record<string, string> = {}

  for (const group of groups) {
    const label = getPerformanceGroupLabel(model, group)
    colors[label] = getGroupChartColor(label)

    for (const point of group.series) {
      if (point.avg_ttft_ms > 0) {
        latency.push({
          timestamp: new Date(point.ts * 1000).toISOString(),
          group: label,
          ttft_ms: point.avg_ttft_ms,
        })
      }
      const uptimePercent = toUptimePercent(point.success_rate)
      uptime.push({
        date: new Date(point.ts * 1000).toISOString(),
        group: label,
        uptime_pct: uptimePercent,
        incidents: uptimePercent < 100 ? 1 : 0,
        outage_minutes: 0,
      })
    }
  }

  return { latency, uptime, colors }
}

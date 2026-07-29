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
import { api } from '@/lib/api'

import type {
  SystemInstanceListResponse,
  SystemTelemetryAgentResponse,
  SystemTelemetryResponse,
} from './types'

export async function listSystemInstances() {
  const res = await api.get<SystemInstanceListResponse>(
    '/api/system-info/instances'
  )
  return res.data
}

export async function getSystemTelemetry(nodeName: string, hours: 1 | 6 | 24) {
  const res = await api.get<SystemTelemetryResponse>(
    '/api/system-info/telemetry',
    {
      params: { node_name: nodeName, hours },
    }
  )
  return res.data
}

export async function getSystemTelemetryAgent() {
  const res = await api.get<SystemTelemetryAgentResponse>(
    '/api/system-info/telemetry-agent'
  )
  return res.data
}

export async function startSystemTelemetryAgent() {
  const res = await api.post<SystemTelemetryAgentResponse>(
    '/api/system-info/telemetry-agent/start'
  )
  return res.data
}

export async function stopSystemTelemetryAgent() {
  const res = await api.delete<SystemTelemetryAgentResponse>(
    '/api/system-info/telemetry-agent/stop'
  )
  return res.data
}

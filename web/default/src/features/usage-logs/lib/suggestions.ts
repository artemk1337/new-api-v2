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
import type { GetLogsParams, LogSuggestionField } from '../types'

export type LogFilterField = LogSuggestionField | 'proxy_supplier_id'

export type LogFilterActor = { id: number; role: number }

export function parseProxySupplierId(value: unknown): string | undefined {
  if (typeof value !== 'number' && typeof value !== 'string') return undefined
  const text = String(value)
  if (!/^(0|[1-9]\d*)$/.test(text)) return undefined
  return Number.isSafeInteger(Number(text)) ? text : undefined
}

export function logSuggestionScope(
  params: GetLogsParams,
  field: LogFilterField
): Omit<GetLogsParams, 'p' | 'page_size'> {
  const { p: _page, page_size: _pageSize, ...scope } = params
  delete scope[field]
  return scope
}

export function logSuggestionQueryKey(
  actor: LogFilterActor,
  field: LogFilterField,
  query: string,
  scope: GetLogsParams
) {
  return [
    'log-filter-options',
    actor.id,
    actor.role,
    field,
    query,
    scope,
  ] as const
}

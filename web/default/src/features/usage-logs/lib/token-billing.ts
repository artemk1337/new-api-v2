/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import type { UsageLog } from '../data/schema'
import type { LogOtherData } from '../types'

export interface TokenBillingParts {
  input: number
  output: number
  cacheRead: number
  cacheWrite: number
  cacheWrite5m: number
  cacheWrite1h: number
  cacheWriteResidual: number
  baseInput: number
}

export function getTokenBillingParts(
  log: UsageLog,
  other: LogOtherData
): TokenBillingParts {
  const input = Math.max(0, log.prompt_tokens || 0)
  const output = Math.max(0, log.completion_tokens || 0)
  const cacheRead = Math.max(0, other.cache_tokens || 0)
  const cacheWrite = Math.max(0, other.cache_creation_tokens || 0)
  const cacheWrite5m = Math.max(0, other.cache_creation_tokens_5m || 0)
  const cacheWrite1h = Math.max(0, other.cache_creation_tokens_1h || 0)
  const cacheWriteResidual = Math.max(
    0,
    cacheWrite - cacheWrite5m - cacheWrite1h
  )
  const baseInput = other.claude
    ? input
    : Math.max(
        0,
        input - cacheRead - cacheWrite5m - cacheWrite1h - cacheWriteResidual
      )

  return {
    input,
    output,
    cacheRead,
    cacheWrite,
    cacheWrite5m,
    cacheWrite1h,
    cacheWriteResidual,
    baseInput,
  }
}

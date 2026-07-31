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
export type RateLimitDurationUnit = 's' | 'm' | 'h'

export type RateLimitDuration = {
  value: number
  unit: RateLimitDurationUnit
}

export type RateLimitPeriodState = {
  effectiveDuration: string
  stagedDuration: string
  isDraft: boolean
  activationPending: boolean
  canActivate: boolean
}

export const rateLimitDurationActivationUpdate = {
  key: 'ModelRequestRateLimitDurationActivated',
  value: true,
} as const

const rateLimitDurationPattern = /^([1-9]\d*)([smh])$/
const activationRefreshRetryInterval = 5_000

export function parseRateLimitDuration(
  duration: string
): RateLimitDuration | null {
  const match = rateLimitDurationPattern.exec(duration)
  if (!match) return null

  const value = Number(match[1])
  if (!Number.isSafeInteger(value)) return null

  return { value, unit: match[2] as RateLimitDurationUnit }
}

export function formatRateLimitDuration(duration: RateLimitDuration): string {
  return `${duration.value}${duration.unit}`
}

export function getRateLimitDurationUnit(
  duration: string,
  fallbackUnit: RateLimitDurationUnit
): RateLimitDurationUnit {
  const parsed = parseRateLimitDuration(duration)
  if (parsed) return parsed.unit

  const unit = duration.at(-1)
  if (unit === 's' || unit === 'm' || unit === 'h') return unit

  return fallbackUnit
}

export function getRateLimitDuration(
  duration: string,
  legacyMinutes: number
): RateLimitDuration {
  const parsed = parseRateLimitDuration(duration)
  if (parsed) return parsed

  if (Number.isSafeInteger(legacyMinutes) && legacyMinutes > 0) {
    return { value: legacyMinutes, unit: 'm' }
  }
  return { value: 1, unit: 'm' }
}

export function getRateLimitActivationRefreshDelay(
  activationAt: number,
  now: number,
  hasRefreshed = false
): number | null {
  if (!Number.isSafeInteger(activationAt) || activationAt <= 0) return null
  if (hasRefreshed) return activationRefreshRetryInterval
  return Math.max(0, activationAt * 1000 - now)
}

export function shouldSaveRateLimitDuration(
  duration: string,
  savedDuration: string,
  staged: boolean
): boolean {
  return !staged || duration !== savedDuration
}

export function getRateLimitPeriodState(
  savedDuration: string,
  legacyMinutes: number,
  activated: boolean,
  active: boolean,
  staged: boolean,
  draftDuration: string
): RateLimitPeriodState {
  const stagedDuration = formatRateLimitDuration(
    getRateLimitDuration(savedDuration, legacyMinutes)
  )
  const legacyDuration = formatRateLimitDuration(
    getRateLimitDuration('', legacyMinutes)
  )
  const isDraft = draftDuration !== stagedDuration

  return {
    effectiveDuration: active ? stagedDuration : legacyDuration,
    stagedDuration,
    isDraft,
    activationPending: activated && !active,
    canActivate:
      staged &&
      !activated &&
      !isDraft &&
      parseRateLimitDuration(draftDuration) !== null,
  }
}

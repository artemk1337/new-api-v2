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
import type { SecuritySettings, SystemOption } from '../types'

export function resolveSecuritySettings(
  settings: SecuritySettings,
  options: SystemOption[] | undefined
): SecuritySettings {
  const activationOption = options?.find(
    (option) => option.key === 'ModelRequestRateLimitDurationActivated'
  )
  const activationAtOption = options?.find(
    (option) => option.key === 'ModelRequestRateLimitDurationActivationAt'
  )
  const activeOption = options?.find(
    (option) => option.key === 'ModelRequestRateLimitDurationActive'
  )
  const stagedOption = options?.find(
    (option) => option.key === 'ModelRequestRateLimitDurationStaged'
  )
  const activationAt = Number(activationAtOption?.value)
  const resolvedSettings = {
    ...settings,
    ModelRequestRateLimitDurationActivationAt:
      Number.isSafeInteger(activationAt) && activationAt > 0 ? activationAt : 0,
    ModelRequestRateLimitDurationActive:
      activeOption?.value === 'true' || activeOption?.value === '1',
    ModelRequestRateLimitDurationActivated:
      activationOption?.value === 'true' || activationOption?.value === '1',
    ModelRequestRateLimitDurationStaged:
      stagedOption?.value === 'true' || stagedOption?.value === '1',
  }
  const hasDuration = options?.some(
    (option) => option.key === 'ModelRequestRateLimitDuration'
  )
  if (hasDuration) return resolvedSettings

  if (settings.ModelRequestRateLimitDurationMinutes > 0) {
    return {
      ...resolvedSettings,
      ModelRequestRateLimitDuration: `${settings.ModelRequestRateLimitDurationMinutes}m`,
    }
  }
  return resolvedSettings
}

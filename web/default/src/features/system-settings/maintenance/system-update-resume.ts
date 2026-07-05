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
import type { SystemUpdateTask } from '../types'

function parseStableVersion(version: string | null | undefined) {
  const match = version?.trim().match(/^v?(\d+)\.(\d+)\.(\d+)$/)
  if (!match) return null
  return match.slice(1).map(Number)
}

function isNewerStableVersion(
  requestedVersion: string,
  currentVersion: string | null | undefined
) {
  const requested = parseStableVersion(requestedVersion)
  const current = parseStableVersion(currentVersion)
  if (!requested || !current) return currentVersion?.trim() !== requestedVersion

  for (let i = 0; i < requested.length; i++) {
    if (requested[i] !== current[i]) return requested[i] > current[i]
  }
  return false
}

export function shouldResumeDeployingSystemUpdateTask(
  task: SystemUpdateTask | null | undefined,
  currentVersion: string | null | undefined
): task is SystemUpdateTask {
  const requestedVersion = task?.result?.requested_version?.trim()
  return (
    task?.type === 'system_update' &&
    task.result?.status === 'deploying' &&
    Boolean(task.result?.job_id) &&
    Boolean(requestedVersion) &&
    isNewerStableVersion(requestedVersion, currentVersion)
  )
}

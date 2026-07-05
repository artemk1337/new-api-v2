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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import type { SystemUpdateTask } from '../types'
import { shouldResumeDeployingSystemUpdateTask } from './system-update-resume'

const deployingTask: SystemUpdateTask = {
  id: 1,
  task_id: 'systask_update',
  type: 'system_update',
  status: 'succeeded',
  payload: { version: 'v1.1.19' },
  state: {
    step: 'deploying',
    progress: 100,
    message: 'update image pulled; deploying service',
  },
  result: {
    previous_version: 'v1.1.18',
    requested_version: 'v1.1.19',
    image: 'ghcr.io/artemk1337/new-api-v2:v1.1.19',
    job_id: 'job-1',
    status: 'deploying',
  },
  error: '',
  locked_by: 'runner-a',
  created_at: 1,
  updated_at: 2,
}

describe('system update resume guard', () => {
  test('does not resume reload polling after requested version is already running', () => {
    assert.equal(
      shouldResumeDeployingSystemUpdateTask(deployingTask, 'v1.1.19'),
      false
    )
  })

  test('resumes deployment polling while the old version is still running', () => {
    assert.equal(
      shouldResumeDeployingSystemUpdateTask(deployingTask, 'v1.1.18'),
      true
    )
  })

  test('does not resume stale deployment polling after a newer version is running', () => {
    assert.equal(
      shouldResumeDeployingSystemUpdateTask(deployingTask, 'v1.1.20'),
      false
    )
  })
})

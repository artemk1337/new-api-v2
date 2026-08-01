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

import type { SystemUpdateRelease } from '../types'
import {
  canDeploySystemUpdateRelease,
  getSystemUpdateReleaseBuildStatus,
} from './system-update-release-status'

describe('system update release status', () => {
  test('keeps releases without a build status unavailable for deployment', () => {
    const release: SystemUpdateRelease = { tag_name: 'v1.2.3' }

    assert.equal(getSystemUpdateReleaseBuildStatus(release), 'unavailable')
    assert.equal(canDeploySystemUpdateRelease(release), false)
  })

  test('does not treat an unavailable build as deployable', () => {
    const release: SystemUpdateRelease = {
      tag_name: 'v1.2.3',
      build_status: 'unavailable',
      ready_to_deploy: false,
    }

    assert.equal(getSystemUpdateReleaseBuildStatus(release), 'unavailable')
    assert.equal(canDeploySystemUpdateRelease(release), false)
  })

  test('allows a ready release only after the backend marks it deployable', () => {
    const release: SystemUpdateRelease = {
      tag_name: 'v1.2.3',
      build_status: 'ready',
      ready_to_deploy: true,
    }

    assert.equal(getSystemUpdateReleaseBuildStatus(release), 'ready')
    assert.equal(canDeploySystemUpdateRelease(release), true)
  })
})

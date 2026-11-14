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
import { expect, it } from 'bun:test'

import { AxiosError } from 'axios'

import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'

import { sendImageGeneration } from './api'

it('keeps the browser session when the image supplier rejects its upstream key', async () => {
  const previousAdapter = api.defaults.adapter
  const previousUser = useAuthStore.getState().auth.user
  const user = { id: 1, username: 'image-test', role: 1 }
  useAuthStore.getState().auth.setUser(user)
  api.defaults.adapter = async (config) => {
    throw new AxiosError(
      'Request failed with status code 401',
      undefined,
      config,
      undefined,
      {
        config,
        status: 401,
        statusText: 'Unauthorized',
        headers: {},
        data: { error: { message: 'Invalid token', code: 'invalid_api_key' } },
      }
    )
  }

  try {
    await expect(
      sendImageGeneration({
        model: 'gpt-image-2.6',
        group: '49',
        prompt: 'A cat',
        n: 1,
      })
    ).rejects.toMatchObject({ response: { status: 401 } })
    expect(useAuthStore.getState().auth.user).toEqual(user)
  } finally {
    api.defaults.adapter = previousAdapter
    useAuthStore.getState().auth.setUser(previousUser)
  }
})

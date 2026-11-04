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
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'

export type Announcement = {
  id: string
  content: string
  publishDate: string
  type: 'default' | 'ongoing' | 'success' | 'warning' | 'error'
  extra?: string
  telegramUrl?: string
  telegramMode: 'platform' | 'link' | 'bot'
  telegramText?: string
  telegramStatus?: string
  telegramError?: string
}

export type AnnouncementSettings = {
  announcements: Announcement[]
  channel: string
  revision: string
}

type AnnouncementResponse = {
  success: boolean
  message?: string
  data: AnnouncementSettings
}

const queryKey = ['announcement-settings']

function unwrap(response: AnnouncementResponse) {
  if (!response.success) {
    throw new Error(response.message || 'Failed to save announcements')
  }
  return response.data
}

export function useAnnouncementSettings() {
  return useQuery({
    queryKey,
    queryFn: async () =>
      unwrap((await api.get<AnnouncementResponse>('/api/announcements/')).data),
    refetchInterval: (query) =>
      query.state.data?.announcements.some((item) =>
        ['pending', 'sending'].includes(item.telegramStatus || '')
      )
        ? 2000
        : false,
  })
}

export function useSaveAnnouncements() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (settings: AnnouncementSettings) =>
      unwrap(
        (
          await api.put<AnnouncementResponse>('/api/announcements/', settings, {
            skipBusinessError: true,
          })
        ).data
      ),
    onSuccess: (settings) => {
      client.setQueryData(queryKey, settings)
      void client.invalidateQueries({ queryKey: ['status'] })
      void client.invalidateQueries({ queryKey: ['system-options'] })
    },
  })
}

export function useRetryAnnouncement() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: async (request: { id: string; confirmUnknown: boolean }) =>
      unwrap(
        (
          await api.post<AnnouncementResponse>(
            `/api/announcements/${encodeURIComponent(request.id)}/retry`,
            { confirmUnknown: request.confirmUnknown },
            { skipBusinessError: true }
          )
        ).data
      ),
    onSuccess: (settings) => client.setQueryData(queryKey, settings),
  })
}

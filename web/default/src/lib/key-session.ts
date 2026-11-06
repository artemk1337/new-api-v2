import { api } from '@/lib/api'

export interface KeySession {
  name: string
  used_quota: number
  remain_quota: number
  unlimited_quota: boolean
  expired_time: number
}

export async function getKeySession(): Promise<KeySession | null> {
  try {
    const response = await api.get<{ success: boolean; data: KeySession }>(
      '/api/key-session',
      { skipErrorHandler: true, skipBusinessError: true, skipAuthReset: true }
    )
    return response.data.success ? response.data.data : null
  } catch {
    return null
  }
}

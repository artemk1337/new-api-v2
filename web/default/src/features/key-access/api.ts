import { api } from '@/lib/api'

export interface KeySession {
  name: string
  used_quota: number
  remain_quota: number
  unlimited_quota: boolean
  expired_time: number
}

export interface KeyLog {
  id: number
  created_at: number
  model_name: string
  quota: number
  prompt_tokens: number
  completion_tokens: number
}

interface Response<T> {
  success: boolean
  message: string
  data: T
}

const config = {
  skipErrorHandler: true,
  skipBusinessError: true,
  skipAuthReset: true,
}

export async function getKeySession(): Promise<KeySession | null> {
  try {
    const response = await api.get<Response<KeySession>>(
      '/api/key-session',
      config
    )
    return response.data.success ? response.data.data : null
  } catch {
    return null
  }
}

export async function loginWithKey(key: string): Promise<void> {
  const response = await api.post<Response<unknown>>(
    '/api/key-login',
    { key },
    config
  )
  if (!response.data.success) throw new Error(response.data.message)
}

export async function getKeyLogs(): Promise<KeyLog[]> {
  const response = await api.get<Response<KeyLog[]>>(
    '/api/key-session/logs',
    config
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data || []
}

export async function logoutKey(): Promise<void> {
  const response = await api.post<Response<unknown>>(
    '/api/key-session/logout',
    undefined,
    config
  )
  if (!response.data.success) throw new Error(response.data.message)
}

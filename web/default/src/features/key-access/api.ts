import { api } from '@/lib/api'

export { getKeySession } from '@/lib/key-session'

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

export async function loginWithKey(key: string): Promise<void> {
  const response = await api.post<Response<unknown>>(
    '/api/key-login',
    { key },
    config
  )
  if (!response.data.success) throw new Error(response.data.message)
}

export interface KeyLogsPage {
  page: number
  page_size: number
  total: number
  items: KeyLog[]
}

export async function getKeyLogs(page: number): Promise<KeyLogsPage> {
  const response = await api.get<Response<KeyLogsPage>>(
    '/api/key-session/logs',
    { ...config, params: { p: page } }
  )
  if (!response.data.success) throw new Error(response.data.message)
  return response.data.data
}

export async function logoutKey(): Promise<void> {
  const response = await api.post<Response<unknown>>(
    '/api/key-session/logout',
    undefined,
    config
  )
  if (!response.data.success) throw new Error(response.data.message)
}

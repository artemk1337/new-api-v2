import { api } from '@/lib/api'

import type { ApiResponse } from './types'

export interface UserToken {
  id: number
  name: string
  status: number
  group: string
}

interface UserTokensPage {
  items: UserToken[]
  total: number
}

export async function getUserTokens(
  userId: number,
  page: number
): Promise<UserTokensPage> {
  const response = await api.get<ApiResponse<UserTokensPage>>(
    `/api/user/${userId}/tokens`,
    { params: { p: page, page_size: 10 } }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message)
  }
  return response.data.data
}

export async function setUserTokenStatus(
  userId: number,
  tokenId: number,
  status: 1 | 2
): Promise<UserToken> {
  const response = await api.put<ApiResponse<UserToken>>(
    `/api/user/${userId}/tokens/${tokenId}/status`,
    { status }
  )
  if (!response.data.success || !response.data.data) {
    throw new Error(response.data.message)
  }
  return response.data.data
}

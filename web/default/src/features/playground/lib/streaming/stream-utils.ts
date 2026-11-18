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
import { ERROR_MESSAGES } from '../../constants'
import type { ChatCompletionChunk } from '../../types'

const STREAM_DONE_MESSAGE = '[DONE]'

export type StreamUpdateType = 'reasoning' | 'content'

export type StreamMessageUpdate = {
  type: StreamUpdateType
  chunk: string
}

type StreamErrorPayload = {
  error?: {
    code?: string
    message?: string
  }
}

export type StreamErrorDetails = {
  errorCode?: string
  errorMessage: string
}

export function parseStreamErrorDetails(data?: string): StreamErrorDetails {
  const fallbackMessage = data || ERROR_MESSAGES.API_REQUEST_ERROR

  if (!data) {
    return { errorMessage: fallbackMessage }
  }

  try {
    const parsed = JSON.parse(data) as StreamErrorPayload

    if (!parsed?.error) {
      return { errorMessage: fallbackMessage }
    }

    return {
      errorCode: parsed.error.code || undefined,
      errorMessage: parsed.error.message || fallbackMessage,
    }
  } catch {
    return { errorMessage: fallbackMessage }
  }
}

export function parseStreamMessage(data: string): {
  updates: StreamMessageUpdate[]
  finishReason?: string
  error?: StreamErrorDetails
} {
  const chunk = JSON.parse(data) as ChatCompletionChunk & StreamErrorPayload
  if (chunk.error) {
    return {
      updates: [],
      error: {
        errorCode: chunk.error.code || undefined,
        errorMessage: chunk.error.message || ERROR_MESSAGES.API_REQUEST_ERROR,
      },
    }
  }

  const choice = chunk.choices?.[0]
  const delta = choice?.delta
  const updates: StreamMessageUpdate[] = []

  if (delta?.reasoning_content) {
    updates.push({ type: 'reasoning', chunk: delta.reasoning_content })
  }

  if (delta?.content) {
    updates.push({ type: 'content', chunk: delta.content })
  }

  return { updates, finishReason: choice?.finish_reason ?? undefined }
}

export function isStreamDoneMessage(data: string): boolean {
  return data === STREAM_DONE_MESSAGE
}

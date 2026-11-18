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
import { useCallback, useEffect, useRef, useState } from 'react'
import { SSE } from 'sse.js'

import { getCommonHeaders } from '@/lib/api'

import { API_ENDPOINTS, ERROR_MESSAGES } from '../constants'
import {
  isStreamDoneMessage,
  parseStreamErrorDetails,
  parseStreamMessage,
} from '../lib'
import type { ChatCompletionRequest } from '../types'

/**
 * Hook for handling streaming chat completion requests
 */
export function useStreamRequest() {
  const sseSourceRef = useRef<SSE | null>(null)
  const [isStreaming, setIsStreaming] = useState(false)

  const disposeStream = useCallback(() => {
    const source = sseSourceRef.current
    sseSourceRef.current = null
    source?.close()
  }, [])

  useEffect(() => disposeStream, [disposeStream])

  const stopStream = useCallback(() => {
    disposeStream()
    setIsStreaming(false)
  }, [disposeStream])

  const sendStreamRequest = useCallback(
    (
      payload: ChatCompletionRequest,
      onUpdate: (type: 'reasoning' | 'content', chunk: string) => void,
      onComplete: (finishReason?: string) => void,
      onError: (error: string, errorCode?: string) => void
    ) => {
      disposeStream()

      const source = new SSE(API_ENDPOINTS.CHAT_COMPLETIONS, {
        headers: getCommonHeaders(),
        method: 'POST',
        payload: JSON.stringify(payload),
        start: false,
      })

      sseSourceRef.current = source
      setIsStreaming(true)
      let finishReason: string | undefined
      const isActive = () => sseSourceRef.current === source

      const handleComplete = () => {
        stopStream()
        onComplete(finishReason)
      }

      const handleError = (errorMessage: string, errorCode?: string) => {
        if (!isActive()) return
        stopStream()
        onError(errorMessage, errorCode)
      }

      source.addEventListener('message', (e: MessageEvent) => {
        if (!isActive()) return
        if (isStreamDoneMessage(e.data)) {
          handleComplete()
          return
        }

        try {
          const message = parseStreamMessage(e.data)
          if (message.error) {
            handleError(message.error.errorMessage, message.error.errorCode)
            return
          }
          finishReason = message.finishReason ?? finishReason

          for (const update of message.updates) {
            onUpdate(update.type, update.chunk)
          }
        } catch (error) {
          // eslint-disable-next-line no-console
          console.error('Failed to parse SSE message:', error)
          handleError(ERROR_MESSAGES.PARSE_ERROR)
        }
      })

      source.addEventListener('error', (e: Event & { data?: string }) => {
        if (!isActive()) return
        const { errorCode, errorMessage } = parseStreamErrorDetails(e.data)
        handleError(errorMessage, errorCode)
      })

      source.addEventListener(
        'readystatechange',
        (e: Event & { readyState?: number }) => {
          if (!isActive() || e.readyState !== SSE.CLOSED) return
          if (finishReason) handleComplete()
          else handleError(ERROR_MESSAGES.INTERRUPTED)
        }
      )

      try {
        source.stream()
      } catch (error: unknown) {
        // eslint-disable-next-line no-console
        console.error('Failed to start SSE stream:', error)
        handleError(ERROR_MESSAGES.STREAM_START_ERROR)
      }
    },
    [disposeStream, stopStream]
  )

  return {
    sendStreamRequest,
    stopStream,
    isStreaming,
  }
}

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
import { t } from 'i18next'

import { ERROR_MESSAGES, MESSAGE_ROLES, MESSAGE_STATUS } from '../../constants'
import type { ChatCompletionResponse, Message } from '../../types'
import { parseThinkTags } from './message-reasoning-utils'
import {
  completeAssistantTiming,
  completeReasoningTiming,
  startReasoningTiming,
} from './message-timing-utils'
import {
  getCurrentVersion,
  hasMessageContent,
  updateCurrentVersionContent,
} from './message-utils'

type StreamingContentMode = 'content' | 'reasoning'

export type StreamingContentAccumulator = {
  append: (type: StreamChunkType, chunk: string) => {
    content: string
    reasoning: string
  }
  finish: () => { content: string; reasoning: string }
  reset: () => void
}

function appendDedupedChunk(
  received: string,
  chunk: string
): { received: string; delta: string } {
  if (!chunk) {
    return { received, delta: '' }
  }

  if (chunk.startsWith(received)) {
    return { received: chunk, delta: chunk.slice(received.length) }
  }

  return { received: received + chunk, delta: chunk }
}

/**
 * Incrementally separates <think> content without reparsing the full stream.
 * The accumulator also accepts both cumulative and delta stream chunks.
 */
export function createStreamingContentAccumulator(): StreamingContentAccumulator {
  let mode: StreamingContentMode = 'content'
  let pendingTag = ''
  let receivedContent = ''
  let receivedReasoning = ''

  const reset = () => {
    mode = 'content'
    pendingTag = ''
    receivedContent = ''
    receivedReasoning = ''
  }

  const appendContent = (chunk: string) => {
    const deduped = appendDedupedChunk(receivedContent, chunk)
    receivedContent = deduped.received

    let visible = ''
    let reasoning = ''
    pendingTag += deduped.delta

    while (pendingTag) {
      const tag = mode === 'content' ? '<think>' : '</think>'
      const tagIndex = pendingTag.indexOf(tag)

      if (tagIndex >= 0) {
        const text = pendingTag.slice(0, tagIndex)
        if (mode === 'content') {
          visible += text
        } else {
          reasoning += text
        }
        pendingTag = pendingTag.slice(tagIndex + tag.length)
        mode = mode === 'content' ? 'reasoning' : 'content'
        continue
      }

      let pendingPrefixLength = 0
      for (
        let length = Math.min(tag.length - 1, pendingTag.length);
        length > 0;
        length--
      ) {
        if (pendingTag.endsWith(tag.slice(0, length))) {
          pendingPrefixLength = length
          break
        }
      }

      const text = pendingPrefixLength
        ? pendingTag.slice(0, -pendingPrefixLength)
        : pendingTag
      if (mode === 'content') {
        visible += text
      } else {
        reasoning += text
      }
      if (pendingPrefixLength) {
        pendingTag = pendingTag.slice(-pendingPrefixLength)
        break
      }
      pendingTag = ''
    }

    return { content: visible, reasoning }
  }

  const finish = () => {
    const result =
      mode === 'content'
        ? { content: pendingTag, reasoning: '' }
        : { content: '', reasoning: pendingTag }
    pendingTag = ''
    return result
  }

  return {
    append: (type, chunk) => {
      if (type === 'content') {
        return appendContent(chunk)
      }

      const deduped = appendDedupedChunk(receivedReasoning, chunk)
      receivedReasoning = deduped.received
      return { content: '', reasoning: deduped.delta }
    },
    finish,
    reset,
  }
}

/**
 * Process content chunk during streaming.
 * Separates <think> reasoning from visible content in real-time.
 * Note: versions[0].content keeps the full raw content with tags during streaming.
 */
export function processStreamingContent(
  message: Message,
  contentChunk?: string
): Message {
  const currentVersion = getCurrentVersion(message)
  const fullContent = contentChunk
    ? currentVersion.content + contentChunk
    : currentVersion.content

  if (!fullContent.includes('<think>')) {
    return {
      ...updateCurrentVersionContent(message, fullContent),
      isReasoningStreaming: false,
    }
  }

  const { reasoning, hasUnclosedTag } = parseThinkTags(fullContent)
  const finalReasoning = reasoning
    ? {
        ...startReasoningTiming(message),
        content: reasoning,
      }
    : message.reasoning

  return {
    ...updateCurrentVersionContent(message, fullContent),
    reasoning: finalReasoning,
    isReasoningStreaming: hasUnclosedTag,
  }
}

export type StreamChunkType = 'reasoning' | 'content'

function getAppendableChunk(currentContent: string, chunk: string): string {
  if (!currentContent || !chunk.startsWith(currentContent)) {
    return chunk
  }

  return chunk.slice(currentContent.length)
}

export function applyStreamingChunk(
  message: Message,
  type: StreamChunkType,
  chunk: string
): Message {
  if (message.status === MESSAGE_STATUS.ERROR) {
    return message
  }

  if (type === 'reasoning') {
    const reasoning = startReasoningTiming(message)
    const appendableChunk = getAppendableChunk(reasoning.content, chunk)

    return {
      ...message,
      reasoning: {
        ...reasoning,
        content: reasoning.content + appendableChunk,
      },
      isReasoningStreaming: true,
      status: MESSAGE_STATUS.STREAMING,
    }
  }

  const currentVersion = getCurrentVersion(message)
  const appendableChunk = getAppendableChunk(currentVersion.content, chunk)
  const contentMessage = processStreamingContent(message, appendableChunk)

  return {
    ...(contentMessage.isReasoningStreaming
      ? contentMessage
      : completeReasoningTiming(contentMessage)),
    status: MESSAGE_STATUS.STREAMING,
  }
}

/**
 * Finalize message after streaming completes.
 * Cleans content and consolidates reasoning from all sources.
 */
export function finalizeMessage(
  message: Message,
  apiReasoningContent?: string
): Message {
  const currentVersion = getCurrentVersion(message)
  const parsedThinkTags = currentVersion.content.includes('<think>')
    ? parseThinkTags(currentVersion.content)
    : undefined
  const visibleContent =
    parsedThinkTags?.visibleContent ?? currentVersion.content
  const finalReasoning =
    apiReasoningContent ||
    message.reasoning?.content ||
    parsedThinkTags?.reasoning ||
    ''

  const finalized = {
    ...updateCurrentVersionContent(message, visibleContent),
    reasoning: finalReasoning
      ? {
          ...startReasoningTiming(message),
          content: finalReasoning,
        }
      : undefined,
    isReasoningStreaming: false,
  }

  return completeReasoningTiming(finalized)
}

export function completeAssistantMessage(message: Message): Message {
  return completeAssistantTiming({
    ...finalizeMessage(message),
    status: MESSAGE_STATUS.COMPLETE,
  })
}

export function isAssistantMessageFinal(message: Message): boolean {
  return (
    message.status === MESSAGE_STATUS.COMPLETE ||
    message.status === MESSAGE_STATUS.ERROR
  )
}

export function isAssistantMessagePending(message: Message): boolean {
  return (
    message.status === MESSAGE_STATUS.LOADING ||
    message.status === MESSAGE_STATUS.STREAMING
  )
}

export function isPendingAssistantMessage(message?: Message): boolean {
  return Boolean(
    message?.from === MESSAGE_ROLES.ASSISTANT &&
    isAssistantMessagePending(message)
  )
}

type ChatCompletionChoice = ChatCompletionResponse['choices'][number]

export function hasChatCompletionChoice(
  response: ChatCompletionResponse
): boolean {
  return Boolean(response.choices?.[0])
}

export function applyChatCompletionChoice(
  message: Message,
  choice: ChatCompletionChoice
): Message {
  return completeAssistantTiming({
    ...finalizeMessage(
      updateCurrentVersionContent(message, choice.message?.content || ''),
      choice.message?.reasoning_content
    ),
    status: MESSAGE_STATUS.COMPLETE,
  })
}

export function applyChatCompletionResponse(
  message: Message,
  response: ChatCompletionResponse
): Message | null {
  const choice = response.choices?.[0]

  if (!choice) {
    return null
  }

  return applyChatCompletionChoice(message, choice)
}

/**
 * Sanitize messages loaded from storage.
 * Converts stuck loading/streaming messages to stable state.
 */
export function sanitizeMessagesOnLoad(messages: Message[]): Message[] {
  let targetIndex = -1

  for (let i = messages.length - 1; i >= 0; i--) {
    const message = messages[i]

    if (isPendingAssistantMessage(message)) {
      targetIndex = i
      break
    }
  }

  if (targetIndex === -1) return messages

  const finalized = finalizeMessage(messages[targetIndex])
  const hasContent = hasMessageContent(finalized)
  const hasReasoning = finalized.reasoning?.content?.trim()

  const sanitized: Message =
    hasContent || hasReasoning
      ? completeAssistantTiming({
          ...finalized,
          status: MESSAGE_STATUS.COMPLETE,
          isReasoningStreaming: false,
        })
      : completeAssistantTiming({
          ...updateCurrentVersionContent(
            finalized,
            `${t(ERROR_MESSAGES.API_REQUEST_ERROR)}: ${t(
              ERROR_MESSAGES.INTERRUPTED
            )}`
          ),
          status: MESSAGE_STATUS.ERROR,
          isReasoningStreaming: false,
        })

  const result = [...messages]
  result[targetIndex] = sanitized
  return result
}

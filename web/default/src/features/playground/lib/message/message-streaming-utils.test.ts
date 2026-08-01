import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { MESSAGE_ROLES, MESSAGE_STATUS } from '../../constants'
import type { Message } from '../../types'
import {
  applyStreamingChunk,
  completeAssistantMessage,
  createStreamingContentAccumulator,
  finalizeMessage,
} from './message-streaming-utils'
import { getMessageContentState } from './message-content-utils'

function createAssistantMessage(
  content = '',
  status: Message['status'] = MESSAGE_STATUS.STREAMING
): Message {
  return {
    key: 'test-message',
    from: MESSAGE_ROLES.ASSISTANT,
    versions: [{ id: 'test-version', content }],
    status,
  }
}

describe('streaming content accumulator', () => {
  test('handles tags split across chunks and keeps visible content incremental', () => {
    const accumulator = createStreamingContentAccumulator()

    assert.deepEqual(accumulator.append('content', 'before <thi'), {
      content: 'before ',
      reasoning: '',
    })
    assert.deepEqual(accumulator.append('content', 'nk>step'), {
      content: '',
      reasoning: 'step',
    })
    assert.deepEqual(accumulator.append('content', '</think>after'), {
      content: 'after',
      reasoning: '',
    })
  })

  test('supports multiple and unclosed think sections', () => {
    const accumulator = createStreamingContentAccumulator()

    assert.deepEqual(
      accumulator.append('content', '<think>one</think>A<think>two'),
      { content: 'A', reasoning: 'onetwo' }
    )
    assert.deepEqual(accumulator.append('content', '</think>B'), {
      content: 'B',
      reasoning: '',
    })
  })

  test('deduplicates cumulative chunks without dropping repeated delta text', () => {
    const accumulator = createStreamingContentAccumulator()

    assert.deepEqual(accumulator.append('content', 'one'), {
      content: 'one',
      reasoning: '',
    })
    assert.deepEqual(accumulator.append('content', 'one two'), {
      content: ' two',
      reasoning: '',
    })
    assert.deepEqual(accumulator.append('content', ' two'), {
      content: ' two',
      reasoning: '',
    })
  })

  test('keeps reasoning_content separate and deduplicated', () => {
    const accumulator = createStreamingContentAccumulator()

    assert.deepEqual(accumulator.append('reasoning', 'plan'), {
      content: '',
      reasoning: 'plan',
    })
    assert.deepEqual(accumulator.append('reasoning', 'plan more'), {
      content: '',
      reasoning: ' more',
    })
    assert.deepEqual(accumulator.append('content', 'answer'), {
      content: 'answer',
      reasoning: '',
    })
  })

  test('flushes a partial tag at stream finalization', () => {
    const accumulator = createStreamingContentAccumulator()

    assert.deepEqual(accumulator.append('content', 'answer <thi'), {
      content: 'answer ',
      reasoning: '',
    })
    assert.deepEqual(accumulator.finish(), {
      content: '<thi',
      reasoning: '',
    })
  })

  test('stop finalizes visible content and reasoning without raw think tags', () => {
    const accumulator = createStreamingContentAccumulator()
    let message = createAssistantMessage()

    const first = accumulator.append('content', 'answer <think>plan')
    message = applyStreamingChunk(message, 'content', first.content)
    message = applyStreamingChunk(message, 'reasoning', first.reasoning)

    const second = accumulator.append('content', '</think>')
    message = applyStreamingChunk(message, 'content', second.content)
    message = applyStreamingChunk(message, 'reasoning', second.reasoning)

    const tail = accumulator.finish()
    message = applyStreamingChunk(message, 'content', tail.content)
    message = applyStreamingChunk(message, 'reasoning', tail.reasoning)
    const stopped = finalizeMessage(message)
    const state = getMessageContentState(stopped, stopped.versions[0].content)

    assert.equal(state.displayContent, 'answer ')
    assert.equal(state.reasoningContent, 'plan')
    assert.ok(!state.displayContent.includes('<think>'))
    assert.ok(!state.reasoningContent?.includes('</think>'))
  })

  test('error does not preserve raw stream content after finalization', () => {
    const accumulator = createStreamingContentAccumulator()
    const initial = accumulator.append('content', 'answer <think>plan</think>')
    let message = createAssistantMessage()
    message = applyStreamingChunk(message, 'content', initial.content)
    message = applyStreamingChunk(message, 'reasoning', initial.reasoning)
    message = { ...message, status: MESSAGE_STATUS.ERROR }

    const finalized = finalizeMessage(message)

    assert.equal(finalized.versions[0].content, 'answer ')
    assert.equal(finalized.reasoning?.content, 'plan')
    assert.ok(!finalized.versions[0].content.includes('<think>'))
    assert.ok(!finalized.reasoning?.content.includes('</think>'))
  })

  test('finalize keeps an incomplete think tag as literal visible content', () => {
    const accumulator = createStreamingContentAccumulator()
    const chunk = accumulator.append('content', 'answer <thi')
    const tail = accumulator.finish()
    let message = createAssistantMessage()

    message = applyStreamingChunk(message, 'content', chunk.content)
    message = applyStreamingChunk(message, 'content', tail.content)
    const finalized = completeAssistantMessage(message)

    assert.equal(finalized.status, MESSAGE_STATUS.COMPLETE)
    assert.equal(finalized.versions[0].content, 'answer <thi')
    assert.equal(finalized.reasoning, undefined)
  })
})

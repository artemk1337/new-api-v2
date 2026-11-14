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
import { beforeEach, describe, expect, test } from 'bun:test'

import { STORAGE_KEYS } from '../../constants'
import type { Message } from '../../types'
import { loadMessages, saveMessages } from './storage'
import { MAX_STORED_MESSAGES_BYTES } from './storage-schema'

const values = new Map<string, string>()
const storage: Storage = {
  get length() {
    return values.size
  },
  clear() {
    values.clear()
  },
  getItem(key) {
    return values.get(key) ?? null
  },
  key(index) {
    return [...values.keys()][index] ?? null
  },
  removeItem(key) {
    values.delete(key)
  },
  setItem(key, value) {
    values.set(key, value)
  },
}
Object.defineProperty(globalThis, 'localStorage', {
  configurable: true,
  value: storage,
})

beforeEach(() => storage.clear())

function message(key: string, content: string, images?: string[]): Message {
  return {
    key,
    from: 'assistant',
    status: 'complete',
    versions: [{ id: key, content, images }],
  }
}

describe('playground message storage', () => {
  test('round-trips text and complete image URLs', () => {
    const messages = [
      message('text', 'Existing conversation'),
      message('images', 'Generated image', [
        'https://example.com/generated.png',
        'data:image/png;base64,aW1hZ2U=',
      ]),
    ]

    saveMessages(messages)

    expect(loadMessages()).toEqual(messages)
  })

  test('preserves text history when an inline image exceeds the storage budget', () => {
    const images = [
      `data:image/png;base64,${'A'.repeat(MAX_STORED_MESSAGES_BYTES)}`,
    ]
    const messages = [
      message('text', 'Existing conversation'),
      message('image', 'Generated image', images),
    ]

    saveMessages(messages)

    const stored = storage.getItem(STORAGE_KEYS.MESSAGES)
    expect(stored).not.toBeNull()
    expect(stored?.length).toBeLessThanOrEqual(MAX_STORED_MESSAGES_BYTES)
    const loaded = loadMessages()
    expect(loaded?.map((item) => item.versions[0].content)).toEqual([
      'Existing conversation',
      'Generated image',
    ])
    expect(loaded?.[1].versions[0].images).toEqual([])
    expect(loaded?.[1].versions[0].imagesOmitted).toBe(true)
    expect(messages[1].versions[0].images).toBe(images)
    expect(messages[1].versions[0].imagesOmitted).toBeUndefined()
  })

  test('drops older images first and retains complete recent images', () => {
    const image = `data:image/png;base64,${'A'.repeat(600_000)}`
    const messages = [
      message('old', 'Earlier image', [image]),
      message('new', 'Recent image', [image]),
    ]

    saveMessages(messages)

    const loaded = loadMessages()
    expect(loaded?.[0].versions[0].images).toEqual([])
    expect(loaded?.[0].versions[0].imagesOmitted).toBe(true)
    expect(loaded?.[1].versions[0].images).toEqual([image])
    expect(loaded?.[1].versions[0].imagesOmitted).toBeUndefined()
  })
})

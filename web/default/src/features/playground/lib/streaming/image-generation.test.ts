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
import { describe, expect, it } from 'bun:test'

import { DEFAULT_CONFIG } from '../../constants'
import {
  createLoadingAssistantMessage,
  createUserMessage,
} from '../message/message-utils'
import {
  buildImageGenerationPayload,
  getGeneratedImageUrls,
  isImageGenerationModel,
} from './image-generation'

describe('playground image generation', () => {
  it('routes GPT image variants and DALL-E through image generation', () => {
    for (const model of [
      'gpt-image-2',
      'gpt-image-2.5',
      'gpt-image-2.6-sunburst',
      'gpt-image-2.5-flare-c',
      'dall-e-3',
    ]) {
      expect(isImageGenerationModel(model)).toBe(true)
    }
    for (const model of ['gpt-4o', 'gemini-2.5-flash-image']) {
      expect(isImageGenerationModel(model)).toBe(false)
    }
  })

  it('uses the latest user prompt and selected group without chat parameters', () => {
    const payload = buildImageGenerationPayload(
      [
        createUserMessage('Earlier prompt'),
        createUserMessage('Draw a mountain'),
        createLoadingAssistantMessage(),
      ],
      { ...DEFAULT_CONFIG, model: 'gpt-image-2.6', group: '49' }
    )
    expect(payload).toEqual({
      model: 'gpt-image-2.6',
      group: '49',
      prompt: 'Draw a mountain',
      n: 1,
    })
  })

  it('accepts URL and base64 responses, including images larger than the markdown limit', () => {
    const base64 = 'A'.repeat(25000)
    expect(
      getGeneratedImageUrls({
        data: [{ url: 'https://example.com/image.png' }, { b64_json: base64 }],
      })
    ).toEqual([
      'https://example.com/image.png',
      `data:image/png;base64,${base64}`,
    ])
    expect(
      getGeneratedImageUrls({
        output_format: 'jpeg',
        data: [{ b64_json: 'abc' }],
      })
    ).toEqual(['data:image/jpeg;base64,abc'])
    expect(
      getGeneratedImageUrls({
        output_format: 'webp',
        data: [{ b64_json: 'abc' }],
      })
    ).toEqual(['data:image/webp;base64,abc'])
  })

  it('rejects empty results and unsafe image URLs', () => {
    expect(getGeneratedImageUrls({})).toEqual([])
    expect(
      getGeneratedImageUrls({
        data: [{}, { url: 'javascript:alert(1)' }, { b64_json: '' }],
      })
    ).toEqual([])
  })
})

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
import type {
  ImageGenerationRequest,
  ImageGenerationResponse,
  Message,
  PlaygroundConfig,
} from '../../types'
import { getMessageContent } from '../message/message-utils'

export function isImageGenerationModel(model: string): boolean {
  return /^(gpt-image(?:-|$)|dall-e(?:-|$))/i.test(model)
}

export function buildImageGenerationPayload(
  messages: Message[],
  config: PlaygroundConfig
): ImageGenerationRequest {
  const lastUserMessage = [...messages]
    .reverse()
    .find((message) => message.from === 'user')
  return {
    model: config.model,
    group: config.group,
    prompt: lastUserMessage ? getMessageContent(lastUserMessage) : '',
    n: 1,
  }
}

export function getGeneratedImageUrls(
  response: ImageGenerationResponse
): string[] {
  const mediaTypes: Record<string, string> = {
    jpeg: 'image/jpeg',
    webp: 'image/webp',
  }
  const mediaType = mediaTypes[response.output_format ?? ''] ?? 'image/png'
  return (response.data ?? []).flatMap((image) => {
    if (image.b64_json) return [`data:${mediaType};base64,${image.b64_json}`]
    if (image.url && /^https?:\/\//i.test(image.url)) return [image.url]
    return []
  })
}

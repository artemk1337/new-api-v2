/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { MODEL_TYPES, type ModelType } from '../constants'
import type { PricingModel } from '../types'

/** Classify the model's task, rather than its API protocol or input formats. */
export function getModelType(model: PricingModel): ModelType {
  const endpoints = model.supported_endpoint_types ?? []
  if (endpoints.includes('jina-rerank')) return MODEL_TYPES.RERANK
  if (endpoints.includes('embeddings')) return MODEL_TYPES.EMBEDDING
  if (endpoints.includes('openai-video')) return MODEL_TYPES.VIDEO
  if (endpoints.includes('image-generation')) return MODEL_TYPES.IMAGE

  if (model.capabilities?.includes('embeddings')) return MODEL_TYPES.EMBEDDING

  const output = model.output_modalities ?? []
  if (output.includes('video')) return MODEL_TYPES.VIDEO
  if (output.includes('image')) return MODEL_TYPES.IMAGE
  if (output.includes('audio')) return MODEL_TYPES.AUDIO

  // Older catalogs only provide names and generic API endpoints.
  const name = model.model_name.toLowerCase()
  if (/rerank/.test(name)) return MODEL_TYPES.RERANK
  if (/embed|(?:^|[/_.-])bge(?:[/_.-]|$)/.test(name)) {
    return MODEL_TYPES.EMBEDDING
  }
  if (/sora|veo|kling|pika|video|seedance|(?:^|[/_.-])wan[\d_.-]/.test(name)) {
    return MODEL_TYPES.VIDEO
  }
  if (
    /dall-e|gpt-image|flux[.-]|imagen|ideogram|stable-diffusion|sdxl|midjourney|recraft|seedream/.test(
      name
    )
  ) {
    return MODEL_TYPES.IMAGE
  }
  if (
    /whisper|tts|speech|voice|audio|realtime|(?:^|[/_.-])asr(?:[/_.-]|$)/.test(
      name
    )
  ) {
    return MODEL_TYPES.AUDIO
  }

  if (
    output.includes('text') ||
    endpoints.some((endpoint) =>
      ['openai', 'openai-response', 'anthropic', 'gemini'].includes(endpoint)
    )
  ) {
    return MODEL_TYPES.TEXT
  }
  return MODEL_TYPES.OTHER
}

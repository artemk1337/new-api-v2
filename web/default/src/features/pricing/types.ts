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
// ----------------------------------------------------------------------------
// Pricing Types
// ----------------------------------------------------------------------------

export type PricingVendor = {
  id: number
  name: string
  icon?: string
  description?: string
}

export type PricingGroupRef = {
  id: number
  name: string
}

export type PricingGroupRecord = PricingGroupRef & {
  ratio: number
  selectable: boolean
  description: string
}

/** Final retail USD rates. Token components are priced per million tokens. */
export type PricingRates = {
  input_usd?: number | null
  output_usd?: number | null
  cache_read_usd?: number | null
  cache_write_usd?: number | null
  cache_write_1h_usd?: number | null
  image_usd?: number | null
  image_output_usd?: number | null
  audio_input_usd?: number | null
  audio_output_usd?: number | null
  reasoning_output_usd?: number | null
  price_usd?: number | null
}

export type PricingTier = PricingRates & {
  label?: string
  min_input_tokens?: number
  max_input_tokens?: number | null
}

export type PricingGroupPrice = PricingRates & {
  group_id: number
  group: string
  group_name: string
  billing_mode: string
  price_unit: string
  /** Final retail USD per tool invocation, independent of token units. */
  tools?: Record<string, number | null>
  tiers?: PricingTier[]
  unavailable?: boolean
}

export type PricingModel = {
  id: number
  model_name: string
  description?: string
  icon?: string
  vendor_id?: number
  vendor_name?: string
  vendor_icon?: string
  vendor_description?: string
  group_prices?: PricingGroupPrice[]
  official_price?: {
    input_usd?: number | null
    output_usd?: number | null
    source: string
    updated_at: number
    price_unit: string
    comparison_enabled?: boolean
  }
  quota_type: number
  model_ratio: number
  completion_ratio: number
  model_price?: number
  cache_ratio?: number | null
  create_cache_ratio?: number | null
  image_ratio?: number | null
  audio_ratio?: number | null
  audio_completion_ratio?: number | null
  enable_groups: string[]
  enable_group_refs?: PricingGroupRef[]
  tags?: string
  supported_endpoint_types?: string[]
  key?: string
  group_ratio?: Record<string, number>
  /** Billing mode (e.g. "tiered_expr") used to flag dynamic pricing */
  billing_mode?: string
  /** Raw expression describing dynamic / tiered billing */
  billing_expr?: string
  /** Pricing version returned by backend, useful for cache busting */
  pricing_version?: string
  /**
   * Optional model metadata fields reserved for backend-provided catalog data.
   * Keep them data-driven; do not synthesize display values on the client.
   */
  context_length?: number
  max_output_tokens?: number
  knowledge_cutoff?: string
  release_date?: string
  parameter_count?: string
  input_modalities?: Modality[]
  output_modalities?: Modality[]
  capabilities?: ModelCapability[]
}

/** Input/output modalities supported by a model. */
export type Modality = 'text' | 'image' | 'audio' | 'video' | 'file'

/** Functional capabilities a model exposes. */
export type ModelCapability =
  | 'function_calling'
  | 'streaming'
  | 'vision'
  | 'json_mode'
  | 'structured_output'
  | 'reasoning'
  | 'tools'
  | 'system_prompt'
  | 'web_search'
  | 'code_interpreter'
  | 'caching'
  | 'embeddings'

export type PricingData = {
  success: boolean
  message?: string
  data: PricingModel[]
  vendors: PricingVendor[]
  group_ratio: Record<string, number>
  usable_group: Record<string, { desc: string; ratio: number }>
  pricing_groups?: PricingGroupRecord[]
  supported_endpoint: Record<string, string>
  auto_groups: string[]
}

export type TokenUnit = 'M' | 'K'
export type PriceType =
  | 'input'
  | 'output'
  | 'cache'
  | 'create_cache'
  | 'image'
  | 'audio_input'
  | 'audio_output'
export type QuotaType = 0 | 1 // 0: token-based, 1: per-request

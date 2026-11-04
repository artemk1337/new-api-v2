export interface Supplier {
  id: number
  code: string
  name: string
  adapter: string
  enabled: boolean
}

export interface SupplierPricingProfile {
  id: number
  supplier_id: number
  code: string
  name: string
  enabled: boolean
  base_url: string
  endpoint: string
  upstream_group: string
  usd_per_credit: number
  interval_seconds: number
  config_revision: number
  last_attempt_at: number
  last_success_at: number
  last_error?: string
}

export interface SupplierChoice {
  id: number
  name: string
  enabled: boolean
  profiles: { id: number; name: string; enabled: boolean }[]
}

export interface SupplierBinding {
  supplier_id: number | null
  profile_id: number | null
}

export interface GroupSupplierBinding extends SupplierBinding {
  group_id: number
}

export interface ChannelSupplierBinding extends SupplierBinding {
  channel_id: number
  group?: string
}

export interface SupplierModelTariff {
  model_name: string
  billing_mode: 'token' | 'fixed' | 'tiered_expr'
  expression?: string
  input_usd?: number | null
  output_usd?: number | null
  cache_read_usd?: number | null
  cache_write_usd?: number | null
  cache_write_1h_usd?: number | null
  image_usd?: number | null
  image_output_usd?: number | null
  audio_input_usd?: number | null
  audio_output_usd?: number | null
  reasoning_usd?: number | null
  price_usd?: number | null
  price_unit: string
  tools?: Record<string, number>
}

export interface SupplierTariffs {
  tariffs: SupplierModelTariff[]
  revision: number
  config_revision: number
}

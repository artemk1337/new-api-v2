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
// ============================================================================
// Wallet Type Definitions
// ============================================================================

/**
 * Generic API response
 */
export interface ApiResponse<T = unknown> {
  success?: boolean
  message?: string
  data?: T
}

/**
 * Standard API response types
 */
export type TopupInfoResponse = ApiResponse<TopupInfo>
export type RedemptionResponse = ApiResponse<number>
export type AmountResponse = ApiResponse<string>
export type PaymentResponse = ApiResponse<Record<string, unknown>> & {
  url?: string
}
export type TopupQuote = {
  currency: string
  rate_to_usd: number
  coefficient: number
  base_amount_usd: number
  charged_amount: number
  commission_usd: number
  cashback_percent: number
  cashback_amount_usd: number
  credited_amount_usd: number
  charged_amount_usd: number
}
export type TopupQuoteResponse = ApiResponse<TopupQuote>
export type StripePaymentResponse = ApiResponse<{ pay_link: string }>
export type AffiliateCodeResponse = ApiResponse<string>
export type AffiliateTransferResponse = ApiResponse
export type CreemPaymentResponse = ApiResponse<{ checkout_url: string }>
export type WaffoPaymentResponse = ApiResponse<
  { payment_url?: string } | string
>
export type WaffoPancakePaymentResponse = ApiResponse<
  | {
      checkout_url?: string
      session_id?: string
      expires_at?: number | string
      order_id?: string
      // Self-service session token + expiry — surfaced by the backend so
      // future flows (refund / cancel from new-api's own UI) can use them
      // without re-issuing checkout. Not consumed by the current handler.
      token?: string
      token_expires_at?: number | string
    }
  | string
>
export type YooKassaPaymentResponse = ApiResponse<{
  confirmation_url?: string
  payment_id?: string
  trade_no?: string
}>
export type NOWPaymentsPaymentResponse = ApiResponse<{
  payment_url?: string
  trade_no?: string
}>

export interface CashbackThreshold {
  min_amount: number
  cashback_percent: number
}

/**
 * Creem product configuration
 */
export interface CreemProduct {
  /** Product display name */
  name: string
  /** Creem product ID */
  productId: string
  /** Product price */
  price: number
  /** Quota amount to credit */
  quota: number
  /** Currency (USD or EUR) */
  currency: 'USD' | 'EUR'
}

/**
 * Creem payment request
 */
export interface CreemPaymentRequest {
  /** Creem product ID */
  product_id: string
  /** Payment method identifier */
  payment_method: 'creem'
}

/**
 * Payment method configuration
 */
export interface PaymentMethod {
  /** Display name of payment method */
  name: string
  /** Payment method type identifier */
  type: string
  /** Legacy optional color for UI display */
  color?: string
  /** Minimum topup amount for this payment method */
  min_topup?: number
  /** Optional react-icons component name or safe icon URL */
  icon?: string
  /** Top-up price multiplier configured for this payment method */
  topup_ratio?: number
  /** Public display config preloaded by the server with top-up info. */
  rate_to_usd?: number
  base_amount_multiplier?: number
  rounding_decimals?: number
  /** Currency configured for the provider (defaults to USD). */
  currency?: string
  /** Optional server-calculated quote amount in the provider currency. */
  payment_amount?: number
  /** Optional symbol supplied by the server for the provider currency. */
  currency_symbol?: string
}

/**
 * Waffo payment method configuration
 */
export interface WaffoPayMethod {
  /** Display name of payment method */
  name: string
  /** Optional icon path */
  icon?: string
  /** Waffo pay method type */
  payMethodType?: string
  /** Waffo pay method name */
  payMethodName?: string
  /** Public display config preloaded by the server with top-up info. */
  currency?: string
  rate_to_usd?: number
  base_amount_multiplier?: number
  topup_ratio?: number
  rounding_decimals?: number
  currency_symbol?: string
}

/**
 * Topup configuration information
 */
export interface TopupInfo {
  /** Whether online topup is enabled */
  enable_online_topup: boolean
  /** Whether Stripe topup is enabled */
  enable_stripe_topup: boolean
  /** Available payment methods */
  pay_methods: PaymentMethod[]
  /** Minimum topup amount for online topup */
  min_topup: number
  /** Minimum topup amount for Stripe */
  stripe_min_topup: number
  /** Preset amount options */
  amount_options: number[]
  /** Cashback thresholds sorted by min_amount */
  cashback: CashbackThreshold[]
  /** Optional topup link for purchasing codes */
  topup_link?: string
  /** Whether Creem topup is enabled */
  enable_creem_topup?: boolean
  /** Available Creem products */
  creem_products?: CreemProduct[]
  /** Whether Waffo topup is enabled */
  enable_waffo_topup?: boolean
  /** Available Waffo payment methods */
  waffo_pay_methods?: WaffoPayMethod[]
  /** Minimum topup amount for Waffo */
  waffo_min_topup?: number
  /** Whether Waffo Pancake topup is enabled */
  enable_waffo_pancake_topup?: boolean
  /** Minimum topup amount for Waffo Pancake */
  waffo_pancake_min_topup?: number
  /** Whether YooKassa topup is enabled */
  enable_yookassa_topup?: boolean
  /** Whether crypto topup through NOWPayments is enabled */
  enable_nowpayments_topup?: boolean
  /** Minimum topup amount for YooKassa */
  yookassa_min_topup?: number
  /** Whether redemption code usage is enabled */
  enable_redemption?: boolean
  /** Whether compliance confirmation has been completed */
  payment_compliance_confirmed?: boolean
  /** Current compliance terms version */
  payment_compliance_terms_version?: string
}

/**
 * Preset amount option with optional cashback percentage
 */
export interface PresetAmount {
  /** Preset amount value */
  value: number
  /** Optional cashback percentage (0-100) */
  cashback_percent?: number
}

/**
 * Redemption code request
 */
export interface RedemptionRequest {
  /** Redemption code key */
  key: string
}

/**
 * Payment request parameters
 */
export interface PaymentRequest {
  /** Topup amount */
  amount: number
  /** Payment method identifier */
  payment_method: string
}

/**
 * Waffo payment request parameters
 */
export interface WaffoPaymentRequest {
  /** Topup amount */
  amount: number
  /** Optional server-side Waffo payment method index */
  pay_method_index?: number
}

/**
 * Waffo Pancake payment request parameters
 */
export interface WaffoPancakePaymentRequest {
  /** Topup amount */
  amount: number
}

/**
 * YooKassa payment request parameters
 */
export interface YooKassaPaymentRequest {
  /** Topup amount */
  amount: number
  /** Payment method identifier */
  payment_method: 'yookassa_sbp'
}

export interface NOWPaymentsPaymentRequest {
  amount: number
  payment_method: 'nowpayments'
}

/**
 * YooKassa payment status sync request
 */
export interface YooKassaSyncRequest {
  /** Trade/order number */
  trade_no: string
}

/**
 * Amount calculation request
 */
export interface AmountRequest {
  /** Topup amount to calculate */
  amount: number
  /** Selected payment method, used to calculate its configured price */
  payment_method?: string
}

/**
 * Affiliate quota transfer request
 */
export interface AffiliateTransferRequest {
  /** Quota amount to transfer */
  quota: number
}

/**
 * User wallet data
 */
export interface UserWalletData {
  /** User ID */
  id: number
  /** Username */
  username: string
  /** Current quota balance */
  quota: number
  /** Total used quota */
  used_quota: number
  /** Total request count */
  request_count: number
  /** Affiliate quota (pending rewards) */
  aff_quota: number
  /** Total affiliate quota earned (historical) */
  aff_history_quota: number
  /** Number of successful affiliate invites */
  aff_count: number
  /** User group */
  group: string
}

/**
 * Topup record status
 */
export type TopupStatus = 'success' | 'pending' | 'failed' | 'expired'

/**
 * Topup billing record
 */
export interface TopupRecord {
  /** Record ID */
  id: number
  /** User ID */
  user_id: number
  /** Topup amount (quota) */
  amount: number
  /** Original top-up amount requested by the user */
  requested_amount?: number
  /** Payment amount (actual money paid) */
  money: number
  /** Trade/order number */
  trade_no: string
  /** Payment method type */
  payment_method: string
  /** Provider identifier used only for legacy history amount fallbacks. */
  payment_provider?: string
  /** Public display name of the configured payment method */
  payment_method_name?: string
  /** Optional source category returned by newer API versions */
  source?: string
  /** Creation timestamp */
  create_time: number
  /** Completion timestamp */
  complete_time?: number
  /** Payment status */
  status: TopupStatus
  /** Immutable wallet/accounting amount in USD for history display */
  accounting_amount_usd?: number
  /** Currency captured for provider-currency history entries (for example EUR). */
  payment_currency?: string
  /** Immutable amount charged by the payment provider, when available. */
  payment_charged_amount?: number
  /** Immutable wallet amount in USD captured at checkout, when available. */
  payment_base_amount?: number
}

/**
 * Billing history response
 */
export interface BillingHistoryResponse {
  items: TopupRecord[]
  total: number
}

/**
 * Complete order request (admin only)
 */
export interface CompleteOrderRequest {
  trade_no: string
}

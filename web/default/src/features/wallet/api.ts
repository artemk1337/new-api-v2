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
import { api } from '@/lib/api'

import type {
  RedemptionRequest,
  PaymentRequest,
  AmountRequest,
  AffiliateTransferRequest,
  ApiResponse,
  TopupInfoResponse,
  RedemptionResponse,
  AmountResponse,
  PaymentResponse,
  StripePaymentResponse,
  AffiliateCodeResponse,
  AffiliateTransferResponse,
  BillingHistoryResponse,
  CompleteOrderRequest,
  CreemPaymentRequest,
  CreemPaymentResponse,
  WaffoPaymentRequest,
  WaffoPaymentResponse,
  WaffoPancakePaymentRequest,
  WaffoPancakePaymentResponse,
  YooKassaPaymentRequest,
  YooKassaPaymentResponse,
  YooKassaSyncRequest,
  NOWPaymentsPaymentRequest,
  NOWPaymentsPaymentResponse,
  TopupQuoteResponse,
  DirectUSDTPaymentResponse,
  DirectUSDTPaymentStatus,
  DirectUSDTNetwork,
} from './types'

// ============================================================================
// Wallet API Functions
// ============================================================================

/**
 * Check if API response is successful
 */
export function isApiSuccess(response: ApiResponse): boolean {
  return response.success === true || response.message === 'success'
}

/**
 * Get topup configuration info
 */
export async function getTopupInfo(): Promise<TopupInfoResponse> {
  const res = await api.get('/api/user/topup/info')
  return res.data
}

export type AvailableCurrency = {
  code: string
  symbol: string
  name: string
  enabled: boolean
}

export async function getAvailableCurrencies(): Promise<AvailableCurrency[]> {
  const res = await api.get<{ success?: boolean; data?: AvailableCurrency[] }>(
    '/api/currencies',
    { skipBusinessError: true, skipErrorHandler: true } as Record<
      string,
      unknown
    >
  )
  return Array.isArray(res.data.data) ? res.data.data : []
}

export async function getTopupQuote(
  request: PaymentRequest
): Promise<TopupQuoteResponse> {
  const res = await api.post<TopupQuoteResponse>(
    '/api/user/topup/quote',
    request,
    {
      skipBusinessError: true,
      skipErrorHandler: true,
    } as Record<string, unknown>
  )
  return res.data
}

/**
 * Redeem a topup code
 */
export async function redeemTopupCode(
  request: RedemptionRequest
): Promise<RedemptionResponse> {
  const res = await api.post('/api/user/topup', request)
  return res.data
}

/**
 * Calculate payment amount for regular payment
 */
export async function calculateAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Stripe payment
 */
export async function calculateStripeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/stripe/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request regular payment
 */
export async function requestPayment(
  request: PaymentRequest
): Promise<PaymentResponse> {
  const res = await api.post('/api/user/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return {
    ...res.data,
    url: res.data.url || (res as unknown as { url?: string }).url,
  }
}

/**
 * Request Stripe payment
 */
export async function requestStripePayment(
  request: PaymentRequest
): Promise<StripePaymentResponse> {
  const res = await api.post('/api/user/stripe/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Creem payment
 */
export async function requestCreemPayment(
  request: CreemPaymentRequest
): Promise<CreemPaymentResponse> {
  const res = await api.post('/api/user/creem/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Waffo payment
 */
export async function requestWaffoPayment(
  request: WaffoPaymentRequest
): Promise<WaffoPaymentResponse> {
  const res = await api.post('/api/user/waffo/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for Waffo Pancake payment
 */
export async function calculateWaffoPancakeAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/waffo-pancake/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request Waffo Pancake payment
 */
export async function requestWaffoPancakePayment(
  request: WaffoPancakePaymentRequest
): Promise<WaffoPancakePaymentResponse> {
  const res = await api.post('/api/user/waffo-pancake/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Calculate payment amount for YooKassa payment
 */
export async function calculateYooKassaAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/yookassa/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Request YooKassa payment
 */
export async function requestYooKassaPayment(
  request: YooKassaPaymentRequest
): Promise<YooKassaPaymentResponse> {
  const res = await api.post('/api/user/yookassa/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

/**
 * Sync a YooKassa payment after returning from the payment page
 */
export async function syncYooKassaPayment(
  request: YooKassaSyncRequest
): Promise<ApiResponse> {
  const res = await api.post('/api/user/yookassa/sync', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function calculateNOWPaymentsAmount(
  request: AmountRequest
): Promise<AmountResponse> {
  const res = await api.post('/api/user/nowpayments/amount', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function requestNOWPaymentsPayment(
  request: NOWPaymentsPaymentRequest
): Promise<NOWPaymentsPaymentResponse> {
  const res = await api.post('/api/user/nowpayments/pay', request, {
    skipBusinessError: true,
  } as Record<string, unknown>)
  return res.data
}

export async function requestUSDTTrc20Payment(
  request: PaymentRequest
): Promise<DirectUSDTPaymentResponse> {
  const res = await api.post<DirectUSDTPaymentResponse>(
    '/api/user/usdt-trc20/pay',
    request,
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

const directCryptoNetworkPath: Record<DirectUSDTNetwork, string> = {
  TRON: 'tron',
  TON: 'ton',
  SOLANA: 'solana',
}

export function getDirectCryptoPaymentEndpoint(network: DirectUSDTNetwork) {
  return `/api/user/crypto/${directCryptoNetworkPath[network]}/pay`
}

export async function requestDirectCryptoPayment(
  network: DirectUSDTNetwork,
  request: PaymentRequest
): Promise<DirectUSDTPaymentResponse> {
  const res = await api.post<DirectUSDTPaymentResponse>(
    getDirectCryptoPaymentEndpoint(network),
    { ...request, payment_method: 'crypto_direct' },
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

export async function getDirectCryptoPaymentStatus(
  network: DirectUSDTNetwork,
  tradeNo: string
): Promise<ApiResponse<DirectUSDTPaymentStatus>> {
  const res = await api.get<ApiResponse<DirectUSDTPaymentStatus>>(
    `/api/user/crypto/${directCryptoNetworkPath[network]}/${encodeURIComponent(tradeNo)}`,
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

export async function getUSDTTrc20PaymentStatus(
  tradeNo: string
): Promise<ApiResponse<DirectUSDTPaymentStatus>> {
  const res = await api.get<ApiResponse<DirectUSDTPaymentStatus>>(
    `/api/user/usdt-trc20/${encodeURIComponent(tradeNo)}`,
    { skipBusinessError: true } as Record<string, unknown>
  )
  return res.data
}

/**
 * Get affiliate code
 */
export async function getAffiliateCode(): Promise<AffiliateCodeResponse> {
  const res = await api.get('/api/user/aff')
  return res.data
}

/**
 * Transfer affiliate quota to balance
 */
export async function transferAffiliateQuota(
  request: AffiliateTransferRequest
): Promise<AffiliateTransferResponse> {
  const res = await api.post('/api/user/aff_transfer', request)
  return res.data
}

/**
 * Get billing history for current user
 */
export async function getUserBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup/self?${params.toString()}`)
  return res.data
}

/**
 * Get billing history for all users (admin only)
 */
export async function getAllBillingHistory(
  page: number,
  pageSize: number,
  keyword?: string
): Promise<ApiResponse<BillingHistoryResponse>> {
  const params = new URLSearchParams({
    p: page.toString(),
    page_size: pageSize.toString(),
  })
  if (keyword) {
    params.append('keyword', keyword)
  }
  const res = await api.get(`/api/user/topup?${params.toString()}`)
  return res.data
}

/**
 * Complete a pending order (admin only)
 */
export async function completeOrder(
  request: CompleteOrderRequest
): Promise<ApiResponse> {
  const res = await api.post('/api/user/topup/complete', request)
  return res.data
}

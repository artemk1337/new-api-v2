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
import type { StatusBadgeProps } from '@/components/status-badge'
import { formatTimestampToDate } from '@/lib/format'
import { formatCurrencyFromUSD } from '@/lib/currency'

import type { TopupStatus } from '../types'
import { formatCurrency, getPaymentCurrencyLabel } from './format'

// ============================================================================
// Billing Utility Functions
// ============================================================================

interface StatusConfig {
  variant: StatusBadgeProps['variant']
  label: string
}

/**
 * Status badge configuration
 */
export const STATUS_CONFIG: Record<TopupStatus, StatusConfig> = {
  success: {
    variant: 'success',
    label: 'Success',
  },
  pending: {
    variant: 'warning',
    label: 'Pending',
  },
  failed: {
    variant: 'danger',
    label: 'Failed',
  },
  expired: {
    variant: 'danger',
    label: 'Expired',
  },
}

/**
 * Get status badge configuration
 */
export function getStatusConfig(status: TopupStatus): StatusConfig {
  return STATUS_CONFIG[status] || STATUS_CONFIG.pending
}

export function getTopupStatusLabel(
  status: TopupStatus,
  t: (key: string) => string
): string {
  return t(getStatusConfig(status).label)
}

/**
 * Payment method display names
 */
export const PAYMENT_METHOD_NAMES: Record<string, string> = {
  stripe: 'Stripe',
  creem: 'Creem',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  waffo: 'Waffo (Global Payment)',
  waffo_pancake: 'Waffo Pancake',
  nowpayments: 'Crypto / NOWPayments',
  balance: 'Balance top-up',
}

const TOPUP_SOURCE_NAMES: Record<string, string> = {
  stripe: 'Stripe',
  creem: 'Creem',
  alipay: 'Alipay',
  wxpay: 'WeChat Pay',
  waffo: 'Waffo (Global Payment)',
  waffo_pancake: 'Waffo Pancake',
  yookassa_sbp: 'СБП',
  nowpayments: 'Crypto / NOWPayments',
  balance: 'Balance top-up',
  promo: 'Top-up by promo code',
  promo_code: 'Top-up by promo code',
  redemption: 'Top-up by promo code',
  redemption_code: 'Top-up by promo code',
  referral: 'Referral top-up income',
  referral_income: 'Referral top-up income',
  referral_reward: 'Referral top-up income',
  affiliate: 'Referral top-up income',
}

/**
 * Get payment method display name
 */
export function getPaymentMethodName(
  method: string,
  t?: (key: string) => string
): string {
  const name = PAYMENT_METHOD_NAMES[method.trim().toLowerCase()] || method
  return t ? t(name) : name
}

export function getTopupSourceLabel(
  source: string | undefined,
  paymentMethod: string,
  t?: (key: string) => string,
  paymentMethodName?: string
): string {
  const sourceKey = source?.trim().toLowerCase()
  const sourceName = sourceKey ? TOPUP_SOURCE_NAMES[sourceKey] : undefined
  if (sourceName) {
    const translatable =
      sourceName === 'Top-up by promo code' ||
      sourceName === 'Referral top-up income'
    return translatable && t ? t(sourceName) : sourceName
  }
  if (paymentMethodName?.trim()) {
    return paymentMethodName.trim()
  }
  const paymentMethodKey = paymentMethod.trim().toLowerCase()
  const paymentName = TOPUP_SOURCE_NAMES[paymentMethodKey] || 'Payment'
  return paymentName === 'Payment' && t ? t(paymentName) : paymentName
}

/**
 * Select a positive finite display amount. New history responses provide an
 * immutable USD accounting amount; older responses use legacy fallbacks.
 */
export function getTopupAmountToDisplay(
  amount: number,
  requestedAmount?: number,
  money?: number,
  accountingAmountUSD?: number
): number {
  // New history responses explicitly mark the immutable USD accounting
  // amount. A zero/invalid marker is intentional: do not fall back to a
  // provider-currency amount and render it as USD.
  if (typeof accountingAmountUSD === 'number') {
    return Number.isFinite(accountingAmountUSD) && accountingAmountUSD > 0
      ? accountingAmountUSD
      : 0
  }
  if (
    typeof requestedAmount === 'number' &&
    Number.isFinite(requestedAmount) &&
    requestedAmount > 0
  ) {
    return requestedAmount
  }
  if (typeof amount === 'number' && Number.isFinite(amount) && amount > 0) {
    return amount
  }
  return typeof money === 'number' && Number.isFinite(money) && money > 0
    ? money
    : 0
}

export interface TopupDisplayAmount {
  amount: number
  currency: string
}

function getProviderHistoryCurrency(
  paymentCurrency: string | undefined,
  paymentProvider: string | undefined
): string {
  const currency = paymentCurrency?.trim().toUpperCase() || 'USD'
  if (currency !== 'USD') return currency

  const provider = paymentProvider?.trim().toLowerCase()
  if (provider === 'nowpayments') return 'USDT'
  if (provider === 'yookassa') return 'RUB'
  return currency
}

/**
 * Select the immutable amount and currency for a history row. Ordinary wallet
 * payments have a USD accounting snapshot even when the provider is paid in
 * another currency. Subscription rows without that snapshot retain their
 * provider amount and captured currency (for example €10), rather than being
 * mislabeled as dollars.
 */
export function getTopupDisplayAmount(input: {
  amount: number
  requestedAmount?: number
  money?: number
  accountingAmountUSD?: number
  paymentBaseAmount?: number
  paymentCurrency?: string
  paymentProvider?: string
}): TopupDisplayAmount {
  const currency = input.paymentCurrency?.trim().toUpperCase() || 'USD'
  const provider = input.paymentProvider?.trim().toLowerCase()
  const accounting =
    typeof input.accountingAmountUSD === 'number' &&
    Number.isFinite(input.accountingAmountUSD) &&
    input.accountingAmountUSD > 0
      ? input.accountingAmountUSD
      : 0
  const paymentBase =
    typeof input.paymentBaseAmount === 'number' &&
    Number.isFinite(input.paymentBaseAmount) &&
    input.paymentBaseAmount > 0
      ? input.paymentBaseAmount
      : 0

  if (accounting > 0 || paymentBase > 0) {
    return { amount: accounting || paymentBase, currency: 'USD' }
  }

  // Some migrated rows have the schema default `payment_currency=USD` even
  // though their provider settles in USDT/RUB. Keep those provider contracts
  // explicit and use the provider amount instead of token-valued requested_amount.
  const providerCurrency = getProviderHistoryCurrency(
    input.paymentCurrency,
    input.paymentProvider
  )
  const providerHasNonUSDHistory =
    currency !== 'USD' ||
    provider === 'nowpayments' ||
    provider === 'yookassa' ||
    provider === 'waffo' ||
    provider === 'creem'
  if (providerHasNonUSDHistory) {
    // Legacy token-mode rows keep requested_amount in wallet display units,
    // while money is the amount actually charged by the provider. Prefer the
    // captured provider amount whenever no immutable USD accounting snapshot
    // exists, otherwise a token count could be rendered as USDT/RUB/EUR.
    const providerAmount = [input.money, input.requestedAmount].find(
      (value) =>
        typeof value === 'number' && Number.isFinite(value) && value > 0
    )
    if (providerAmount !== undefined) {
      return { amount: providerAmount, currency: providerCurrency }
    }
  }

  return {
    amount: getTopupAmountToDisplay(
      input.amount,
      input.requestedAmount,
      input.money,
      input.accountingAmountUSD
    ),
    currency: 'USD',
  }
}

/**
 * Select the actual provider charge for history details. Unlike the wallet
 * accounting amount, this value must retain the provider's payment currency.
 */
export function getTopupPaymentDisplayAmount(input: {
  money?: number
  paymentChargedAmount?: number
  paymentCurrency?: string
  paymentProvider?: string
  source?: string
}): TopupDisplayAmount | null {
  const amount = [input.paymentChargedAmount, input.money].find(
    (value) => typeof value === 'number' && Number.isFinite(value) && value > 0
  )
  const source = input.source?.trim().toLowerCase()
  if (
    amount === undefined &&
    (source === 'promo' ||
      source === 'promo_code' ||
      source === 'redemption' ||
      source === 'redemption_code' ||
      source === 'referral' ||
      source === 'referral_income' ||
      source === 'referral_reward' ||
      source === 'affiliate')
  ) {
    return null
  }
  return {
    amount: amount ?? 0,
    currency: getProviderHistoryCurrency(
      input.paymentCurrency,
      input.paymentProvider
    ),
  }
}

export function formatTopupDisplayAmount(display: TopupDisplayAmount): string {
  if (display.currency === 'USD') {
    return formatCurrencyFromUSD(display.amount, {
      digitsLarge: 2,
      digitsSmall: 2,
      abbreviate: false,
    })
  }
  return `${getPaymentCurrencyLabel(display.currency)}${formatCurrency(display.amount, 2)}`
}

/** Format an already-settled provider charge without converting it to wallet currency. */
export function formatTopupPaymentDisplayAmount(
  display: TopupDisplayAmount | null
): string {
  if (!display) return '—'
  return `${getPaymentCurrencyLabel(display.currency)}${formatCurrency(display.amount, 2)}`
}

/**
 * Format timestamp to readable date string
 */
export function formatTimestamp(timestamp: number): string {
  return formatTimestampToDate(timestamp)
}

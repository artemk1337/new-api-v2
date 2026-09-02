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
import { getCurrencyDisplay } from '@/lib/currency'

import {
  PAYMENT_TYPES,
  DEFAULT_PRESET_MULTIPLIERS,
  DEFAULT_PAYMENT_TYPE,
  DEFAULT_MIN_TOPUP,
} from '../constants'
import type {
  CashbackThreshold,
  PaymentMethod,
  PresetAmount,
  TopupInfo,
} from '../types'

// ============================================================================
// Payment Processing Functions
// ============================================================================

/**
 * Accept only absolute HTTP(S) URLs returned by payment providers.
 *
 * Provider responses are untrusted input: non-web schemes such as
 * javascript: and data: must never be passed to a browser navigation API.
 */
export function isSafeHttpRedirectUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed) {
    return false
  }

  try {
    const url = new URL(trimmed)
    return url.protocol === 'http:' || url.protocol === 'https:'
  } catch {
    return false
  }
}

/**
 * Navigate to a verified provider checkout in the current tab. Keeping this
 * synchronous navigation primitive separate makes it safe to call after an
 * async payment request without relying on a popup user gesture.
 */
export function redirectToPaymentPage(
  value: string,
  navigate: (url: string) => void = (url) => {
    window.location.href = url
  }
): boolean {
  if (!isSafeHttpRedirectUrl(value)) {
    return false
  }
  navigate(value.trim())
  return true
}

/**
 * Submit payment form in the current tab. The provider response arrives after
 * an async request, so using a new tab can be blocked as a popup.
 */
export function submitPaymentForm(
  url: string,
  params: Record<string, unknown>
): void {
  const form = document.createElement('form')
  form.action = url
  form.method = 'POST'

  // Add form parameters
  Object.entries(params).forEach(([key, value]) => {
    const input = document.createElement('input')
    input.type = 'hidden'
    input.name = key
    input.value = String(value)
    form.appendChild(input)
  })

  document.body.appendChild(form)
  form.submit()
  document.body.removeChild(form)
}

/**
 * Check if payment method is Stripe
 */
export function isStripePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.STRIPE
}

/**
 * Check if payment method is Waffo Pancake
 *
 * Pancake is a metered-style payment that goes through a dedicated checkout
 * URL flow rather than the generic epay form submission, so it must be
 * special-cased in payment dispatch logic.
 */
export function isWaffoPancakePayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO_PANCAKE
}

/**
 * Check if the payment type must use Waffo's dedicated checkout endpoint.
 * Waffo may be present in the generic method list for display purposes, but
 * it cannot be submitted through the legacy EPay form endpoint.
 */
export function isWaffoPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.WAFFO
}

export type PaymentCheckoutKind = 'waffo' | 'waffo-pancake' | 'generic'

// Select the browser payment flow from the configured method type. This must
// stay independent of which Waffo sub-method the user selected: an omitted
// sub-method is a valid Waffo auto-selection, not an EPay fallback.
export function getPaymentCheckoutKind(
  paymentType: string
): PaymentCheckoutKind {
  if (isWaffoPayment(paymentType)) {
    return 'waffo'
  }
  if (isWaffoPancakePayment(paymentType)) {
    return 'waffo-pancake'
  }
  return 'generic'
}

type PaymentErrorResponse = {
  message?: unknown
  data?: unknown
}

// Payment providers may return a useful user-facing error in `data`, while
// legacy endpoints use the non-actionable literal `error` as `message`.
// Keep provider payloads out of the UI unless they are short, plain text and
// do not look like a credential-bearing diagnostic.
export function getPaymentErrorMessage(
  response: PaymentErrorResponse,
  fallback: string
): string {
  const isSafeMessage = (value: unknown): value is string => {
    if (typeof value !== 'string') return false
    const text = value.trim()
    const hasControlCharacter = [...text].some(
      (character) => character.charCodeAt(0) < 0x20
    )
    if (
      !text ||
      text.length > 280 ||
      /[<>]/.test(text) ||
      hasControlCharacter
    ) {
      return false
    }
    return !/\b(api[ _-]?key|secret|token|password|authorization|bearer|signature|private key)\b/i.test(
      text
    )
  }

  if (isSafeMessage(response.data)) {
    return response.data.trim()
  }
  if (
    isSafeMessage(response.message) &&
    !['error', 'success'].includes(response.message.trim().toLowerCase())
  ) {
    return response.message.trim()
  }
  return fallback
}

/**
 * Check if payment method is YooKassa
 */
export function isYooKassaPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.YOOKASSA_SBP
}

export function isNOWPaymentsPayment(paymentType: string): boolean {
  return paymentType === PAYMENT_TYPES.NOWPAYMENTS
}

export function isUSDTTrc20Payment(paymentType: string): boolean {
  return paymentType === 'usdt_trc20_direct'
}

export type DirectUSDTNetwork = 'TRON' | 'TON' | 'SOLANA'

const directUSDTTypeNetwork: Record<string, DirectUSDTNetwork> = {
  [PAYMENT_TYPES.USDT_TRON]: 'TRON',
  [PAYMENT_TYPES.USDT_TON]: 'TON',
  [PAYMENT_TYPES.USDT_SOLANA]: 'SOLANA',
}

export function isDirectUSDTPayment(paymentType: string): boolean {
  if (paymentType.trim().toLowerCase() === PAYMENT_TYPES.CRYPTO_DIRECT) {
    return true
  }
  return Object.hasOwn(directUSDTTypeNetwork, paymentType.trim().toLowerCase())
}

export function getDirectUSDTNetwork(
  paymentType: string
): DirectUSDTNetwork | null {
  return directUSDTTypeNetwork[paymentType.trim().toLowerCase()] ?? null
}

export function isUSDTTrc20TerminalStatus(status: string): boolean {
  return (
    status === 'paid' ||
    status === 'success' ||
    status === 'expired' ||
    status === 'failed'
  )
}

export function getUSDTTrc20DisplayStatus(
  status: string | undefined,
  expiredByTime: boolean
): string | undefined {
  if (expiredByTime) return 'expired'
  if (status === 'paid') return 'success'
  return status
}

export function isSafeInternalUSDTTrc20Url(value: string): boolean {
  const trimmed = value.trim()
  // Payment links are generated by our API and must stay relative to this
  // application. In particular, reject protocol-relative URLs (`//host`) and
  // query/hash payloads that could alter the destination or expose a redirect.
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//')) {
    return false
  }
  let parsed: URL
  try {
    parsed = new URL(trimmed, 'https://internal.invalid')
  } catch {
    return false
  }
  return (
    !parsed.search &&
    !parsed.hash &&
    /^\/usdt-trc20\/[^/?#%]+$/.test(parsed.pathname)
  )
}

/** Validate only our allowlisted multichain checkout paths. */
export function isSafeInternalDirectUSDTUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//')) {
    return false
  }
  let parsed: URL
  try {
    parsed = new URL(trimmed, 'https://internal.invalid')
  } catch {
    return false
  }
  return (
    !parsed.search &&
    !parsed.hash &&
    (/^\/crypto\/(tron|ton|solana)\/[^/?#%]+$/.test(parsed.pathname) ||
      /^\/usdt-trc20\/[^/?#%]+$/.test(parsed.pathname))
  )
}

/**
 * Return a payment method minimum in the wallet amount unit expected by the
 * API. Provider minimums are stored in settlement currency; convert them with
 * the same quote inputs the server exposes to the wallet.
 */
export function getPaymentMethodMinimumAmount(
  method: PaymentMethod | undefined
): number | null {
  if (!method) return null
  const minimum = Number(method.min_topup)
  if (!Number.isFinite(minimum) || minimum <= 0) return null
  const { config } = getCurrencyDisplay()
  if (isDirectUSDTPayment(method.type)) {
    if (config.quotaDisplayType === 'TOKENS') {
      return minimum * config.quotaPerUnit
    }
    return minimum
  }
  const rateToUSD = Number(method.rate_to_usd)
  const multiplier = Number(method.base_amount_multiplier)
  const coefficient = Math.max(1, Number(method.topup_ratio))
  if (
    !Number.isFinite(rateToUSD) ||
    rateToUSD <= 0 ||
    !Number.isFinite(multiplier) ||
    multiplier <= 0 ||
    !Number.isFinite(coefficient) ||
    coefficient <= 0
  ) {
    return null
  }
  return minimum / (rateToUSD * multiplier * coefficient)
}

/** Minimum amount across the currently visible methods, in wallet units. */
export function getMinimumAvailablePaymentMethodAmount(
  topupInfo: TopupInfo | null
): number {
  if (!topupInfo) return DEFAULT_MIN_TOPUP
  const methods = topupInfo.pay_methods ?? []
  if (methods.length === 0) return getMinTopupAmount(topupInfo)
  const minimums = methods
    .map((method) => getPaymentMethodMinimumAmount(method))
    .filter((minimum): minimum is number => minimum !== null)
  if (minimums.length > 0) return Math.min(...minimums)
  return getMinTopupAmount(topupInfo)
}

export function isPaymentMethodAmountEligible(
  amount: number,
  method: PaymentMethod | undefined
): boolean {
  if (!Number.isFinite(amount) || amount <= 0) return false
  const minimum = getPaymentMethodMinimumAmount(method)
  const configuredMinimum = Number(method?.min_topup)
  if (configuredMinimum > 0 && minimum === null) return false
  return minimum === null || amount >= minimum
}

export function getEffectiveCashbackTiers(
  regularTiers: CashbackThreshold[],
  isReferralCashback: boolean | undefined
): CashbackThreshold[] {
  const normalized = normalizeCashbackTiers(regularTiers)
  if (!isReferralCashback) return normalized

  return normalized.map((tier) => ({
    ...tier,
    cashback_percent:
      tier.referral_cashback_percent ?? tier.cashback_percent,
  }))
}

/**
 * Get default payment type from topup info
 */
export function getDefaultPaymentType(topupInfo: TopupInfo | null): string {
  if (!topupInfo) {
    return DEFAULT_PAYMENT_TYPE
  }

  // Return first available payment method or default
  if (topupInfo.pay_methods?.length > 0) {
    return topupInfo.pay_methods[0].type
  }

  if (topupInfo.enable_stripe_topup) {
    return PAYMENT_TYPES.STRIPE
  }

  if (topupInfo.enable_waffo_topup) {
    return PAYMENT_TYPES.WAFFO
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return PAYMENT_TYPES.WAFFO_PANCAKE
  }

  if (topupInfo.enable_yookassa_topup) {
    return PAYMENT_TYPES.YOOKASSA_SBP
  }

  if (topupInfo.enable_nowpayments_topup) {
    return PAYMENT_TYPES.NOWPAYMENTS
  }

  return DEFAULT_PAYMENT_TYPE
}

/**
 * Get minimum topup amount from topup info
 */
export function getMinTopupAmount(
  topupInfo: TopupInfo | null,
  selectedMethod?: PaymentMethod
): number {
  if (!topupInfo) {
    return DEFAULT_MIN_TOPUP
  }

  const selectedMinimum = getPaymentMethodMinimumAmount(selectedMethod)
  if (selectedMinimum !== null) {
    return selectedMinimum
  }

  const availableMinimum =
    getMinimumAvailablePaymentMethodAmountWithoutRecursion(topupInfo)
  if (availableMinimum !== null) return availableMinimum

  // When direct USDT is the only visible method, its server-provided minimum
  // is also the form minimum. Keep mixed method lists on their existing shared
  // minimum so lower-cost gateways remain usable.
  const methods = topupInfo.pay_methods ?? []
  if (
    methods.length > 0 &&
    methods.every((method) => isDirectUSDTPayment(method.type))
  ) {
    return getPaymentMethodMinimumAmount(methods[0]) ?? DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_online_topup) {
    return topupInfo.min_topup
  }

  if (topupInfo.enable_stripe_topup) {
    return topupInfo.stripe_min_topup
  }

  if (topupInfo.enable_waffo_topup) {
    return topupInfo.waffo_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_waffo_pancake_topup) {
    return topupInfo.waffo_pancake_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_yookassa_topup) {
    return topupInfo.yookassa_min_topup || DEFAULT_MIN_TOPUP
  }

  if (topupInfo.enable_nowpayments_topup) {
    return DEFAULT_MIN_TOPUP
  }

  return DEFAULT_MIN_TOPUP
}

function getMinimumAvailablePaymentMethodAmountWithoutRecursion(
  topupInfo: TopupInfo
): number | null {
  const minimums = (topupInfo.pay_methods ?? [])
    .map((method) => getPaymentMethodMinimumAmount(method))
    .filter((minimum): minimum is number => minimum !== null)
  return minimums.length > 0 ? Math.min(...minimums) : null
}

/**
 * Generate preset amounts based on minimum topup
 */
export function generatePresetAmounts(minAmount: number): PresetAmount[] {
  return DEFAULT_PRESET_MULTIPLIERS.map((multiplier) => ({
    value: minAmount * multiplier,
  }))
}

/**
 * Merge custom preset amounts with cashback thresholds
 */
export function mergePresetAmounts(
  amountOptions: number[],
  cashback: CashbackThreshold[] = []
): PresetAmount[] {
  if (!amountOptions || amountOptions.length === 0) {
    return []
  }

  return amountOptions.map((amount) => ({
    value: amount,
    cashback_percent: getCashbackPercentForAmount(amount, cashback),
  }))
}

/**
 * Keep the client-side cashback model aligned with the server contract.
 * Legacy settings are untrusted input: thresholds must be non-negative and
 * percentages must stay within the meaningful 0..100 range. Zero-percent
 * tiers are intentional and must remain so they can disable cashback at a
 * higher threshold.
 */
export function normalizeCashbackTiers(data: unknown): CashbackThreshold[] {
  if (!Array.isArray(data)) return []

  return data
    .filter(
      (tier): tier is Record<string, unknown> =>
        typeof tier === 'object' && tier !== null
    )
    .map((tier) => {
      const cashbackPercent = Number(tier.cashback_percent)
      const referralCashbackPercent = Number(tier.referral_cashback_percent)
      return {
        min_amount: Number(tier.min_amount),
        cashback_percent: cashbackPercent,
        ...(Number.isFinite(referralCashbackPercent) &&
        referralCashbackPercent >= cashbackPercent &&
        referralCashbackPercent <= 100
          ? { referral_cashback_percent: referralCashbackPercent }
          : {}),
      }
    })
    .filter(
      (tier) =>
        Number.isFinite(tier.min_amount) &&
        tier.min_amount >= 0 &&
        Number.isFinite(tier.cashback_percent) &&
        tier.cashback_percent >= 0 &&
        tier.cashback_percent <= 100
    )
    .sort((left, right) => left.min_amount - right.min_amount)
}

export function getCashbackPercentForAmount(
  amount: number,
  cashback: CashbackThreshold[] = []
): number {
  let cashbackPercent = 0
  let bestMinAmount = -1
  for (const threshold of normalizeCashbackTiers(cashback)) {
    const minAmount = Number(threshold.min_amount)
    const percent = Number(threshold.cashback_percent)
    if (
      minAmount <= amount &&
      minAmount > bestMinAmount &&
      Number.isFinite(percent) &&
      percent >= 0
    ) {
      bestMinAmount = minAmount
      cashbackPercent = percent
    }
  }
  return cashbackPercent
}

export type PaymentMethodDisplayQuote = {
  currency: string
  baseAmountUSD: number
  commissionUSD: number
  creditedAmountUSD: number
  cashbackPercent: number
  cashbackAmountUSD: number
  chargedAmountUSD: number
  chargedAmount: number
}

function getDecimalParts(
  value: number
): { digits: bigint; scale: number } | null {
  const [rawCoefficient, exponentValue] = value
    .toString()
    .toLowerCase()
    .split('e')
  const exponent = exponentValue ? Number(exponentValue) : 0
  const sign = rawCoefficient.startsWith('-') ? -1n : 1n
  const coefficient = rawCoefficient.replace(/^[+-]/, '')
  const [whole, fraction = ''] = coefficient.split('.')
  const scale = fraction.length - exponent
  if (!Number.isInteger(exponent) || Math.abs(scale) > 20) return null
  const digits = `${whole}${fraction}`.replace(/^0+(?=\d)/, '') || '0'
  return { digits: sign * BigInt(digits), scale }
}

type DecimalValue = { digits: bigint; scale: number }
type FractionValue = { numerator: bigint; denominator: bigint }

function multiplyDecimalValues(
  left: DecimalValue,
  right: DecimalValue
): DecimalValue {
  return {
    digits: left.digits * right.digits,
    scale: left.scale + right.scale,
  }
}

function decimalToFraction(value: DecimalValue): FractionValue {
  if (value.scale >= 0) {
    return { numerator: value.digits, denominator: 10n ** BigInt(value.scale) }
  }
  return {
    numerator: value.digits * 10n ** BigInt(-value.scale),
    denominator: 1n,
  }
}

function divideDecimalValues(
  left: DecimalValue,
  right: DecimalValue
): FractionValue {
  const leftFraction = decimalToFraction(left)
  const rightFraction = decimalToFraction(right)
  return {
    numerator: leftFraction.numerator * rightFraction.denominator,
    denominator: leftFraction.denominator * rightFraction.numerator,
  }
}

function subtractFractionValues(
  left: FractionValue,
  right: FractionValue
): FractionValue {
  return {
    numerator:
      left.numerator * right.denominator - right.numerator * left.denominator,
    denominator: left.denominator * right.denominator,
  }
}

function multiplyFractionValues(
  left: FractionValue,
  right: FractionValue
): FractionValue {
  return {
    numerator: left.numerator * right.numerator,
    denominator: left.denominator * right.denominator,
  }
}

function fractionToNumber(value: FractionValue): number | null {
  if (value.denominator === 0n) return null
  const result = Number(value.numerator) / Number(value.denominator)
  return Number.isFinite(result) ? result : null
}

function roundDisplayAmount(
  values: DecimalValue[],
  decimals: number
): DecimalValue | null {
  let digits = 1n
  let scale = 0
  for (const value of values) {
    const product = multiplyDecimalValues({ digits, scale }, value)
    digits = product.digits
    scale = product.scale
  }
  if (Math.abs(scale) > 20) return null

  if (scale <= decimals) {
    return {
      digits: digits * 10n ** BigInt(decimals - scale),
      scale: decimals,
    }
  }
  const divisor = 10n ** BigInt(scale - decimals)
  // Provider invoices are rounded upward on the server. Keeping the same
  // positive-value ceiling here prevents the preview from showing a smaller
  // amount (for example 1.001 -> 1.01 at two decimals) than the immutable
  // settlement quote created on submit.
  return {
    digits: (digits + divisor - 1n) / divisor,
    scale: decimals,
  }
}

export function getPaymentMethodDisplayQuote(
  amount: number,
  method: PaymentMethod,
  cashback: CashbackThreshold[] = []
): PaymentMethodDisplayQuote | null {
  if (isDirectUSDTPayment(method.type)) {
    if (!Number.isFinite(amount) || amount <= 0) return null
    const { config } = getCurrencyDisplay()
    const baseAmountUSD =
      config.quotaDisplayType === 'TOKENS'
        ? amount / config.quotaPerUnit
        : amount
    if (!Number.isFinite(baseAmountUSD) || baseAmountUSD <= 0) return null
    const cashbackPercent = getCashbackPercentForAmount(baseAmountUSD, cashback)
    const cashbackAmountUSD = (baseAmountUSD * cashbackPercent) / 100
    return {
      currency: 'USDT',
      baseAmountUSD,
      commissionUSD: 0,
      creditedAmountUSD: baseAmountUSD,
      cashbackPercent,
      cashbackAmountUSD,
      chargedAmountUSD: baseAmountUSD,
      chargedAmount: baseAmountUSD,
    }
  }

  // The wallet receives these values once with top-up metadata. Never fill in
  // a missing currency locally: doing so can make an incompletely configured
  // gateway look payable while the server cannot build the same quote.
  const currency = method.currency?.trim().toUpperCase()
  const rateToUSD = Number(method.rate_to_usd)
  const multiplier = Number(method.base_amount_multiplier)
  const coefficient = Number(method.topup_ratio)
  const roundingDecimals = Number(method.rounding_decimals)
  if (
    !currency ||
    !Number.isFinite(amount) ||
    amount <= 0 ||
    !Number.isFinite(rateToUSD) ||
    rateToUSD <= 0 ||
    !Number.isFinite(multiplier) ||
    multiplier <= 0 ||
    !Number.isFinite(coefficient) ||
    coefficient <= 0 ||
    !Number.isInteger(roundingDecimals) ||
    roundingDecimals < 0 ||
    roundingDecimals > 8
  ) {
    return null
  }

  const amountDecimal = getDecimalParts(amount)
  const multiplierDecimal = getDecimalParts(multiplier)
  const rateDecimal = getDecimalParts(rateToUSD)
  const coefficientDecimal = getDecimalParts(coefficient)
  if (
    !amountDecimal ||
    !multiplierDecimal ||
    !rateDecimal ||
    !coefficientDecimal
  ) {
    return null
  }

  const effectiveCoefficientDecimal =
    coefficient > 1 ? coefficientDecimal : { digits: 1n, scale: 0 }

  const baseAmountDecimal = multiplyDecimalValues(
    amountDecimal,
    multiplierDecimal
  )
  const baseAmountUSD = fractionToNumber(decimalToFraction(baseAmountDecimal))
  if (baseAmountUSD === null) return null
  const cashbackPercent = getCashbackPercentForAmount(baseAmountUSD, cashback)
  const chargedAmountDecimal = roundDisplayAmount(
    [
      amountDecimal,
      multiplierDecimal,
      effectiveCoefficientDecimal,
      rateDecimal,
    ],
    roundingDecimals
  )
  if (chargedAmountDecimal === null) return null

  const chargedAmount = fractionToNumber(
    decimalToFraction(chargedAmountDecimal)
  )
  const chargedAmountUSD = fractionToNumber(
    divideDecimalValues(chargedAmountDecimal, rateDecimal)
  )
  if (chargedAmount === null || chargedAmountUSD === null) return null

  const commissionFraction = subtractFractionValues(
    divideDecimalValues(chargedAmountDecimal, rateDecimal),
    decimalToFraction(baseAmountDecimal)
  )
  const commissionUSD =
    commissionFraction.numerator > 0n ? fractionToNumber(commissionFraction) : 0
  if (commissionUSD === null) return null

  const cashbackPercentDecimal = getDecimalParts(cashbackPercent)
  if (!cashbackPercentDecimal) return null
  const cashbackAmountUSD = fractionToNumber(
    multiplyFractionValues(
      decimalToFraction(baseAmountDecimal),
      multiplyFractionValues(decimalToFraction(cashbackPercentDecimal), {
        numerator: 1n,
        denominator: 100n,
      })
    )
  )
  if (cashbackAmountUSD === null) return null
  return {
    currency,
    baseAmountUSD,
    commissionUSD,
    // The entered amount is the wallet credit. The fee is charged on top of
    // it and is represented separately in the summary.
    creditedAmountUSD: baseAmountUSD,
    cashbackPercent,
    cashbackAmountUSD,
    chargedAmountUSD,
    chargedAmount,
  }
}

export interface CashbackTierSummary {
  current: CashbackThreshold | null
  next: CashbackThreshold | null
  progress: number
}

export function getCashbackTierSummary(
  amount: number,
  cashback: CashbackThreshold[] = []
): CashbackTierSummary {
  const tiers = normalizeCashbackTiers(cashback)

  const normalizedAmount = Number.isFinite(amount) ? Math.max(0, amount) : 0
  let current: CashbackThreshold | null = null
  let next: CashbackThreshold | null = null

  for (const tier of tiers) {
    if (tier.min_amount <= normalizedAmount) {
      current = tier
    } else {
      next = tier
      break
    }
  }

  if (!next) {
    return { current, next: null, progress: current ? 100 : 0 }
  }

  const start = current?.min_amount ?? 0
  const range = next.min_amount - start
  const progress =
    range > 0
      ? Math.min(100, Math.max(0, ((normalizedAmount - start) / range) * 100))
      : 0

  return { current, next, progress }
}

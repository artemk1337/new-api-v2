const FIXED_PAYMENT_CURRENCIES: Record<string, string> = {
  stripe: 'USD',
  waffo_pancake: 'USD',
  yookassa_sbp: 'RUB',
  nowpayments: 'USDT',
}

const PROVIDER_OWNED_MINIMUMS = new Set([
  'stripe',
  'waffo',
  'waffo_pancake',
  'yookassa_sbp',
  'nowpayments',
])

/** Whether the minimum is configured in the provider integration tab. */
export function hasEditablePaymentMethodMinimum(paymentType: string): boolean {
  return !PROVIDER_OWNED_MINIMUMS.has(paymentType.trim().toLowerCase())
}

export function getPaymentMethodMinimumForDisplay(
  paymentType: string,
  minimum?: string
): string | undefined {
  return hasEditablePaymentMethodMinimum(paymentType) ? minimum : undefined
}

/**
 * PayMethods stores the minimum in the settlement currency of that method.
 * EPay methods settle in USD; Waffo uses the currency configured by its
 * integration. Keep this mapping in one place so the editor cannot imply
 * that every minimum is a USD value.
 */
export function getPaymentMethodMinimumCurrency(
  paymentType: string,
  waffoCurrency = 'USD'
): string {
  const normalizedType = paymentType.trim().toLowerCase()
  if (normalizedType === 'waffo') {
    return waffoCurrency.trim().toUpperCase() || 'USD'
  }
  return FIXED_PAYMENT_CURRENCIES[normalizedType] ?? 'USD'
}

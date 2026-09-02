const FIXED_PAYMENT_CURRENCIES: Record<string, string> = {
  crypto_direct: 'USDT',
  usdt_trc20_direct: 'USDT',
  usdt_ton_direct: 'USDT',
  usdt_solana_direct: 'USDT',
  stripe: 'USD',
  waffo_pancake: 'USD',
  yookassa_sbp: 'RUB',
  manual_transfer: 'RUB',
  nowpayments: 'USDT',
}

/** Every payment method may override its effective minimum in the method dialog. */
export function hasEditablePaymentMethodMinimum(paymentType: string): boolean {
  return paymentType.trim().length > 0
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

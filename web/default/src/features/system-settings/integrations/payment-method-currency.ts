import type { PlatformCurrency } from '../general/platform-currencies-api'

/**
 * Keep currency choices aligned with the gateway validation contract.
 */
export function getSupportedPaymentCurrencies(
  paymentType: string,
  currencies: PlatformCurrency[],
  advertisedCurrency?: string
): PlatformCurrency[] {
  const type = paymentType.trim().toLowerCase()
  const advertised = advertisedCurrency?.trim().toUpperCase()
  let supportedCodes = ['USD']
  if (type === 'yookassa_sbp') {
    supportedCodes = ['RUB']
  } else if (
    type === 'nowpayments' ||
    type === 'crypto_direct' ||
    type === 'usdt_trc20_direct' ||
    type === 'usdt_ton_direct' ||
    type === 'usdt_solana_direct'
  ) {
    supportedCodes = ['USDT']
  } else if (type === 'waffo') {
    supportedCodes = [advertised || 'USD']
  }

  return currencies.filter((currency) =>
    supportedCodes.includes(currency.code.trim().toUpperCase())
  )
}

export function getPreferredPaymentCurrency(
  paymentType: string,
  waffoCurrency?: string
): string {
  const type = paymentType.trim().toLowerCase()
  if (type === 'yookassa_sbp') return 'RUB'
  if (
    type === 'nowpayments' ||
    type === 'crypto_direct' ||
    type === 'usdt_trc20_direct' ||
    type === 'usdt_ton_direct' ||
    type === 'usdt_solana_direct'
  )
    return 'USDT'
  if (type === 'waffo') return waffoCurrency || 'USD'
  return 'USD'
}

export function usesFixedPaymentCurrency(paymentType: string): boolean {
  const type = paymentType.trim().toLowerCase()
  switch (type) {
    case 'nowpayments':
    case 'usdt_trc20_direct':
    case 'usdt_ton_direct':
    case 'usdt_solana_direct':
    case 'stripe':
    case 'waffo':
    case 'waffo_pancake':
    case 'yookassa_sbp':
      return true
    default:
      // Custom methods are handled by EPay, whose settlement currency is USD.
      return type !== ''
  }
}

export function normalizePaymentMethodCurrency(
  paymentType: string,
  _currency: string,
  waffoCurrency?: string
): string {
  return getPreferredPaymentCurrency(paymentType, waffoCurrency)
}

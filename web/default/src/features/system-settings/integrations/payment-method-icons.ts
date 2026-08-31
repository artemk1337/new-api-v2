export const BUILT_IN_PAYMENT_ICONS = [
  'LuCreditCard',
  'LuWalletCards',
  'LuBitcoin',
  'LuBanknote',
  'SiAlipay',
  'SiWechat',
  'SiStripe',
  'SiPaypal',
  'SiVisa',
  'SiMastercard',
  'SiTether',
  'SiTron',
  'SiSolana',
] as const

export function parseAvailablePaymentIcons(value: string | undefined): string[] {
  if (!value?.trim()) return [...BUILT_IN_PAYMENT_ICONS]
  try {
    const parsed: unknown = JSON.parse(value)
    if (!Array.isArray(parsed)) return [...BUILT_IN_PAYMENT_ICONS]
    const available = new Set(parsed.filter((item): item is string => typeof item === 'string'))
    return BUILT_IN_PAYMENT_ICONS.filter((icon) => available.has(icon))
  } catch {
    return [...BUILT_IN_PAYMENT_ICONS]
  }
}

export function serializeAvailablePaymentIcons(icons: string[]): string {
  const selected = new Set(icons)
  return JSON.stringify(
    BUILT_IN_PAYMENT_ICONS.filter((icon) => selected.has(icon))
  )
}

export function getPaymentTypeLabel(
  type: string,
  fallback: string,
  t: (key: string) => string
): string {
  if (type.trim().toLowerCase() === 'crypto_direct' || type.trim().toLowerCase().startsWith('usdt_'))
    return t('Crypto')
  return fallback
}

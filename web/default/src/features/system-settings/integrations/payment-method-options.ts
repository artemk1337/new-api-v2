type PaymentTypeOption = {
  iconName: string
  label: string
  name: string
  value: string
}

export const CRYPTO_PAYMENT_TYPE = 'crypto_direct'
export const MANUAL_TRANSFER_PAYMENT_TYPE = 'manual_transfer'

const LEGACY_CRYPTO_PAYMENT_TYPES = [
  'usdt_trc20_direct',
  'usdt_ton_direct',
  'usdt_solana_direct',
] as const

/** Normalize persisted pre-unification crypto entries for the editor/runtime. */
export function normalizePaymentMethodType(type: string): string {
  const normalized = type.trim().toLowerCase()
  return (LEGACY_CRYPTO_PAYMENT_TYPES as readonly string[]).includes(normalized)
    ? CRYPTO_PAYMENT_TYPE
    : normalized
}

export function isCryptoPaymentType(type: string): boolean {
  return normalizePaymentMethodType(type) === CRYPTO_PAYMENT_TYPE
}

export function getPaymentTypeOptions(
  t: (key: string) => string
): PaymentTypeOption[] {
  return [
    {
      iconName: 'LuCreditCard',
      label: 'СБП / YooKassa',
      name: 'СБП / YooKassa',
      value: 'yookassa_sbp',
    },
    {
      iconName: 'SiAlipay',
      label: t('Alipay'),
      name: t('Alipay'),
      value: 'alipay',
    },
    {
      iconName: 'SiWechat',
      label: t('WeChat Pay'),
      name: t('WeChat Pay'),
      value: 'wxpay',
    },
    {
      iconName: 'SiStripe',
      label: t('Stripe'),
      name: t('Stripe'),
      value: 'stripe',
    },
    {
      iconName: 'LuCreditCard',
      label: 'Waffo Pancake',
      name: 'Waffo Pancake',
      value: 'waffo_pancake',
    },
    {
      iconName: 'LuCreditCard',
      label: t('Waffo'),
      name: t('Waffo'),
      value: 'waffo',
    },
    {
      iconName: 'LuBitcoin',
      label: 'NOWPayments',
      name: 'NOWPayments',
      value: 'nowpayments',
    },
    {
      iconName: 'LuExternalLink',
      label: t('Direct transfer'),
      name: t('Direct transfer'),
      value: MANUAL_TRANSFER_PAYMENT_TYPE,
    },
    {
      iconName: 'LuWalletCards',
      label: t('Crypto'),
      name: t('Crypto'),
      value: CRYPTO_PAYMENT_TYPE,
    },
  ]
}

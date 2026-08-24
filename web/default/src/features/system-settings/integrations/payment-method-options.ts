type PaymentTypeOption = {
  iconName: string
  label: string
  name: string
  value: string
}

export function getPaymentTypeOptions(
  t: (key: string) => string
): PaymentTypeOption[] {
  return [
    {
      iconName: 'LuCreditCard',
      label: 'СБП / YooKassa (yookassa_sbp)',
      name: 'СБП / YooKassa',
      value: 'yookassa_sbp',
    },
    {
      iconName: 'SiAlipay',
      label: `${t('Alipay')} (Epay: alipay)`,
      name: t('Alipay'),
      value: 'alipay',
    },
    {
      iconName: 'SiWechat',
      label: `${t('WeChat Pay')} (Epay: wxpay)`,
      name: t('WeChat Pay'),
      value: 'wxpay',
    },
    {
      iconName: 'SiStripe',
      label: `${t('Stripe')} (stripe)`,
      name: t('Stripe'),
      value: 'stripe',
    },
    {
      iconName: 'LuCreditCard',
      label: 'Waffo Pancake (waffo_pancake)',
      name: 'Waffo Pancake',
      value: 'waffo_pancake',
    },
  ]
}

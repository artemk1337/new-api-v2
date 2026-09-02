type OptionUpdate = {
  key: string
}

function getPaymentOptionUpdatePriority(key: string): number {
  switch (key) {
    case 'WaffoCurrency':
    case 'YooKassaShopID':
    case 'YooKassaSecretKey':
    case 'YooKassaReturnURL':
    case 'NOWPaymentsAPIKey':
    case 'NOWPaymentsIPNSecret':
    case 'NOWPaymentsIPNCallbackURL':
      return 1
    case 'WaffoEnabled':
    case 'YooKassaEnabled':
    case 'NOWPaymentsEnabled':
      return 3
    default:
      return 2
  }
}

/**
 * Saves gateway prerequisites before fields that can make a gateway ready.
 * The original order is retained for unrelated updates.
 */
export function orderPaymentOptionUpdates<T extends OptionUpdate>(
  updates: readonly T[]
): T[] {
  return updates
    .map((update, index) => ({ update, index }))
    .sort(
      (left, right) =>
        getPaymentOptionUpdatePriority(left.update.key) -
          getPaymentOptionUpdatePriority(right.update.key) ||
        left.index - right.index
    )
    .map(({ update }) => update)
}

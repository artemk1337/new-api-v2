import { safeJsonParseWithValidation } from '../utils/json-parser'
import { isArray } from '../utils/json-validators'
import type { PaymentMethodData } from './payment-method-dialog'
import {
  CRYPTO_PAYMENT_TYPE,
  normalizePaymentMethodType,
} from './payment-method-options'

export type PaymentMethodMoveDirection = 'up' | 'down'

function isPaymentMethodItem(item: unknown): item is PaymentMethodData {
  return (
    typeof item === 'object' &&
    item !== null &&
    'name' in item &&
    'type' in item &&
    typeof item.name === 'string' &&
    typeof item.type === 'string'
  )
}

/**
 * Moves one payment method while retaining unrelated JSON array entries and
 * the persisted metadata of every method.
 */
export function movePaymentMethodInJson(
  value: string,
  method: Pick<PaymentMethodData, 'name' | 'type'>,
  direction: PaymentMethodMoveDirection
): string | null {
  const parsed = safeJsonParseWithValidation<unknown[]>(value, {
    fallback: [],
    validator: isArray,
    silent: true,
  })
  const normalizedType = normalizePaymentMethodType(method.type)
  const seenTypes = new Set<string>()
  const methodIndexes = parsed.reduce<number[]>((indexes, item, index) => {
    if (!isPaymentMethodItem(item)) return indexes
    const itemType = normalizePaymentMethodType(item.type)
    if (itemType === CRYPTO_PAYMENT_TYPE && seenTypes.has(itemType)) {
      return indexes
    }
    seenTypes.add(itemType)
    indexes.push(index)
    return indexes
  }, [])
  const targetIndex = methodIndexes.findIndex((index) => {
    const item = parsed[index]
    if (!isPaymentMethodItem(item)) return false
    return (
      normalizePaymentMethodType(item.type) === normalizedType &&
      (normalizedType === CRYPTO_PAYMENT_TYPE || item.name === method.name)
    )
  })

  if (targetIndex === -1) return null

  const adjacentIndex = targetIndex + (direction === 'up' ? -1 : 1)
  if (adjacentIndex < 0 || adjacentIndex >= methodIndexes.length) return null

  const reordered = [...parsed]
  const currentArrayIndex = methodIndexes[targetIndex]
  const adjacentArrayIndex = methodIndexes[adjacentIndex]
  const adjacentItem = parsed[adjacentArrayIndex]
  if (!isPaymentMethodItem(adjacentItem)) return null
  const adjacentType = normalizePaymentMethodType(adjacentItem.type)
  const targetArrayIndexes =
    normalizedType === CRYPTO_PAYMENT_TYPE
      ? parsed.reduce<number[]>((indexes, item, index) => {
          if (
            isPaymentMethodItem(item) &&
            normalizePaymentMethodType(item.type) === CRYPTO_PAYMENT_TYPE
          ) {
            indexes.push(index)
          }
          return indexes
        }, [])
      : [currentArrayIndex]
  const adjacentArrayIndexes =
    adjacentType === CRYPTO_PAYMENT_TYPE
      ? parsed.reduce<number[]>((indexes, item, index) => {
          if (
            isPaymentMethodItem(item) &&
            normalizePaymentMethodType(item.type) === CRYPTO_PAYMENT_TYPE
          ) {
            indexes.push(index)
          }
          return indexes
        }, [])
      : [adjacentArrayIndex]
  const reorderedIndexes = [...targetArrayIndexes, ...adjacentArrayIndexes].sort(
    (left, right) => left - right
  )
  const targetEntries = targetArrayIndexes.map((index) => parsed[index])
  const adjacentEntries = adjacentArrayIndexes.map((index) => parsed[index])
  const entries =
    direction === 'up'
      ? [...targetEntries, ...adjacentEntries]
      : [...adjacentEntries, ...targetEntries]

  for (const [index, arrayIndex] of reorderedIndexes.entries()) {
    reordered[arrayIndex] = entries[index]
  }

  return JSON.stringify(reordered, null, 2)
}

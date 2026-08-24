import { walletDisplayAmountToBackend } from '@/lib/currency'

export function getBackendTopupAmount(amount: number | null): number {
  return amount === null ? 0 : walletDisplayAmountToBackend(amount)
}

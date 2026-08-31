/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type { DirectUSDTNetwork, PaymentRequest } from '../types'

const directCryptoNetworkPaths: Record<DirectUSDTNetwork, string> = {
  TRON: 'tron',
  TON: 'ton',
  SOLANA: 'solana',
}

export function getDirectCryptoInvoicePath(
  network: DirectUSDTNetwork,
  tradeNo: string
): string {
  return `/crypto/${directCryptoNetworkPaths[network]}/${encodeURIComponent(tradeNo)}`
}

export function getDirectCryptoCheckoutSearch(
  paymentType: string,
  amount: number
): { amount: number } | null {
  if (
    paymentType.trim().toLowerCase() !== 'crypto_direct' ||
    !Number.isFinite(amount) ||
    amount <= 0
  ) {
    return null
  }
  return { amount }
}

export function isSafeDirectCryptoInvoiceUrl(value: string): boolean {
  const trimmed = value.trim()
  if (!trimmed || !trimmed.startsWith('/') || trimmed.startsWith('//')) {
    return false
  }

  try {
    const parsed = new URL(trimmed, 'https://internal.invalid')
    return (
      !parsed.search &&
      !parsed.hash &&
      /^\/crypto\/(tron|ton|solana)\/[^/?#%]+$/.test(parsed.pathname)
    )
  } catch {
    return false
  }
}

export function parseDirectCryptoInvoiceUrl(
  value: string
): { network: DirectUSDTNetwork; tradeNo: string } | null {
  if (!isSafeDirectCryptoInvoiceUrl(value)) return null
  const [, , network, tradeNo] = value.trim().split('/')
  const networks: Record<string, DirectUSDTNetwork> = {
    tron: 'TRON',
    ton: 'TON',
    solana: 'SOLANA',
  }
  return networks[network] ? { network: networks[network], tradeNo } : null
}

export function prepareDirectCryptoPayment(
  amount: number,
  availableNetworks: DirectUSDTNetwork[],
  selectedNetwork: DirectUSDTNetwork | null,
  directMethodAvailable: boolean
): { network: DirectUSDTNetwork; request: PaymentRequest } | null {
  if (
    !directMethodAvailable ||
    !Number.isFinite(amount) ||
    amount <= 0 ||
    !selectedNetwork ||
    !availableNetworks.includes(selectedNetwork)
  ) {
    return null
  }

  return {
    network: selectedNetwork,
    request: { amount, payment_method: 'crypto_direct' },
  }
}

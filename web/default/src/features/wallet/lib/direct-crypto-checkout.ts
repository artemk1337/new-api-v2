/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import type {
  DirectUSDTNetwork,
  DirectUSDTPaymentStatus,
  PaymentRequest,
  TopupRecord,
} from '../types'

const directCryptoNetworkPaths: Record<DirectUSDTNetwork, string> = {
  TRON: 'tron',
  TON: 'ton',
  SOLANA: 'solana',
}

const legacyDirectCryptoNetworks: Record<string, DirectUSDTNetwork> = {
  usdt_trc20_direct: 'TRON',
  usdt_ton_direct: 'TON',
  usdt_solana_direct: 'SOLANA',
}

/** Resolve a history row's chain, including pre-crypto_direct provider IDs. */
export function getDirectCryptoHistoryNetwork(
  paymentProvider: string | undefined,
  paymentMethod: string | undefined,
  tradeNo: string
): DirectUSDTNetwork | null {
  for (const value of [paymentProvider, paymentMethod]) {
    const network = value
      ? legacyDirectCryptoNetworks[value.trim().toLowerCase()]
      : undefined
    if (network) return network
  }

  if (
    paymentProvider?.trim().toLowerCase() !== 'crypto_direct' &&
    paymentMethod?.trim().toLowerCase() !== 'crypto_direct'
  ) {
    return null
  }

  const normalizedTradeNo = tradeNo.trim().toUpperCase()
  if (normalizedTradeNo.startsWith('TON')) return 'TON'
  if (normalizedTradeNo.startsWith('SOLANA')) return 'SOLANA'
  return 'TRON'
}

export function canOpenDirectCryptoHistoryPayment(
  record: Pick<
    TopupRecord,
    'status' | 'user_id' | 'payment_provider' | 'payment_method' | 'trade_no'
  >,
  currentUserId: number | undefined
): boolean {
  return (
    record.status === 'pending' &&
    currentUserId !== undefined &&
    record.user_id === currentUserId &&
    getDirectCryptoHistoryNetwork(
      record.payment_provider,
      record.payment_method,
      record.trade_no
    ) !== null
  )
}

export function getDirectCryptoInvoicePath(
  network: DirectUSDTNetwork,
  tradeNo: string
): string {
  return `/crypto/${directCryptoNetworkPaths[network]}/${encodeURIComponent(tradeNo)}`
}

export function getDirectCryptoPaymentAddress(
  payment: Pick<
    DirectUSDTPaymentStatus,
    'receiving_address' | 'destination_token_account' | 'address'
  >,
  network: DirectUSDTNetwork
): string {
  if (network === 'SOLANA') {
    return payment.destination_token_account?.trim() || ''
  }
  return (
    payment.destination_token_account?.trim() ||
    payment.receiving_address?.trim() ||
    payment.address?.trim() ||
    ''
  )
}

/** Trim transport padding without coercing exact token amounts through float. */
export function formatDirectCryptoAmount(value: string): string {
  const trimmed = value.trim()
  if (!/^\d+\.\d+$/.test(trimmed)) return trimmed
  return trimmed
    .replace(/(\.\d*?[1-9])0+$/, '$1')
    .replace(/\.0+$/, '')
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

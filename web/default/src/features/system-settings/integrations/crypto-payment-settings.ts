export const CRYPTO_PAYMENT_CURRENCY = 'USDT' as const
export const CRYPTO_AMOUNT_TAIL_SCALE = 1_000_000
export const CRYPTO_AMOUNT_TAIL_MIN_UNITS = 2
export const CRYPTO_AMOUNT_TAIL_MAX_UNITS = 10_000
export const CRYPTO_AMOUNT_TAIL_DEFAULT_UNITS = CRYPTO_AMOUNT_TAIL_MAX_UNITS

/** User-facing presets for the random USDT suffix. */
export const CRYPTO_AMOUNT_PRECISION_OPTIONS = [3, 4, 5, 6] as const

export const CRYPTO_NETWORKS = [
  { value: 'TRON', supported: true },
  { value: 'TON', supported: true },
  { value: 'SOLANA', supported: true },
] as const

export type CryptoReceivingAddressField = {
  name:
    | 'USDTTRC20ReceivingAddress'
    | 'USDTTONReceivingAddress'
    | 'USDTSolanaReceivingAddress'
    | 'USDTSolanaReceivingTokenAccount'
  network: (typeof CRYPTO_NETWORKS)[number]['value']
  supported: boolean
}

export const CRYPTO_RECEIVING_ADDRESS_FIELDS: CryptoReceivingAddressField[] = [
  { name: 'USDTTRC20ReceivingAddress', network: 'TRON', supported: true },
  { name: 'USDTTONReceivingAddress', network: 'TON', supported: true },
  { name: 'USDTSolanaReceivingAddress', network: 'SOLANA', supported: true },
  { name: 'USDTSolanaReceivingTokenAccount', network: 'SOLANA', supported: true },
]

export function isCryptoNetworkPayable(network: string): boolean {
  return CRYPTO_NETWORKS.some(
    (option) => option.value === network.toUpperCase() && option.supported
  )
}

export function decimalUsdtToMicroUnits(value: string): number | null {
  const normalized = value.trim()
  const match = /^0(?:\.(\d{1,6}))?$/.exec(normalized)
  if (!match) return null

  const fraction = (match[1] ?? '').padEnd(6, '0')
  let units = 0
  for (const digit of fraction) units = units * 10 + digit.charCodeAt(0) - 48
  if (
    !Number.isInteger(units) ||
    units < CRYPTO_AMOUNT_TAIL_MIN_UNITS ||
    units > CRYPTO_AMOUNT_TAIL_MAX_UNITS
  ) {
    return null
  }
  return units
}

export function microUnitsToDecimalUsdt(units: number): string {
  if (
    !Number.isInteger(units) ||
    units < CRYPTO_AMOUNT_TAIL_MIN_UNITS ||
    units > CRYPTO_AMOUNT_TAIL_MAX_UNITS
  ) {
    return ''
  }
  const fraction = String(units % CRYPTO_AMOUNT_TAIL_SCALE).padStart(6, '0')
  const trimmedFraction = fraction.replace(/0+$/, '')
  return trimmedFraction ? `0.${trimmedFraction}` : '0'
}

export function cryptoAmountTailVariants(value: string): number | null {
  const units = decimalUsdtToMicroUnits(value)
  return units === null ? null : units - 1
}

export function shouldUpdateCryptoPaymentCredential(
  currentValue: string,
  initialValue: string
): boolean {
  const current = currentValue.trim()
  return current !== '' && current !== initialValue.trim()
}

export function legacyPrecisionToTailLimitUnits(
  precision: string | number
): number | null {
  const normalized = String(precision).trim()
  if (!/^[3-6]$/.test(normalized)) return null
  return {
    '3': 10,
    '4': 100,
    '5': 1_000,
    '6': 10_000,
  }[normalized] ?? null
}

export function cryptoPrecisionToTailLimitUnits(
  precision: string | number
): number | null {
  return legacyPrecisionToTailLimitUnits(precision)
}

export function cryptoTailLimitUnitsToPrecision(units: number): number | null {
  const option = CRYPTO_AMOUNT_PRECISION_OPTIONS.find(
    (precision) => legacyPrecisionToTailLimitUnits(precision) === units
  )
  return option ?? null
}

export function resolveCryptoTailLimitSettings<
  T extends { USDTTRC20AmountTailLimitUnits: number },
>(settings: T, raw: Array<{ key: string; value: string }> | undefined): T {
  if (
    raw?.some((option) => option.key === 'USDTTRC20AmountTailLimitUnits')
  ) {
    return settings
  }
  const legacy = raw?.find(
    (option) => option.key === 'USDTTRC20AmountPrecision'
  )
  const mapped = legacyPrecisionToTailLimitUnits(legacy?.value ?? '')
  if (mapped === null) return settings
  return { ...settings, USDTTRC20AmountTailLimitUnits: mapped }
}

export function isValidCryptoAmountTailLimit(value: string): boolean {
  return decimalUsdtToMicroUnits(value) !== null
}

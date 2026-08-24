const PROVIDER_LABELS: Record<string, string> = {
  bybit_p2p: 'Bybit',
  cbr: 'CBR',
  coingecko: 'CoinGecko',
}

const PROVIDER_CURRENCIES: Record<string, ReadonlySet<string>> = {
  cbr: new Set([
    'AED',
    'AMD',
    'AUD',
    'AZN',
    'BGN',
    'BRL',
    'BYN',
    'CAD',
    'CHF',
    'CNY',
    'CZK',
    'DKK',
    'EGP',
    'EUR',
    'GBP',
    'GEL',
    'HKD',
    'HUF',
    'IDR',
    'INR',
    'JPY',
    'KGS',
    'KRW',
    'KZT',
    'MDL',
    'NOK',
    'NZD',
    'PLN',
    'QAR',
    'RON',
    'RSD',
    'RUB',
    'SEK',
    'SGD',
    'THB',
    'TJS',
    'TMT',
    'TRY',
    'UAH',
    'UZS',
    'VND',
    'XDR',
    'ZAR',
  ]),
  coingecko: new Set(['USDT']),
}

export function getSyncProviderLabel(provider: string) {
  return PROVIDER_LABELS[provider] ?? provider
}

export function getSupportedSyncProviders(currencyCode: string) {
  const code = currencyCode.trim().toUpperCase()
  return Object.keys(PROVIDER_CURRENCIES).filter((provider) =>
    PROVIDER_CURRENCIES[provider]?.has(code)
  )
}

export function getSyncIntervalLabel(
  interval: string,
  t: (key: string) => string
) {
  const labels: Record<string, string> = {
    minute: t('Every minute'),
    hour: t('Every hour'),
    day: t('Every day'),
  }
  return labels[interval] ?? interval
}

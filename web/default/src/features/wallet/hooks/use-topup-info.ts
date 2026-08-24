/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { useState, useEffect, useCallback } from 'react'

import { getAvailableCurrencies, getTopupInfo } from '../api'
import {
  generatePresetAmounts,
  mergePresetAmounts,
  getMinTopupAmount,
  normalizeCashbackTiers,
} from '../lib'
import type {
  TopupInfo,
  PresetAmount,
  CreemProduct,
  PaymentMethod,
  WaffoPayMethod,
  CashbackThreshold,
} from '../types'

// ============================================================================
// Topup Info Hook
// ============================================================================

function parseJsonArray(data: unknown): unknown[] {
  if (Array.isArray(data)) {
    return data
  }

  if (typeof data === 'string') {
    try {
      const parsed = JSON.parse(data)
      return Array.isArray(parsed) ? parsed : []
    } catch {
      return []
    }
  }

  return []
}

function parsePaymentMethods(
  data: unknown,
  stripeMinTopup: number
): PaymentMethod[] {
  return parseJsonArray(data)
    .filter(
      (item): item is Record<string, unknown> =>
        !!item && typeof item === 'object'
    )
    .map((item) => {
      const rawMinTopup = Number(item.min_topup)
      const normalizedMinTopup = Number.isFinite(rawMinTopup) ? rawMinTopup : 0
      const type = typeof item.type === 'string' ? item.type : ''
      const rawTopupRatio = Number(item.topup_ratio)
      const rawPaymentAmount = Number(item.payment_amount)
      const rawRateToUSD = Number(item.rate_to_usd)
      const rawBaseAmountMultiplier = Number(item.base_amount_multiplier)
      const rawRoundingDecimals = Number(item.rounding_decimals)

      return {
        name: typeof item.name === 'string' ? item.name : '',
        type,
        color: typeof item.color === 'string' ? item.color : undefined,
        icon: typeof item.icon === 'string' ? item.icon : undefined,
        min_topup:
          type === 'stripe' && normalizedMinTopup <= 0
            ? stripeMinTopup
            : normalizedMinTopup,
        topup_ratio: Number.isFinite(rawTopupRatio) ? rawTopupRatio : 1,
        rate_to_usd: Number.isFinite(rawRateToUSD) ? rawRateToUSD : undefined,
        base_amount_multiplier: Number.isFinite(rawBaseAmountMultiplier)
          ? rawBaseAmountMultiplier
          : undefined,
        rounding_decimals: Number.isInteger(rawRoundingDecimals)
          ? rawRoundingDecimals
          : undefined,
        // Currency is part of the amount-independent quote snapshot. Keep a
        // missing value missing so the wallet renders the method as
        // unavailable instead of inventing USD for an incomplete config.
        currency:
          typeof item.currency === 'string' && item.currency.trim()
            ? item.currency.trim().toUpperCase()
            : undefined,
        payment_amount: Number.isFinite(rawPaymentAmount)
          ? rawPaymentAmount
          : undefined,
        currency_symbol:
          typeof item.currency_symbol === 'string'
            ? item.currency_symbol
            : undefined,
      }
    })
    .filter((item) => item.name && item.type)
}

function parseWaffoPayMethods(data: unknown): WaffoPayMethod[] {
  return parseJsonArray(data)
    .filter(
      (item): item is Record<string, unknown> =>
        !!item && typeof item === 'object'
    )
    .map((item) => {
      const rateToUSD = Number(item.rate_to_usd)
      const multiplier = Number(item.base_amount_multiplier)
      const ratio = Number(item.topup_ratio)
      const roundingDecimals = Number(item.rounding_decimals)

      return {
        name: typeof item.name === 'string' ? item.name : '',
        icon: typeof item.icon === 'string' ? item.icon : undefined,
        payMethodType:
          typeof item.payMethodType === 'string'
            ? item.payMethodType
            : undefined,
        payMethodName:
          typeof item.payMethodName === 'string'
            ? item.payMethodName
            : undefined,
        currency:
          typeof item.currency === 'string' && item.currency.trim()
            ? item.currency.trim().toUpperCase()
            : undefined,
        rate_to_usd: Number.isFinite(rateToUSD) ? rateToUSD : undefined,
        base_amount_multiplier: Number.isFinite(multiplier)
          ? multiplier
          : undefined,
        topup_ratio: Number.isFinite(ratio) ? ratio : undefined,
        rounding_decimals: Number.isInteger(roundingDecimals)
          ? roundingDecimals
          : undefined,
        currency_symbol:
          typeof item.currency_symbol === 'string'
            ? item.currency_symbol
            : undefined,
      }
    })
    .filter((item) => item.name)
}

function parseCreemProducts(data: unknown): CreemProduct[] {
  return parseJsonArray(data)
    .filter(
      (item): item is Record<string, unknown> =>
        !!item && typeof item === 'object'
    )
    .map((item) => {
      const currency: CreemProduct['currency'] =
        item.currency === 'EUR' ? 'EUR' : 'USD'

      return {
        name: typeof item.name === 'string' ? item.name : '',
        productId: typeof item.productId === 'string' ? item.productId : '',
        price: Number(item.price) || 0,
        quota: Number(item.quota) || 0,
        currency,
      }
    })
    .filter((item) => item.name && item.productId)
}

function parseAmountOptions(data: unknown): number[] {
  return parseJsonArray(data)
    .map((item) => Number(item))
    .filter((item) => Number.isFinite(item) && item > 0)
}

export function filterAvailablePaymentMethods(
  methods: PaymentMethod[],
  topupInfo: Pick<
    TopupInfo,
    | 'enable_online_topup'
    | 'enable_stripe_topup'
    | 'enable_waffo_topup'
    | 'enable_waffo_pancake_topup'
    | 'enable_yookassa_topup'
    | 'enable_nowpayments_topup'
  >,
  hasVisibleWaffoMethod: boolean
): PaymentMethod[] {
  return methods.filter((method) => {
    switch (method.type) {
      case 'stripe':
        return Boolean(topupInfo.enable_stripe_topup)
      case 'waffo':
        return Boolean(topupInfo.enable_waffo_topup) && !hasVisibleWaffoMethod
      case 'waffo_pancake':
        return Boolean(topupInfo.enable_waffo_pancake_topup)
      case 'yookassa_sbp':
        return Boolean(topupInfo.enable_yookassa_topup)
      case 'nowpayments':
        return Boolean(topupInfo.enable_nowpayments_topup)
      default:
        return Boolean(topupInfo.enable_online_topup)
    }
  })
}

export function parseCashbackConfig(data: unknown): CashbackThreshold[] {
  if (!data) {
    return []
  }

  let parsedData = data

  if (typeof data === 'string') {
    try {
      parsedData = JSON.parse(data)
    } catch {
      return []
    }
  }

  if (!parsedData || typeof parsedData !== 'object') {
    return []
  }

  if (Array.isArray(parsedData)) {
    return normalizeCashbackTiers(parsedData)
  }

  return normalizeCashbackTiers(
    Object.entries(parsedData).map(([key, value]) => ({
      min_amount: Number(key),
      cashback_percent: Number(value),
    }))
  )
}

export function useTopupInfo() {
  const [topupInfo, setTopupInfo] = useState<TopupInfo | null>(null)
  const [presetAmounts, setPresetAmounts] = useState<PresetAmount[]>([])
  const [loading, setLoading] = useState(true)

  const fetchTopupInfo = useCallback(async () => {
    try {
      setLoading(true)

      const [response, availableCurrencies] = await Promise.all([
        getTopupInfo(),
        getAvailableCurrencies().catch(() => []),
      ])

      if (!response.success || !response.data) {
        // eslint-disable-next-line no-console
        console.error('Failed to fetch topup info:', response.message)
        return
      }

      const cashback = parseCashbackConfig(response.data.cashback)
      const symbolsByCode = new Map(
        availableCurrencies.map((currency) => [
          currency.code.toUpperCase(),
          currency.symbol,
        ])
      )
      const waffoPayMethods = parseWaffoPayMethods(
        response.data.waffo_pay_methods
      )
      const hasVisibleWaffoMethod = waffoPayMethods.some((method) => {
        const values = [method.name, method.payMethodType, method.payMethodName]
        return !values.some(
          (value) => value?.trim().toLowerCase() === 'custom1'
        )
      })
      const processedData: TopupInfo = {
        ...response.data,
        pay_methods: filterAvailablePaymentMethods(
          parsePaymentMethods(
            response.data.pay_methods,
            response.data.stripe_min_topup
          ),
          response.data,
          hasVisibleWaffoMethod
        ),
        amount_options: parseAmountOptions(response.data.amount_options),
        cashback,
        creem_products: parseCreemProducts(response.data.creem_products),
        waffo_pay_methods: waffoPayMethods,
      }
      processedData.pay_methods = processedData.pay_methods.map((method) => ({
        ...method,
        currency_symbol:
          method.currency_symbol ||
          symbolsByCode.get(method.currency?.toUpperCase() || 'USD'),
      }))

      setTopupInfo(processedData)

      if (processedData.amount_options.length > 0) {
        const customPresets = mergePresetAmounts(
          processedData.amount_options,
          processedData.cashback || []
        )
        setPresetAmounts(customPresets)
      } else {
        const minTopup = getMinTopupAmount(processedData)
        const defaultPresets = generatePresetAmounts(minTopup)
        setPresetAmounts(defaultPresets)
      }
    } catch (err) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch topup info:', err)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    let cancelled = false

    queueMicrotask(() => {
      if (!cancelled) void fetchTopupInfo()
    })

    return () => {
      cancelled = true
    }
  }, [fetchTopupInfo])

  return {
    topupInfo,
    presetAmounts,
    loading,
    refetch: fetchTopupInfo,
  }
}

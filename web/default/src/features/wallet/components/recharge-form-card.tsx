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
import { ChevronDown, ChevronUp, ExternalLink, Loader2 } from 'lucide-react'
import { useState, useEffect, useMemo, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Skeleton } from '@/components/ui/skeleton'
import {
  backendAmountToWalletDisplay,
  getCurrencyDisplay,
  getCurrencyLabel,
} from '@/lib/currency'
import { formatNumber } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  formatCurrency,
  getPaymentIcon,
  getMinTopupAmount,
  getPaymentMethodMinimumAmount,
  isPaymentMethodAmountEligible,
  getPaymentMethodDisplayQuote,
  getCashbackTierSummary,
  normalizeCashbackTiers,
} from '../lib'
import { getPaymentCurrencyLabel } from '../lib/format'
import { getDirectUSDTNetwork, type CashbackTierSummary } from '../lib/payment'
import { getBackendTopupAmount } from '../lib/topup-input'
import type {
  CashbackThreshold,
  PaymentMethod,
  TopupInfo,
  CreemProduct,
  WaffoPayMethod,
  UserWalletData,
} from '../types'
import { AffiliateRewardsCard } from './affiliate-rewards-card'
import { CreemProductsSection } from './creem-products-section'

export type RechargeValidationTarget = 'amount' | 'payment-method'

interface RechargeFormCardProps {
  topupInfo: TopupInfo | null
  topupAmount: number
  onTopupAmountChange: (amount: number) => void
  onPaymentMethodSelect: (method: PaymentMethod) => void
  selectedPaymentMethod?: PaymentMethod
  selectedWaffoMethodIndex?: number
  redemptionCode: string
  onRedemptionCodeChange: (code: string) => void
  onRedeem: () => void
  redeeming: boolean
  topupLink?: string
  loading?: boolean
  creemProducts?: CreemProduct[]
  enableCreemTopup?: boolean
  onCreemProductSelect?: (product: CreemProduct) => void
  enableWaffoTopup?: boolean
  waffoPayMethods?: WaffoPayMethod[]
  waffoMinTopup?: number
  onWaffoMethodSelect?: (method: WaffoPayMethod, index: number) => void
  enableWaffoPancakeTopup?: boolean
  affiliateUser: UserWalletData | null
  affiliateLink: string
  onTransferAffiliate: () => void
  affiliateLoading?: boolean
  affiliateComplianceConfirmed?: boolean
  validationTarget?: RechargeValidationTarget | null
  onValidationRequest?: () => void
}

export function parseTopupAmount(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (!normalized || normalized.endsWith('.')) {
    return null
  }

  const amount = Number(normalized)
  return Number.isFinite(amount) && amount >= 0 ? amount : null
}

export function formatWalletInputAmount(amount: number): string {
  if (!Number.isFinite(amount) || amount <= 0) return ''
  return backendAmountToWalletDisplay(amount)
    .toFixed(8)
    .replace(/\.0+$/, '')
    .replace(/(\.[0-9]*?)0+$/, '$1')
}

export function sanitizeTopupAmount(value: string): string {
  let sanitized = ''
  let hasDecimalSeparator = false

  for (const character of value) {
    if (character >= '0' && character <= '9') {
      sanitized += character
      continue
    }
    if ((character === '.' || character === ',') && !hasDecimalSeparator) {
      sanitized += '.'
      hasDecimalSeparator = true
    }
  }

  return sanitized
}

export function getRenderableCashbackTiers(
  cashback: CashbackThreshold[] | null | undefined
): CashbackThreshold[] {
  return normalizeCashbackTiers(cashback)
}

export function hasPositiveCashbackTier(
  tiers: CashbackThreshold[] | null | undefined
): boolean {
  return normalizeCashbackTiers(tiers).some((tier) => tier.cashback_percent > 0)
}

export function getRechargeStep(
  amount: number,
  minimum: number,
  hasPaymentMethod: boolean
): 1 | 2 | 3 {
  if (amount <= 0 || amount < minimum) return 1
  return hasPaymentMethod ? 3 : 2
}

function getPaymentMethodDisplayLabel(method: PaymentMethod): string {
  if (method.type === 'crypto_direct') {
    const network = method.crypto_network
    return network
      ? `Crypto · ${network === 'SOLANA' ? 'Solana' : network}`
      : 'Crypto'
  }
  const network = getDirectUSDTNetwork(method.type)
  if (network) return `Crypto · ${network === 'SOLANA' ? 'Solana' : network}`
  return method.name
}

/**
 * Provider minimums are expressed in each gateway's settlement currency.
 * They must be validated by the server quote, not compared with the wallet's
 * USD input in the browser.
 */
export function canSelectPaymentMethod(topupAmount: number): boolean {
  return Number.isFinite(topupAmount) && topupAmount > 0
}

export function getRechargeValidationTarget(
  topupAmount: number,
  minimum: number,
  hasPaymentMethod: boolean
): RechargeValidationTarget | null {
  if (
    !Number.isFinite(topupAmount) ||
    topupAmount <= 0 ||
    topupAmount < minimum
  ) {
    return 'amount'
  }
  return hasPaymentMethod ? null : 'payment-method'
}

export function getManualTopupContact(topupInfo: TopupInfo | null): { minAmount: number; url: string } | null {
  const minAmount = topupInfo?.manual_topup_min_amount
  const url = topupInfo?.manual_topup_contact_url?.trim()
  if (!topupInfo?.manual_topup_enabled || !minAmount || minAmount <= 0 || !url) return null
  try {
    const protocol = new URL(url).protocol
    return protocol === 'https:' || protocol === 'http:' ? { minAmount, url } : null
  } catch {
    return null
  }
}

export function getTopupAmountErrorMessage(
  topupAmount: number,
  minimum: number,
  currencyLabel: string,
  t: (key: string, options?: Record<string, unknown>) => string
): string | null {
  if (
    Number.isFinite(topupAmount) &&
    topupAmount > 0 &&
    Number.isFinite(minimum) &&
    minimum > 0 &&
    topupAmount < minimum
  ) {
    return t('Minimum topup amount: {{amount}}', {
      amount: `${minimum < 1 ? formatWalletInputAmount(minimum) : formatCurrency(minimum)} ${currencyLabel}`,
    })
  }
  return null
}

export function isTopupAmountValidationActive(
  topupAmount: number,
  globalMinimum: number,
  validationTarget: RechargeValidationTarget | null | undefined
): boolean {
  const amountBelowGlobalMinimum =
    Number.isFinite(topupAmount) &&
    topupAmount > 0 &&
    Number.isFinite(globalMinimum) &&
    globalMinimum > 0 &&
    topupAmount < globalMinimum
  return (
    amountBelowGlobalMinimum ||
    (validationTarget === 'amount' && topupAmount <= 0)
  )
}

type PaymentQuoteDisplay = {
  charged_amount: number
  charged_amount_usd: number
  currency: string
  rate_to_usd: number
  rounding_decimals: number
}

export function getPaymentQuoteDisplay(
  method: PaymentMethod,
  topupAmount: number,
  cashback: CashbackThreshold[] = []
): PaymentQuoteDisplay | null {
  const quote = getPaymentMethodDisplayQuote(topupAmount, method, cashback)
  if (!quote) return null
  return {
    charged_amount: quote.chargedAmount,
    charged_amount_usd: quote.chargedAmountUSD,
    currency: quote.currency,
    rate_to_usd: Number(method.rate_to_usd),
    rounding_decimals: Number(method.rounding_decimals),
  }
}

export function shouldShowPaymentMethodQuote(
  unavailable: boolean,
  quote: PaymentQuoteDisplay | null
): quote is PaymentQuoteDisplay {
  return !unavailable && quote !== null
}

export function hasPaymentMethodDisplayConfig(method: PaymentMethod): boolean {
  return getPaymentMethodDisplayQuote(1, method) !== null
}

export function getWaffoPaymentMethod(method: WaffoPayMethod): PaymentMethod {
  return {
    name: method.name,
    type: 'waffo',
    icon: method.icon,
    currency: method.currency,
    rate_to_usd: method.rate_to_usd,
    base_amount_multiplier: method.base_amount_multiplier,
    topup_ratio: method.topup_ratio,
    rounding_decimals: method.rounding_decimals,
    currency_symbol: method.currency_symbol,
  }
}

export function getCashbackDisplayAmount(
  topupAmount: number,
  selectedPaymentMethod: PaymentMethod | undefined,
  cashback: CashbackThreshold[]
): number {
  return (
    getPaymentMethodDisplayQuote(
      topupAmount,
      selectedPaymentMethod ?? { name: '', type: '' },
      cashback
    )?.baseAmountUSD ?? topupAmount
  )
}

export function formatPaymentQuoteAmount(
  quote: PaymentQuoteDisplay,
  currencySymbol?: string
): string {
  const amount = `${getPaymentCurrencyLabel(
    quote.currency,
    currencySymbol
  )}${formatCurrency(quote.charged_amount, quote.rounding_decimals)}`
  return quote.currency === 'USD' ? amount : `~ ${amount}`
}

export function RechargeFormCard({
  topupInfo,
  topupAmount,
  onTopupAmountChange,
  onPaymentMethodSelect,
  selectedPaymentMethod,
  selectedWaffoMethodIndex,
  redemptionCode,
  onRedemptionCodeChange,
  onRedeem,
  redeeming,
  topupLink,
  loading,
  creemProducts,
  enableCreemTopup,
  onCreemProductSelect,
  enableWaffoTopup,
  waffoPayMethods,
  onWaffoMethodSelect,
  enableWaffoPancakeTopup,
  affiliateUser,
  affiliateLink,
  onTransferAffiliate,
  affiliateLoading,
  affiliateComplianceConfirmed = true,
  validationTarget,
  onValidationRequest,
}: RechargeFormCardProps) {
  const { t } = useTranslation()
  const amountInputRef = useRef<HTMLInputElement>(null)
  const lastInputBackendAmountRef = useRef<number | null>(null)
  const [localAmount, setLocalAmount] = useState(() =>
    formatWalletInputAmount(topupAmount)
  )
  const [cryptoExpanded, setCryptoExpanded] = useState(false)
  const standardPaymentMethods = useMemo(
    () =>
      topupInfo?.pay_methods?.filter(
        (method) => !isLegacyCustomMethod(method)
      ) ?? [],
    [topupInfo?.pay_methods]
  )
  const isCryptoExpanded =
    cryptoExpanded && selectedPaymentMethod?.type === 'crypto_direct'

  useEffect(() => {
    if (lastInputBackendAmountRef.current === topupAmount) {
      lastInputBackendAmountRef.current = null
      return
    }
    setLocalAmount(formatWalletInputAmount(topupAmount))
  }, [topupAmount])

  useEffect(() => {
    if (validationTarget === 'amount') {
      amountInputRef.current?.focus()
    }
  }, [validationTarget])

  useEffect(() => {
    if (selectedPaymentMethod?.type !== 'crypto_direct') {
      setCryptoExpanded(false)
    }
  }, [selectedPaymentMethod?.type])

  const handleAmountChange = (value: string) => {
    const sanitizedAmount = sanitizeTopupAmount(value)
    setLocalAmount(sanitizedAmount)
    const backendAmount = getBackendTopupAmount(
      parseTopupAmount(sanitizedAmount)
    )
    lastInputBackendAmountRef.current = backendAmount
    onTopupAmountChange(backendAmount)
  }

  const manualTopup = getManualTopupContact(topupInfo)
  const hasConfigurableTopup =
    topupInfo?.enable_online_topup ||
    topupInfo?.enable_stripe_topup ||
    topupInfo?.enable_yookassa_topup ||
    topupInfo?.enable_nowpayments_topup ||
    enableWaffoTopup ||
    enableWaffoPancakeTopup ||
    standardPaymentMethods.some((method) =>
      [
        'crypto_direct',
        'usdt_trc20_direct',
        'usdt_ton_direct',
        'usdt_solana_direct',
      ].includes(method.type)
    )
  const hasAnyTopup = hasConfigurableTopup || enableCreemTopup || manualTopup
  const hasStandardPaymentMethods = standardPaymentMethods.length > 0
  const availableWaffoMethods = getAvailableWaffoMethods(waffoPayMethods)
  const hasWaffoPaymentMethods = availableWaffoMethods.length > 0
  const minTopup = getMinTopupAmount(topupInfo, selectedPaymentMethod)
  const globalMinTopup = getMinTopupAmount(topupInfo)
  const amountValidationActive = isTopupAmountValidationActive(
    topupAmount,
    globalMinTopup,
    validationTarget
  )
  const redemptionEnabled = topupInfo?.enable_redemption !== false
  const cashbackTiers = getRenderableCashbackTiers(topupInfo?.cashback)
  const hasCashback = hasPositiveCashbackTier(cashbackTiers)
  const cashbackSummary = getCashbackTierSummary(
    getCashbackDisplayAmount(topupAmount, selectedPaymentMethod, cashbackTiers),
    cashbackTiers
  )
  const currencyLabel = getCurrencyLabel()
  const currencyDisplay = getCurrencyDisplay()
  const currencySymbol =
    currencyDisplay.meta.kind === 'tokens' ? '' : currencyDisplay.meta.symbol
  const currentStep = getRechargeStep(
    topupAmount,
    minTopup,
    Boolean(selectedPaymentMethod)
  )

  if (loading) {
    return (
      <Card data-card-hover='false' className='overflow-hidden py-0'>
        <CardContent className='space-y-5 p-3 sm:space-y-6 sm:p-5'>
          <div className='space-y-4 sm:space-y-6'>
            <div className='space-y-3'>
              <Skeleton className='h-5 w-48' />
              <Skeleton className='h-[42px] w-full' />
            </div>
            <div className='space-y-3'>
              <Skeleton className='h-3 w-32' />
              <div className='space-y-2'>
                {['one', 'two', 'three'].map((key) => (
                  <Skeleton key={key} className='h-14 w-full rounded-lg' />
                ))}
              </div>
            </div>
          </div>
          <div className='space-y-3 border-t pt-8'>
            <Skeleton className='h-3 w-24' />
            <div className='flex gap-2'>
              <Skeleton className='h-10 flex-1' />
              <Skeleton className='h-10 w-20' />
            </div>
          </div>
        </CardContent>
      </Card>
    )
  }

  return (
    <Card data-card-hover='false' className='overflow-hidden py-0'>
      <CardContent className='space-y-4 p-3 sm:space-y-6 sm:p-5'>
        <div className='space-y-3'>
          <div className='text-muted-foreground flex items-center gap-2 overflow-x-auto text-xs font-medium whitespace-nowrap'>
            <StepIndicator
              number={1}
              label={t('Amount')}
              active={currentStep === 1}
              complete={currentStep > 1}
            />
            <span aria-hidden='true'>→</span>
            <StepIndicator
              number={2}
              label={t('Payment Method')}
              active={currentStep === 2}
              complete={currentStep > 2}
            />
            <span aria-hidden='true'>→</span>
            <StepIndicator
              number={3}
              label={t('Payment step')}
              active={currentStep === 3}
              complete={currentStep === 3}
            />
          </div>
          <div className='flex items-center justify-between gap-3'>
            <h3 className='text-base leading-6 font-semibold'>
              {t('Top-up amount')}
            </h3>
          </div>
        </div>

        {/* Online Topup Section */}
        {hasAnyTopup ? (
          <div className='space-y-4 sm:space-y-6'>
            {hasConfigurableTopup && (
              <>
                <div className='space-y-2.5 sm:space-y-3'>
                  <div className='grid gap-2 sm:grid-cols-[minmax(0,5fr)_minmax(0,1fr)]'>
                    <Input
                      ref={amountInputRef}
                      id='topup-amount'
                      type='text'
                      inputMode='decimal'
                      value={localAmount}
                      onChange={(e) => handleAmountChange(e.target.value)}
                      min={minTopup}
                      step='any'
                      aria-describedby={
                        amountValidationActive
                          ? 'topup-amount-error'
                          : undefined
                      }
                      aria-invalid={amountValidationActive}
                      placeholder={t('Wallet minimum: {{amount}}', {
                        amount: formatWalletInputAmount(minTopup),
                      })}
                      className={cn(
                        'h-11 text-base font-semibold sm:text-lg',
                        amountValidationActive &&
                          'border-destructive focus-visible:ring-destructive/30'
                      )}
                    />
                    <div
                      aria-label={t('Currency')}
                      className='border-input bg-background text-foreground flex h-11 items-center justify-between rounded-md border px-3 text-sm'
                    >
                      <span className='text-muted-foreground text-xs'>
                        {t('Currency')}
                      </span>
                      <span className='flex items-center gap-1 font-medium'>
                        {currencyLabel}
                        <ChevronDown
                          aria-hidden='true'
                          className='text-muted-foreground size-3.5'
                        />
                      </span>
                    </div>
                  </div>
                  {amountValidationActive && (
                    <p
                      id='topup-amount-error'
                      role='status'
                      aria-live='polite'
                      className='text-destructive text-xs'
                    >
                      {getTopupAmountErrorMessage(
                        topupAmount,
                        globalMinTopup,
                        currencyLabel,
                        t
                      ) ?? t('Enter an amount')}
                    </p>
                  )}
                </div>

                {hasCashback && (
                  <CashbackTierPanel
                    amount={topupAmount}
                    currencyLabel={currencySymbol}
                    summary={cashbackSummary}
                    tiers={cashbackTiers}
                    baseAmountMultiplier={
                      selectedPaymentMethod?.base_amount_multiplier
                    }
                  />
                )}

                <div
                  aria-describedby={
                    validationTarget === 'payment-method'
                      ? 'payment-method-error'
                      : undefined
                  }
                  aria-invalid={validationTarget === 'payment-method'}
                  className='space-y-2.5 rounded-lg sm:space-y-3'
                  role='group'
                >
                  <div className='flex flex-wrap items-center justify-between gap-2'>
                    <Label className='text-base leading-6 font-semibold'>
                      {t('Payment method')}
                    </Label>
                  </div>
                  {validationTarget === 'payment-method' && (
                    <p
                      id='payment-method-error'
                      role='status'
                      aria-live='polite'
                      className='text-destructive text-xs'
                    >
                      {t('Choose a payment method')}
                    </p>
                  )}
                  {hasStandardPaymentMethods && (
                    <div className='space-y-2'>
                      {standardPaymentMethods.map((method) => {
                        // method.min_topup is denominated in the provider's
                        // settlement currency (for example RUB), while
                        // topupAmount is the wallet amount in USD. Do not
                        // compare them locally; the server quote enforces
                        // the exact provider minimum.
                        const hasDisplayConfig =
                          hasPaymentMethodDisplayConfig(method)
                        const methodMinimum =
                          getPaymentMethodMinimumAmount(method)
                        const unavailable =
                          !hasDisplayConfig ||
                          !isPaymentMethodAmountEligible(topupAmount, method)
                        const quote = getPaymentQuoteDisplay(
                          method,
                          topupAmount,
                          cashbackTiers
                        )
                        const cryptoDisclosureIcon = isCryptoExpanded ? (
                          <ChevronUp className='size-4 shrink-0' />
                        ) : (
                          <ChevronDown className='size-4 shrink-0' />
                        )
                        return (
                          <>
                            <Button
                              key={method.type}
                              variant='outline'
                              onClick={() => {
                                if (unavailable) {
                                  if (!hasDisplayConfig) return
                                  if (topupAmount < globalMinTopup) {
                                    onValidationRequest?.()
                                  }
                                  return
                                }
                                if (method.type === 'crypto_direct') {
                                  const selected =
                                    selectedPaymentMethod?.type ===
                                    'crypto_direct'
                                  if (!selected) onPaymentMethodSelect(method)
                                  setCryptoExpanded(
                                    !selected || !isCryptoExpanded
                                  )
                                  return
                                }
                                onPaymentMethodSelect(method)
                              }}
                              aria-disabled={unavailable}
                              aria-expanded={
                                method.type === 'crypto_direct'
                                  ? isCryptoExpanded
                                  : undefined
                              }
                              aria-controls={
                                method.type === 'crypto_direct'
                                  ? `crypto-networks-${method.type}`
                                  : undefined
                              }
                              className={cn(
                                'border-input min-h-[68px] w-full min-w-0 justify-start gap-2.5 rounded-lg px-3 py-2 text-left',
                                unavailable && 'cursor-not-allowed opacity-50',
                                validationTarget === 'payment-method' &&
                                  'border-destructive ring-1 ring-destructive/50'
                              )}
                            >
                              {method.type === 'crypto_direct' ? (
                                cryptoDisclosureIcon
                              ) : (
                                <span
                                  className={cn(
                                    'size-4 shrink-0 rounded-full border',
                                    selectedPaymentMethod?.type ===
                                      method.type &&
                                      'border-primary border-[5px]'
                                  )}
                                />
                              )}
                              {getPaymentIcon(
                                method.type,
                                'h-4 w-4',
                                method.icon,
                                method.name
                              )}
                              <span className='flex min-w-0 flex-1 items-center gap-3'>
                                <span className='min-w-0 flex-1'>
                                  <span className='block truncate font-medium'>
                                    {getPaymentMethodDisplayLabel(method)}
                                  </span>
                                </span>
                                <span className='flex shrink-0 flex-col items-end text-right'>
                                  {shouldShowPaymentMethodQuote(
                                    unavailable,
                                    quote
                                  ) && (
                                    <span className='text-foreground text-base font-semibold tracking-tight tabular-nums sm:text-lg'>
                                      {formatPaymentQuoteAmount(quote)}
                                    </span>
                                  )}
                                  <span className='text-muted-foreground text-xs leading-4'>
                                    {hasDisplayConfig
                                      ? getMethodCommissionLabel(
                                          method.topup_ratio,
                                          t
                                        )
                                      : t('Payment method is unavailable')}
                                  </span>
                                  {methodMinimum !== null &&
                                    topupAmount < methodMinimum && (
                                      <span className='text-muted-foreground text-xs leading-4'>
                                        {t('Wallet minimum: {{amount}}', {
                                          amount:
                                            formatWalletInputAmount(
                                              methodMinimum
                                            ),
                                        })}
                                      </span>
                                    )}
                                </span>
                              </span>
                            </Button>
                            {method.type === 'crypto_direct' &&
                              selectedPaymentMethod?.type === 'crypto_direct' &&
                              isCryptoExpanded && (
                                <div
                                  id={`crypto-networks-${method.type}`}
                                  role='radiogroup'
                                  className='ml-8 grid gap-2'
                                >
                                  {(method.crypto_networks ?? []).map(
                                    (network) => (
                                      <Button
                                        key={network}
                                        type='button'
                                        variant={
                                          selectedPaymentMethod.crypto_network ===
                                          network
                                            ? 'default'
                                            : 'secondary'
                                        }
                                        role='radio'
                                        aria-checked={
                                          selectedPaymentMethod.crypto_network ===
                                          network
                                        }
                                        className='justify-start'
                                        onClick={() =>
                                          onPaymentMethodSelect({
                                            ...method,
                                            crypto_network: network,
                                          })
                                        }
                                      >
                                        <span
                                          className={cn(
                                            'size-4 rounded-full border',
                                            selectedPaymentMethod.crypto_network ===
                                              network &&
                                              'border-primary-foreground border-[5px]'
                                          )}
                                        />
                                        {network === 'SOLANA'
                                          ? 'Solana'
                                          : network}
                                      </Button>
                                    )
                                  )}
                                </div>
                              )}
                          </>
                        )
                      })}
                    </div>
                  )}
                  {!hasStandardPaymentMethods && !hasWaffoPaymentMethods && (
                    <Alert>
                      <AlertDescription>
                        {t(
                          'No payment methods available. Please contact administrator.'
                        )}
                      </AlertDescription>
                    </Alert>
                  )}
                </div>

                {enableWaffoTopup &&
                  hasWaffoPaymentMethods &&
                  onWaffoMethodSelect && (
                    <div className='space-y-2.5 sm:space-y-3'>
                      <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                        {t('Waffo Payment')}
                      </Label>
                      <div className='space-y-1.5'>
                        {availableWaffoMethods.map(({ method, index }) => {
                          // Waffo's minimum is in its configured settlement
                          // currency. Let the server quote validate it
                          // instead of comparing it to the USD wallet input.
                          const paymentMethod = getWaffoPaymentMethod(method)
                          const hasDisplayConfig =
                            hasPaymentMethodDisplayConfig(paymentMethod)
                          const unavailable =
                            !canSelectPaymentMethod(topupAmount) ||
                            !hasDisplayConfig
                          const quote = getPaymentQuoteDisplay(
                            paymentMethod,
                            topupAmount,
                            cashbackTiers
                          )
                          const methodKey = `${method.name}-${method.payMethodType || method.payMethodName || 'default'}`
                          let methodIcon = getPaymentIcon('waffo')
                          if (method.icon) {
                            methodIcon = (
                              <img
                                src={method.icon}
                                alt={method.name}
                                className='h-4 w-4 object-contain'
                              />
                            )
                          }

                          return (
                            <Button
                              key={methodKey}
                              variant='outline'
                              onClick={() => {
                                if (unavailable) {
                                  if (!hasDisplayConfig) return
                                  onValidationRequest?.()
                                  return
                                }
                                onWaffoMethodSelect(method, index)
                              }}
                              aria-disabled={unavailable}
                              aria-label={method.name}
                              className={cn(
                                'min-h-[68px] w-full min-w-0 justify-start gap-2.5 rounded-lg px-3 py-2 text-left',
                                unavailable && 'cursor-not-allowed opacity-50',
                                validationTarget === 'payment-method' &&
                                  'border-destructive ring-1 ring-destructive/50'
                              )}
                            >
                              <span
                                className={cn(
                                  'size-4 shrink-0 rounded-full border',
                                  selectedWaffoMethodIndex === index &&
                                    'border-primary border-[5px]'
                                )}
                              />
                              {methodIcon}
                              <span className='flex min-w-0 flex-1 items-center gap-3'>
                                <span className='min-w-0 flex-1'>
                                  <span className='block truncate font-medium'>
                                    {method.name}
                                  </span>
                                </span>
                                <span className='flex shrink-0 flex-col items-end text-right'>
                                  {shouldShowPaymentMethodQuote(
                                    unavailable,
                                    quote
                                  ) && (
                                    <span className='text-foreground text-base font-semibold tracking-tight tabular-nums sm:text-lg'>
                                      {formatPaymentQuoteAmount(
                                        quote,
                                        method.currency_symbol
                                      )}
                                    </span>
                                  )}
                                  <span className='text-muted-foreground text-xs leading-4'>
                                    {hasDisplayConfig
                                      ? getMethodCommissionLabel(
                                          method.topup_ratio,
                                          t
                                        )
                                      : t('Payment method is unavailable')}
                                  </span>
                                </span>
                              </span>
                            </Button>
                          )
                        })}
                      </div>
                    </div>
                  )}
              </>
            )}
            {manualTopup && (
              <a href={manualTopup.url} target='_blank' rel='noreferrer' className='border-primary/30 bg-primary/5 hover:bg-primary/10 flex min-h-[88px] w-full items-center gap-3 rounded-lg border p-3 text-left transition-colors'>
                <ExternalLink className='text-primary size-4 shrink-0' aria-hidden='true' />
                <span className='min-w-0 flex-1'>
                  <span className='block font-medium'>{t('Large payment without commission')}</span>
                  <span className='text-muted-foreground block text-sm'>
                    {t('From {{amount}} ₽ — contact the manager to arrange an SBP transfer.', { amount: new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 0 }).format(manualTopup.minAmount) })}
                  </span>
                  <span className='text-muted-foreground mt-0.5 block text-xs'>{t('Balance is credited manually after payment confirmation.')}</span>
                </span>
                <ExternalLink className='text-muted-foreground size-4 shrink-0' aria-hidden='true' />
              </a>
            )}
          </div>
        ) : (
          <Alert>
            <AlertDescription>
              {t(
                'Online topup is not enabled. Please use redemption code or contact administrator.'
              )}
            </AlertDescription>
          </Alert>
        )}

        {/* Creem Products Section */}
        {enableCreemTopup &&
          Array.isArray(creemProducts) &&
          creemProducts.length > 0 &&
          onCreemProductSelect && (
            <div className='space-y-2.5 border-t pt-4 sm:space-y-3 sm:pt-6'>
              <Label className='text-muted-foreground text-xs font-medium tracking-wider uppercase'>
                {t('Creem Payment')}
              </Label>
              <CreemProductsSection
                products={creemProducts}
                onProductSelect={onCreemProductSelect}
              />
            </div>
          )}

        {/* Redemption Code Section */}
        {redemptionEnabled ? (
          <div className='space-y-2.5 border-t pt-3 sm:space-y-3 sm:pt-4'>
            <Label
              htmlFor='redemption-code'
              className='text-muted-foreground text-xs font-medium tracking-wider uppercase'
            >
              {t('Promo / bonus code')}
            </Label>
            <div className='grid grid-cols-[minmax(0,1fr)_auto] gap-2'>
              <Input
                id='redemption-code'
                value={redemptionCode}
                onChange={(e) => onRedemptionCodeChange(e.target.value)}
                placeholder={t('Enter a promo or bonus code')}
                className='h-9 min-w-0'
              />
              <Button
                onClick={onRedeem}
                disabled={redeeming}
                variant='outline'
                className='h-9 px-4'
              >
                {redeeming && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
                {t('Apply')}
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              {t('The bonus will be added after verification.')}
            </p>
            {topupLink && (
              <p className='text-muted-foreground text-xs'>
                {t('Need a redemption code?')}{' '}
                <a
                  href={topupLink}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='inline-flex items-center gap-1 underline-offset-4 hover:underline'
                >
                  {t('Get one here')}
                  <ExternalLink className='h-3 w-3' />
                </a>
              </p>
            )}
          </div>
        ) : (
          <Alert className='border-t'>
            <AlertDescription>
              {t(
                'Redemption codes are disabled until the administrator confirms compliance terms.'
              )}
            </AlertDescription>
          </Alert>
        )}

        <AffiliateRewardsCard
          user={affiliateUser}
          affiliateLink={affiliateLink}
          onTransfer={onTransferAffiliate}
          complianceConfirmed={affiliateComplianceConfirmed}
          loading={affiliateLoading}
          embedded
        />
      </CardContent>
    </Card>
  )
}

function StepIndicator(props: {
  number: number
  label: string
  active: boolean
  complete: boolean
}) {
  return (
    <span
      className={cn(
        'flex shrink-0 items-center gap-2',
        props.active && 'text-foreground'
      )}
    >
      <span
        className={cn(
          'flex size-7 items-center justify-center rounded-full border text-sm',
          props.active && 'border-primary bg-primary text-primary-foreground',
          props.complete && !props.active && 'border-primary/60 text-primary'
        )}
      >
        {props.number}
      </span>
      <span>{props.label}</span>
    </span>
  )
}

export function isLegacyCustomMethod(
  method:
    | Pick<PaymentMethod, 'type' | 'name'>
    | Pick<WaffoPayMethod, 'name' | 'payMethodType' | 'payMethodName'>
): boolean {
  const values = [
    method.name,
    'type' in method ? method.type : method.payMethodType,
    'payMethodName' in method ? method.payMethodName : undefined,
  ]
  return values.some((value) => value?.trim().toLowerCase() === 'custom1')
}

export function getAvailableWaffoMethods(
  methods: WaffoPayMethod[] | undefined
): Array<{ method: WaffoPayMethod; index: number }> {
  return (methods ?? [])
    .map((method, index) => ({ method, index }))
    .filter(({ method }) => !isLegacyCustomMethod(method))
}

export function getMethodCommissionLabel(
  topupRatio: number | undefined,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const ratio = Number(topupRatio ?? 1)
  if (!Number.isFinite(ratio) || ratio <= 1) {
    return t('Commission 0%')
  }

  return t('Commission {{percent}}%', {
    percent: formatNumber((ratio - 1) * 100),
  })
}

export function getCashbackTierPosition(
  minAmount: number,
  tierCount: number,
  firstTier: number,
  tierRange: number
): number {
  if (tierCount === 1) return 50
  return Math.min(100, Math.max(0, ((minAmount - firstTier) / tierRange) * 100))
}

export function getCashbackTierRangeLabel(
  tier: CashbackThreshold,
  nextTier: CashbackThreshold | undefined,
  currencyLabel: string,
  t: (key: string, options?: Record<string, unknown>) => string
): string {
  const start = `${currencyLabel}${formatNumber(tier.min_amount)}`
  if (!nextTier) return t('From {{amount}}', { amount: start })
  return `${start}–${currencyLabel}${formatNumber(nextTier.min_amount)}`
}

/**
 * Cashback thresholds are configured against the base amount used by the
 * payment gateway. A method multiplier changes how much of the entered
 * amount reaches that base, so expose thresholds in the same units as the
 * amount field (for example, a $10 base threshold is shown as $5 when the
 * selected method has a multiplier of 2).
 */
export function getCashbackTierDisplayThresholds(
  tiers: CashbackThreshold[],
  baseAmountMultiplier: number | undefined
): CashbackThreshold[] {
  const multiplier = Number(baseAmountMultiplier)
  const safeMultiplier =
    Number.isFinite(multiplier) && multiplier > 0 ? multiplier : 1

  return normalizeCashbackTiers(tiers).map((tier) => ({
    ...tier,
    min_amount: tier.min_amount / safeMultiplier,
  }))
}

export function getCashbackTierTranslate(
  index: number,
  tierCount: number
): string {
  if (tierCount === 1 || (index > 0 && index < tierCount - 1)) return '-50%'
  return index === 0 ? '0%' : '-100%'
}

function CashbackTierPanel(props: {
  amount: number
  currencyLabel: string
  summary: CashbackTierSummary
  tiers: CashbackThreshold[]
  baseAmountMultiplier?: number
}) {
  const { t } = useTranslation()
  const { currencyLabel, summary, tiers, baseAmountMultiplier } = props

  if (!summary.current && !summary.next) {
    return (
      <div className='border-muted bg-muted/20 space-y-1 rounded-lg border px-3 py-3 sm:px-4'>
        <div className='text-base leading-6 font-semibold'>
          {t('Top-up cashback')}
        </div>
        <p className='text-muted-foreground text-xs'>
          {t('Cashback is not configured yet')}
        </p>
      </div>
    )
  }

  const activeTierIndex = summary.current
    ? tiers.findIndex(
        (tier) =>
          tier.min_amount === summary.current?.min_amount &&
          tier.cashback_percent === summary.current?.cashback_percent
      )
    : -1
  const displayTiers = getCashbackTierDisplayThresholds(
    tiers,
    baseAmountMultiplier
  ).map((tier) => ({
    ...tier,
    min_amount: backendAmountToWalletDisplay(tier.min_amount),
  }))

  return (
    <div className='space-y-3'>
      <div className='text-base leading-6 font-semibold'>
        {t('Top-up cashback')}
      </div>
      <div className='overflow-x-auto pb-1'>
        <div
          aria-label={t('Cashback progress')}
          className='bg-muted/70 flex min-w-[360px] overflow-hidden rounded-full'
          role='progressbar'
          aria-valuemin={0}
          aria-valuemax={tiers.length}
          aria-valuenow={activeTierIndex >= 0 ? activeTierIndex + 1 : 0}
        >
          {tiers.map((tier, index) => {
            const displayTier = displayTiers[index]
            const nextTier = displayTiers[index + 1]
            const isActive =
              summary.current?.min_amount === tier.min_amount &&
              summary.current?.cashback_percent === tier.cashback_percent

            return (
              <div
                key={`range-${tier.min_amount}-${tier.cashback_percent}`}
                className={cn(
                  'flex min-h-10 min-w-0 flex-1 items-center justify-center px-3 text-center text-[11px] font-medium leading-tight whitespace-nowrap transition-colors',
                  index > 0 && 'border-l border-background/30',
                  isActive
                    ? 'bg-primary/75 text-primary-foreground shadow-[inset_0_0_0_1px_hsl(var(--primary)/0.35)]'
                    : 'text-muted-foreground hover:bg-muted'
                )}
              >
                {getCashbackTierRangeLabel(
                  displayTier,
                  nextTier,
                  currencyLabel,
                  t
                )}{' '}
                · {tier.cashback_percent}%
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

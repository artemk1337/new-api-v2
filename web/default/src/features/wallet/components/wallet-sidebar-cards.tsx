/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { ChevronLeft, ChevronRight, History, ReceiptText } from 'lucide-react'
import { type ReactNode, useEffect } from 'react'
import { useTranslation } from 'react-i18next'

import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  formatBillingCurrencyFromUSD,
} from '@/lib/currency'
import { formatNumber } from '@/lib/format'

import { useBillingHistory } from '../hooks/use-billing-history'
import { formatCashbackCredit, getPaymentMethodDisplayQuote } from '../lib'
import {
  getPaymentMethodName,
  getTopupSourceLabel,
  getTopupDisplayAmount,
  formatTopupDisplayAmount,
  getStatusConfig,
  getTopupStatusLabel,
  formatTimestamp,
} from '../lib/billing'
import type { CashbackThreshold, PaymentMethod, TopupRecord } from '../types'

interface WalletSummaryCardProps {
  topupAmount: number
  selectedPaymentMethod?: PaymentMethod
  cashback: CashbackThreshold[]
  onPay: () => void
  onPayUnavailable: () => void
  payDisabled: boolean
}

export function WalletSummaryCard(props: WalletSummaryCardProps) {
  const { t } = useTranslation()
  const hasPaymentSelection = isWalletSummaryReady(
    props.topupAmount,
    Boolean(props.selectedPaymentMethod)
  )
  const displayQuote = props.selectedPaymentMethod
    ? getPaymentMethodDisplayQuote(
        props.topupAmount,
        props.selectedPaymentMethod,
        props.cashback
      )
    : null
  const hasPaymentQuote = hasPaymentSelection && displayQuote !== null
  const summaryValues = getPaymentSummaryValues(displayQuote)
  const value = (amount: number) => formatPaymentSummaryAmount(amount)
  let cashbackValue = '—'
  if (hasPaymentQuote) {
    cashbackValue =
      summaryValues.cashbackAmount > 0
        ? `+${formatCashbackCredit(summaryValues.cashbackAmount)} (${formatNumber(summaryValues.cashbackPercent)}%)`
        : '—'
  }

  return (
    <div className='relative rounded-xl'>
      <Card
        data-card-hover='false'
        className='border-primary/20 relative z-0 overflow-hidden py-0'
      >
        <CardHeader className='border-b px-4 py-3 sm:px-5'>
          <div className='flex items-center gap-2'>
            <ReceiptText className='text-muted-foreground size-4' />
            <h2 className='text-sm font-semibold'>{t('Summary')}</h2>
          </div>
        </CardHeader>
        <CardContent className='divide-border/70 divide-y px-4 pt-0 pb-2 sm:px-5'>
          <SummaryRow
            label={t('Top up')}
            value={hasPaymentQuote ? value(summaryValues.topup) : '—'}
            muted={!hasPaymentQuote}
            first
          />
          <SummaryRow
            label={t('Commission')}
            value={hasPaymentQuote ? value(summaryValues.commission) : '—'}
            muted={!hasPaymentQuote}
          />
          <SummaryRow
            label={t('Added to balance')}
            value={hasPaymentQuote ? value(summaryValues.credited) : '—'}
            strong={hasPaymentQuote}
            muted={!hasPaymentQuote}
          />
          <SummaryRow
            label={t('Cashback')}
            value={cashbackValue}
            positive={hasPaymentQuote && summaryValues.cashbackAmount > 0}
            muted={!hasPaymentQuote || summaryValues.cashbackAmount <= 0}
          />
          <div className='relative my-3 h-11 w-full rounded-lg'>
            <Button
              type='button'
              className={
                props.payDisabled
                  ? 'h-full w-full cursor-not-allowed rounded-lg opacity-50'
                  : 'h-full w-full rounded-lg text-base'
              }
              aria-disabled={props.payDisabled}
              onClick={() => {
                if (props.payDisabled) {
                  props.onPayUnavailable()
                  return
                }
                props.onPay()
              }}
            >
              {t('Wallet payment')}
            </Button>
          </div>
        </CardContent>
      </Card>
      {!props.payDisabled && (
        <span
          aria-hidden='true'
          className='wallet-payment-contour-pulse pointer-events-none absolute inset-0 rounded-[inherit] border-2 border-emerald-500/75'
        />
      )}
    </div>
  )
}

/**
 * Summary amounts are accounting values in USD. Format them through the
 * configured wallet display currency so CNY/custom-currency wallets do not
 * silently fall back to a hard-coded dollar sign.
 */
export function formatPaymentSummaryAmount(amountUSD: number): string {
  return formatBillingCurrencyFromUSD(amountUSD, {
    digitsLarge: 2,
    digitsSmall: 2,
    abbreviate: false,
    locale: 'en-US',
  })
}

export function isWalletSummaryReady(
  topupAmount: number,
  hasPaymentMethod: boolean
): boolean {
  return topupAmount > 0 && hasPaymentMethod
}

export function getPaymentSummaryValues(
  quote: Pick<
    NonNullable<ReturnType<typeof getPaymentMethodDisplayQuote>>,
    | 'baseAmountUSD'
    | 'commissionUSD'
    | 'creditedAmountUSD'
    | 'chargedAmountUSD'
    | 'cashbackPercent'
    | 'cashbackAmountUSD'
  > | null
): {
  topup: number
  commission: number
  credited: number
  cashbackPercent: number
  cashbackAmount: number
} {
  if (
    !quote ||
    !Number.isFinite(quote.baseAmountUSD) ||
    quote.baseAmountUSD <= 0 ||
    !Number.isFinite(quote.chargedAmountUSD) ||
    quote.chargedAmountUSD < quote.baseAmountUSD ||
    !Number.isFinite(quote.commissionUSD) ||
    quote.commissionUSD < 0 ||
    !Number.isFinite(quote.creditedAmountUSD) ||
    quote.creditedAmountUSD < 0 ||
    !Number.isFinite(quote.cashbackPercent) ||
    quote.cashbackPercent < 0 ||
    !Number.isFinite(quote.cashbackAmountUSD) ||
    quote.cashbackAmountUSD < 0
  ) {
    return {
      topup: 0,
      commission: 0,
      credited: 0,
      cashbackPercent: 0,
      cashbackAmount: 0,
    }
  }
  return {
    topup: quote.chargedAmountUSD,
    commission: roundSummaryAmount(
      quote.chargedAmountUSD - quote.baseAmountUSD
    ),
    credited: quote.baseAmountUSD,
    cashbackPercent: quote.cashbackPercent,
    cashbackAmount: quote.cashbackAmountUSD,
  }
}

function roundSummaryAmount(value: number): number {
  return Math.round((value + Number.EPSILON) * 1e12) / 1e12
}

export function getSelectedPaymentMethodName(
  method: PaymentMethod | undefined,
  t?: (key: string) => string
): string {
  if (!method) return ''
  return method.name.trim() || getPaymentMethodName(method.type, t)
}

export function getSelectedPaymentMethodSubtitle(
  method: PaymentMethod | undefined,
  t?: (key: string) => string
): string | null {
  if (!method) return null
  const mappedName = getPaymentMethodName(method.type, t)
  if (mappedName === method.type) return null
  return mappedName.toLowerCase() === method.name.trim().toLowerCase()
    ? null
    : mappedName
}

function SummaryRow(props: {
  label: string
  value: string
  muted?: boolean
  strong?: boolean
  positive?: boolean
  first?: boolean
}) {
  let valueClass = 'font-medium'
  if (props.positive) {
    valueClass = 'font-medium text-green-600'
  } else if (props.muted) {
    valueClass = 'text-muted-foreground'
  } else if (props.strong) {
    valueClass = 'font-semibold'
  }

  return (
    <div
      className={`flex items-center justify-between gap-3 text-sm ${
        props.first ? 'pt-0 pb-2.5' : 'py-2.5'
      }`}
    >
      <span className='text-muted-foreground'>{props.label}</span>
      <span className={valueClass}>{props.value}</span>
    </div>
  )
}

export function RecentOperationsCard() {
  const { t } = useTranslation()
  const { records, total, page, loading, handlePageChange } = useBillingHistory(
    { initialPageSize: 5 }
  )
  const totalPages = getTopupHistoryTotalPages(total)

  useEffect(() => {
    if (page > totalPages) {
      handlePageChange(totalPages)
    }
  }, [handlePageChange, page, totalPages])

  let content: ReactNode

  if (loading) {
    content = (
      <div className='space-y-3 py-2'>
        {['one', 'two', 'three'].map((key) => (
          <div key={key} className='flex items-center justify-between gap-3'>
            <div className='space-y-1'>
              <Skeleton className='h-3.5 w-24' />
              <Skeleton className='h-3 w-32' />
            </div>
            <Skeleton className='h-4 w-14' />
          </div>
        ))}
      </div>
    )
  } else if (records.length === 0) {
    content = (
      <div className='text-muted-foreground flex flex-col items-center gap-1 py-6 text-center'>
        <History className='size-5 opacity-50' />
        <p className='text-xs'>{t('No recent operations')}</p>
      </div>
    )
  } else {
    content = (
      <div className='divide-border/60 divide-y'>
        {records.map((record, index) => (
          <RecentOperationRow
            key={record.id}
            record={record}
            first={index === 0}
          />
        ))}
      </div>
    )
  }

  return (
    <Card data-card-hover='false' className='overflow-hidden py-0'>
      <CardHeader className='flex-row items-center justify-between border-b px-4 py-3 sm:px-5'>
        <div className='flex items-center gap-2'>
          <History className='text-muted-foreground size-4' />
          <h2 className='text-sm font-semibold'>{t('Top-up history')}</h2>
        </div>
      </CardHeader>
      <CardContent className='px-4 pt-0 pb-2 sm:px-5'>
        {content}
        {!loading && total > 5 && (
          <div className='border-border/60 mt-2 flex items-center justify-between border-t pt-2'>
            <Button
              variant='ghost'
              size='icon'
              className='size-7'
              onClick={() => handlePageChange(page - 1)}
              disabled={page === 1}
              aria-label={t('Previous page')}
            >
              <ChevronLeft className='size-4' />
            </Button>
            <span className='text-muted-foreground text-xs tabular-nums'>
              {t('Page {{current}} of {{total}}', {
                current: page,
                total: totalPages,
              })}
            </span>
            <Button
              variant='ghost'
              size='icon'
              className='size-7'
              onClick={() => handlePageChange(page + 1)}
              disabled={page === totalPages}
              aria-label={t('Next page')}
            >
              <ChevronRight className='size-4' />
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

export function getTopupHistoryTotalPages(total: number, pageSize = 5): number {
  const normalizedTotal = Number.isFinite(total) ? Math.max(0, total) : 0
  const normalizedPageSize = Math.max(1, Math.floor(pageSize))
  return Math.max(1, Math.ceil(normalizedTotal / normalizedPageSize))
}

function RecentOperationRow(props: { record: TopupRecord; first?: boolean }) {
  const { t } = useTranslation()
  const status = getStatusConfig(props.record.status)
  const amount = getTopupDisplayAmount({
    amount: props.record.amount,
    requestedAmount: props.record.requested_amount,
    money: props.record.money,
    accountingAmountUSD: props.record.accounting_amount_usd,
    paymentBaseAmount: props.record.payment_base_amount,
    paymentCurrency: props.record.payment_currency,
    paymentProvider: props.record.payment_provider,
  })

  return (
    <div
      className={`flex items-center justify-between gap-3 ${
        props.first ? 'pt-0 pb-3' : 'py-3'
      }`}
    >
      <div className='min-w-0'>
        <div className='flex items-center gap-2'>
          <span className='truncate text-sm font-medium'>
            {getTopupSourceLabel(
              props.record.source,
              props.record.payment_method,
              t,
              props.record.payment_method_name
            )}
          </span>
          <StatusBadge
            label={getTopupStatusLabel(props.record.status, t)}
            variant={status.variant}
            size='sm'
          />
        </div>
        <div className='text-muted-foreground mt-0.5 text-xs'>
          {formatTimestamp(props.record.create_time)}
        </div>
      </div>
      <span className='shrink-0 text-sm font-semibold tabular-nums'>
        {formatTopupDisplayAmount(amount)}
      </span>
    </div>
  )
}

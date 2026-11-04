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
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table'

import {
  getDynamicPriceEntries,
  getDynamicPricingSummary,
} from '../lib/dynamic-price'
import { getPricingTierLabel } from '../lib/pricing-tier-label'
import type { PricingModel, TokenUnit } from '../types'
import { DynamicPricingBreakdown } from './dynamic-pricing-breakdown'

export function CataloguePriceRates(props: {
  model: PricingModel
  priceRate: number
  usdExchangeRate: number
  tokenUnit: TokenUnit
  showRechargePrice: boolean
}) {
  const { t } = useTranslation()
  const summary = getDynamicPricingSummary(props.model, {
    tokenUnit: props.tokenUnit,
    showRechargePrice: props.showRechargePrice,
    priceRate: props.priceRate,
    usdExchangeRate: props.usdExchangeRate,
    groupRatioMultiplier: 1,
  })
  if (!summary) return null

  if (summary.isSpecialExpression) {
    return (
      <div className='rounded-lg border p-3'>
        <p className='text-sm font-medium'>{t('Special billing expression')}</p>
        <p className='text-muted-foreground mt-1 text-xs'>
          {t('Unable to parse structured pricing')}
        </p>
        <details className='mt-3 text-xs'>
          <summary className='cursor-pointer'>{t('Raw expression')}</summary>
          <code className='mt-2 block break-all'>{summary.rawExpression}</code>
        </details>
      </div>
    )
  }

  const rows = summary.tiers.map((tier) => ({
    key: tier.label,
    label: getPricingTierLabel(tier, summary.tierCount, t),
    name: /^(step_\d+|base|default)$/i.test(tier.label) ? '' : tier.label,
    entries: getDynamicPriceEntries(tier, {
      tokenUnit: props.tokenUnit,
      showRechargePrice: props.showRechargePrice,
      priceRate: props.priceRate,
      usdExchangeRate: props.usdExchangeRate,
      groupRatioMultiplier: 1,
    }),
  }))
  const secondaryFields = [
    ...new Map(
      rows
        .flatMap((row) => row.entries)
        .filter(
          (entry) =>
            entry.field !== 'inputPrice' && entry.field !== 'outputPrice'
        )
        .map((entry) => [entry.field, entry])
    ).values(),
  ]

  return (
    <section className='min-w-0'>
      <div className='mb-2 flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1'>
        <h3 className='text-sm font-semibold'>{t('Base Price')}</h3>
        <span className='text-muted-foreground text-xs'>
          {t('Per {{unit}} tokens', {
            unit: props.tokenUnit === 'K' ? '1K' : '1M',
          })}
        </span>
      </div>
      <StaticDataTable
        className='rounded-lg border'
        tableClassName='text-xs sm:text-sm'
        headerRowClassName='bg-muted/30 hover:bg-muted/30'
        data={rows}
        getRowKey={(_row, index) => String(index)}
        columns={[
          {
            id: 'condition',
            header: t('Applies to'),
            className: 'text-muted-foreground py-2 text-xs font-medium',
            cellClassName: 'py-2.5 whitespace-nowrap',
            cell: (row) => (
              <div>
                <span>{row.label}</span>
                {row.name && (
                  <span className='text-muted-foreground mt-0.5 block text-xs'>
                    {row.name}
                  </span>
                )}
              </div>
            ),
          },
          ...['inputPrice', 'outputPrice'].map((field) => ({
            id: field,
            header: t(field === 'inputPrice' ? 'Input' : 'Output'),
            className:
              'text-muted-foreground py-2 text-right text-xs font-medium',
            cellClassName:
              'py-2.5 text-right font-mono font-semibold tabular-nums',
            cell: (row: (typeof rows)[number]) =>
              row.entries.find((entry) => entry.field === field)?.formatted ??
              '—',
          })),
          ...secondaryFields.map((field) => ({
            id: field.field,
            header: t(field.shortLabel),
            className:
              'text-muted-foreground py-2 text-right text-xs font-medium',
            cellClassName: 'py-2.5 text-right font-mono tabular-nums',
            cell: (row: (typeof rows)[number]) =>
              row.entries.find((entry) => entry.field === field.field)
                ?.formatted ?? '—',
          })),
        ]}
      />
      {summary.hasRequestRules && (
        <details className='mt-2 rounded-lg border px-3 py-2.5'>
          <summary className='cursor-pointer text-xs font-medium'>
            {t('Conditional multipliers')}
          </summary>
          <DynamicPricingBreakdown billingExpr={props.model.billing_expr} />
        </details>
      )}
    </section>
  )
}

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
import { ChevronRight, Copy } from 'lucide-react'
import { memo } from 'react'
import { useTranslation } from 'react-i18next'

import { useCopyToClipboard } from '@/hooks/use-copy-to-clipboard'
import { getLobeIcon } from '@/lib/lobe-icon'
import { cn } from '@/lib/utils'

import { DEFAULT_TOKEN_UNIT, FILTER_ALL } from '../constants'
import { getDynamicPricingSummary } from '../lib/dynamic-price'
import { isTokenBasedModel } from '../lib/model-helpers'
import {
  formatRequestPriceAtRatio,
  formatTokenPriceAtRatio,
  getDisplayGroupRatio,
} from '../lib/price'
import type { PricingModel, TokenUnit } from '../types'

function normalizeCompactPrice(value: string): string {
  return value
    .replaceAll(/([\d])[\s\u00a0\u202f]+([$€£¥₽₩₹])/gu, '$1$2')
    .replaceAll(/([$€£¥₽₩₹])[\s\u00a0\u202f]+([\d])/gu, '$1$2')
}

function renderHeroPrice(
  label: string,
  current: string,
  base: string,
  unit: string
): React.ReactNode {
  return (
    <div className='flex min-w-[125px] flex-col gap-0.5 leading-none'>
      <span className='flex items-baseline gap-2 whitespace-nowrap'>
        <span className='text-foreground font-mono text-sm font-bold sm:text-base'>
          {label} {current}/{unit}
        </span>
        <span className='font-mono text-sm text-red-600/70 line-through dark:text-red-400/70'>
          {base}
        </span>
      </span>
    </div>
  )
}

export interface ModelCardProps {
  model: PricingModel
  onClick: () => void
  priceRate?: number
  usdExchangeRate?: number
  tokenUnit?: TokenUnit
  showRechargePrice?: boolean
  groupFilter?: string
}

export const ModelCard = memo(function ModelCard(props: ModelCardProps) {
  const { t } = useTranslation()
  const { copyToClipboard } = useCopyToClipboard()
  const tokenUnit = props.tokenUnit ?? DEFAULT_TOKEN_UNIT
  const priceRate = props.priceRate ?? 1
  const usdExchangeRate = props.usdExchangeRate ?? 1
  const showRechargePrice = props.showRechargePrice ?? false
  const isTokenBased = isTokenBasedModel(props.model)
  const hasGroupFilter = Boolean(
    props.groupFilter && props.groupFilter !== FILTER_ALL
  )
  const displayRatio = getDisplayGroupRatio(
    props.model,
    hasGroupFilter ? props.groupFilter : undefined
  )
  const tokenUnitLabel = tokenUnit === 'K' ? '1K' : '1M'
  const modelIconKey = props.model.icon || props.model.vendor_icon
  const modelIcon = modelIconKey ? getLobeIcon(modelIconKey, 28) : null
  const initial = props.model.model_name?.charAt(0).toUpperCase() || '?'
  const isDynamicPricing =
    props.model.billing_mode === 'tiered_expr' &&
    Boolean(props.model.billing_expr)
  const dynamicSummary = isDynamicPricing
    ? getDynamicPricingSummary(props.model, {
        tokenUnit,
        showRechargePrice,
        priceRate,
        usdExchangeRate,
        groupRatioMultiplier: displayRatio,
      })
    : null
  const benefitPercent = Math.max(0, Math.round((1 - displayRatio) * 100))
  const pricingCaption = t('Benefit {{percent}}%', {
    percent: benefitPercent,
  })
  let pricingContent: React.ReactNode

  if (dynamicSummary?.isSpecialExpression) {
    pricingContent = (
      <span className='min-w-0'>
        <span className='text-amber-700 dark:text-amber-300'>
          {t('Special billing expression')}
        </span>
        <code className='text-muted-foreground/70 mt-0.5 line-clamp-1 block font-mono text-[11px] break-all'>
          {dynamicSummary.rawExpression}
        </code>
      </span>
    )
  } else if (dynamicSummary?.primaryEntries.length) {
    pricingContent = (
      <div className='flex flex-col gap-1'>
        {dynamicSummary.primaryEntries.map((entry) => {
          const baseEntry = dynamicSummary.baseEntries.find(
            (candidate) => candidate.key === entry.key
          )
          const hero = renderHeroPrice(
            t(entry.shortLabel),
            normalizeCompactPrice(entry.formatted),
            normalizeCompactPrice(baseEntry?.formatted || entry.formatted),
            tokenUnitLabel
          )
          return <div key={entry.key}>{hero}</div>
        })}
      </div>
    )
  } else if (dynamicSummary) {
    pricingContent = (
      <span className='text-muted-foreground text-xs'>
        {t('Dynamic Pricing')}
      </span>
    )
  } else if (isTokenBased) {
    pricingContent = (
      <div className='flex flex-col gap-1'>
        {renderHeroPrice(
          t('Input'),
          normalizeCompactPrice(
            formatTokenPriceAtRatio(
              props.model,
              'input',
              displayRatio,
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate
            )
          ),
          normalizeCompactPrice(
            formatTokenPriceAtRatio(
              props.model,
              'input',
              1,
              tokenUnit,
              showRechargePrice,
              priceRate,
              usdExchangeRate
            )
          ),
          tokenUnitLabel
        )}
        <div>
          {renderHeroPrice(
            t('Output'),
            normalizeCompactPrice(
              formatTokenPriceAtRatio(
                props.model,
                'output',
                displayRatio,
                tokenUnit,
                showRechargePrice,
                priceRate,
                usdExchangeRate
              )
            ),
            normalizeCompactPrice(
              formatTokenPriceAtRatio(
                props.model,
                'output',
                1,
                tokenUnit,
                showRechargePrice,
                priceRate,
                usdExchangeRate
              )
            ),
            tokenUnitLabel
          )}
        </div>
      </div>
    )
  } else {
    pricingContent = (
      <div className='flex flex-col gap-3'>
        {renderHeroPrice(
          '',
          normalizeCompactPrice(
            formatRequestPriceAtRatio(
              props.model,
              displayRatio,
              showRechargePrice,
              priceRate,
              usdExchangeRate
            )
          ),
          normalizeCompactPrice(
            formatRequestPriceAtRatio(
              props.model,
              1,
              showRechargePrice,
              priceRate,
              usdExchangeRate
            )
          ),
          t('request')
        )}
      </div>
    )
  }

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    copyToClipboard(props.model.model_name || '')
  }

  return (
    <div
      className={cn(
        'group relative flex w-full max-w-[480px] flex-col rounded-xl border p-3 transition-colors sm:p-5',
        'hover:bg-muted/20'
      )}
    >
      {/* Header: icon + name + price + actions */}
      <div className='flex items-start justify-between gap-2.5 sm:gap-3'>
        <div className='flex min-w-0 items-start gap-2.5 sm:gap-3'>
          <div className='bg-muted/40 flex size-9 shrink-0 items-center justify-center rounded-lg sm:size-10 sm:rounded-xl'>
            {modelIcon || (
              <span className='text-muted-foreground text-sm font-bold'>
                {initial}
              </span>
            )}
          </div>
          <div className='min-w-0'>
            <h3 className='text-foreground truncate font-mono text-[15px] leading-tight font-bold'>
              {props.model.model_name}
            </h3>
            <div className='mt-1 flex flex-wrap items-start justify-start gap-x-5 gap-y-3 text-xs'>
              <span className='text-[11px] font-medium text-emerald-600 dark:text-emerald-400'>
                {pricingCaption}
              </span>
              {pricingContent}
            </div>
          </div>
        </div>

        <div className='flex shrink-0 items-center gap-1.5'>
          <button
            type='button'
            onClick={handleCopy}
            className='text-muted-foreground hover:text-foreground hover:bg-muted rounded-md border p-1.5 transition-colors'
            title={t('Copy')}
          >
            <Copy className='size-3.5' />
          </button>
          <button
            type='button'
            onClick={props.onClick}
            className='text-muted-foreground hover:text-foreground hover:bg-muted inline-flex items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors sm:px-2.5 sm:py-1.5'
          >
            {t('Details')}
            <ChevronRight className='size-3.5' />
          </button>
        </div>
      </div>
    </div>
  )
})

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

import { GroupBadge } from '@/components/group-badge'

import {
  formatRetailPrice,
  getComparisonReference,
  getDisplayGroupPrice,
  getGroupPriceAmount,
  getGroupPriceBenefit,
  hasRetailRate,
  isTokenGroupPrice,
  type RetailPriceOptions,
} from '../lib/group-price'
import type { PricingModel } from '../types'

export function ModelPriceSummary(
  props: RetailPriceOptions & {
    model: PricingModel
    groupFilter?: string
    quotaTypeFilter?: string
  }
) {
  const { t } = useTranslation()
  const price = getDisplayGroupPrice(
    props.model,
    props.groupFilter,
    props.quotaTypeFilter
  )

  if (!price || price.unavailable) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Pricing unavailable')}
      </span>
    )
  }

  const tokenBased = isTokenGroupPrice(price)
  const reference = getComparisonReference(props.model)
  const benefit = getGroupPriceBenefit(props.model, price)
  const hasSavings = benefit !== null && benefit > 0
  const variable =
    getGroupPriceAmount(price) === null && price.billing_mode === 'tiered_expr'
  let unit = price.price_unit
  if (tokenBased) unit = props.tokenUnit === 'K' ? '1K' : '1M'
  if (unit === 'request') unit = t('request')
  if (unit === 'seconds') unit = t('seconds')

  return (
    <div className='flex min-w-0 flex-col items-start gap-1.5'>
      <div className='flex flex-wrap items-center gap-2'>
        <GroupBadge group={price.group_name || price.group} size='sm' />
        {hasSavings && (
          <span className='text-xs font-medium text-emerald-700 dark:text-emerald-400'>
            {t('Benefit {{percent}}%', { percent: Math.round(benefit) })}
          </span>
        )}
      </div>
      {variable ? (
        <span className='text-muted-foreground text-xs'>
          {t('Price depends on request conditions')}
        </span>
      ) : (
        <div className='space-y-1 font-mono text-sm tabular-nums'>
          {tokenBased ? (
            <>
              {(['input_usd', 'output_usd'] as const).map((key) => (
                <div
                  key={key}
                  className='flex flex-wrap items-baseline gap-x-2'
                >
                  <span className='font-semibold'>
                    {t(key === 'input_usd' ? 'Input' : 'Output')}{' '}
                    {formatRetailPrice(price[key], props)} / {unit}
                  </span>
                  {hasSavings &&
                    reference &&
                    hasRetailRate(price[key]) &&
                    reference[key] > price[key] && (
                      <del className='text-sm text-red-600/70 dark:text-red-400/70'>
                        {formatRetailPrice(reference[key], props)}
                      </del>
                    )}
                </div>
              ))}
            </>
          ) : (
            <div>
              {formatRetailPrice(price.price_usd, props, false)} / {unit}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

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
  getDisplayGroupPrice,
  getGroupPriceAmount,
  hasTwoContextTiers,
  isTokenGroupPrice,
  type RetailPriceOptions,
} from '../lib/group-price'
import type { PricingModel } from '../types'
import { InputContext } from './input-context'

export function ModelPriceSummary(
  props: RetailPriceOptions & { model: PricingModel; groupFilter?: string }
) {
  const { t } = useTranslation()
  const price = getDisplayGroupPrice(props.model, props.groupFilter)

  if (!price || price.unavailable) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('Pricing unavailable')}
      </span>
    )
  }

  const tokenBased = isTokenGroupPrice(price)
  const variable =
    getGroupPriceAmount(price) === null && price.billing_mode === 'tiered_expr'
  let unit = price.price_unit
  if (tokenBased) unit = props.tokenUnit === 'K' ? '1K' : '1M'
  if (unit === 'request') unit = t('request')
  if (unit === 'seconds') unit = t('seconds')

  return (
    <div className='flex min-w-0 flex-col items-start gap-1.5'>
      <GroupBadge group={price.group_name || price.group} size='sm' />
      {variable ? (
        <span className='text-muted-foreground text-xs'>
          {t('Price depends on request conditions')}
        </span>
      ) : (
        <div className='space-y-1 font-mono text-sm tabular-nums'>
          {tokenBased ? (
            <>
              <div>
                {t('Input')} {formatRetailPrice(price.input_usd, props)} /{' '}
                {unit}
              </div>
              <div>
                {t('Output')} {formatRetailPrice(price.output_usd, props)} /{' '}
                {unit}
              </div>
            </>
          ) : (
            <div>
              {formatRetailPrice(price.price_usd, props, false)} / {unit}
            </div>
          )}
        </div>
      )}
      {price.tiers?.[0] && (
        <div className='text-muted-foreground text-[10px]'>
          <InputContext
            tier={price.tiers[0]}
            name={hasTwoContextTiers(price.tiers) ? 'short' : undefined}
          />
        </div>
      )}
    </div>
  )
}

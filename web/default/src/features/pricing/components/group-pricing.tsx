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
import { GroupBadge } from '@/components/group-badge'

import {
  compareGroupPrices,
  formatRetailPrice,
  getPricingRateFields,
  hasTwoContextTiers,
  type RetailPriceOptions,
} from '../lib/group-price'
import type { PricingGroupPrice, PricingModel, PricingTier } from '../types'
import { InputContext } from './input-context'

function GroupTariff(
  props: RetailPriceOptions & { price: PricingGroupPrice; selected: boolean }
) {
  const { t } = useTranslation()
  const price = props.price
  const rows: PricingTier[] = price.tiers?.length ? price.tiers : [price]
  const fields = getPricingRateFields(rows)
  const twoContextTiers = hasTwoContextTiers(price.tiers)
  const tokenUnit = t(
    props.tokenUnit === 'K' ? 'Per 1K tokens' : 'Per 1M tokens'
  )
  let unit = price.price_unit
  if (unit === 'request') unit = t('request')
  if (unit === 'seconds') unit = t('seconds')
  const unavailable = price.unavailable || fields.length === 0
  const contextName = (index: number): 'short' | 'long' | undefined => {
    if (!twoContextTiers) return undefined
    return index === 0 ? 'short' : 'long'
  }

  return (
    <div className='min-w-0 overflow-hidden rounded-lg border'>
      <div className='bg-muted/20 flex flex-wrap items-center justify-between gap-2 border-b px-3 py-2'>
        <GroupBadge group={price.group_name || price.group} size='sm' />
        {props.selected && (
          <span className='text-muted-foreground text-xs'>
            {t('Selected group')}
          </span>
        )}
      </div>
      {unavailable ? (
        <p className='text-muted-foreground px-3 py-4 text-sm'>
          {t(
            !price.unavailable && price.billing_mode === 'tiered_expr'
              ? 'Price depends on request conditions'
              : 'Pricing unavailable'
          )}
        </p>
      ) : (
        <>
          <div className='divide-y sm:hidden'>
            {rows.map((tier, index) => (
              <div
                key={`${price.group}-${tier.min_input_tokens ?? 0}-${tier.max_input_tokens ?? 'open'}`}
                className='space-y-3 p-3'
              >
                {Boolean(price.tiers?.length) && (
                  <div className='text-muted-foreground text-xs'>
                    <InputContext tier={tier} name={contextName(index)} />
                  </div>
                )}
                <dl className='grid grid-cols-2 gap-x-3 gap-y-4'>
                  {fields.map((field) => (
                    <div key={field.key} className='min-w-0 space-y-1'>
                      <dt className='text-muted-foreground text-xs'>
                        {t(field.label)}
                      </dt>
                      <dd className='space-y-0.5'>
                        <div className='font-mono text-sm font-medium'>
                          {formatRetailPrice(
                            tier[field.key],
                            props,
                            field.key !== 'price_usd'
                          )}
                        </div>
                        <div className='text-muted-foreground text-[10px]'>
                          {field.key === 'price_usd' ? `/ ${unit}` : tokenUnit}
                        </div>
                      </dd>
                    </div>
                  ))}
                </dl>
              </div>
            ))}
          </div>
          <StaticDataTable
            className='hidden rounded-none border-0 sm:block'
            tableClassName='text-sm'
            tableProps={{
              tabIndex: 0,
              'aria-label': price.group_name || price.group,
            }}
            headerRowClassName='hover:bg-transparent'
            data={rows}
            getRowKey={(_, index) => `${price.group}-${index}`}
            columns={[
              ...(price.tiers?.length
                ? [
                    {
                      id: 'tier',
                      header: t('Input context'),
                      cellClassName:
                        'text-muted-foreground whitespace-nowrap py-2.5',
                      cell: (tier: PricingTier, index: number) => (
                        <InputContext tier={tier} name={contextName(index)} />
                      ),
                    },
                  ]
                : []),
              ...fields.map((field) => ({
                id: field.key,
                header: (
                  <div>
                    <div>{t(field.label)}</div>
                    <div className='text-muted-foreground/60 mt-0.5 text-[10px] font-normal'>
                      {field.key === 'price_usd' ? `/ ${unit}` : tokenUnit}
                    </div>
                  </div>
                ),
                className:
                  'text-muted-foreground text-right whitespace-nowrap text-xs',
                cellClassName: 'py-2.5 text-right font-mono',
                cell: (tier: PricingTier) =>
                  formatRetailPrice(
                    tier[field.key],
                    props,
                    field.key !== 'price_usd'
                  ),
              })),
            ]}
          />
        </>
      )}
    </div>
  )
}

export function GroupPricingSection(
  props: RetailPriceOptions & { model: PricingModel; groupFilter?: string }
) {
  const { t } = useTranslation()
  const prices = [...(props.model.group_prices ?? [])].sort(compareGroupPrices)

  if (prices.length === 0) {
    return (
      <p className='text-muted-foreground text-sm'>
        {t('Pricing unavailable')}
      </p>
    )
  }

  return (
    <section className='min-w-0 space-y-3' aria-label={t('Pricing by Group')}>
      {prices.map((price) => (
        <GroupTariff
          key={price.group}
          {...props}
          price={price}
          selected={props.groupFilter === price.group}
        />
      ))}
      {prices.some((price) => price.tiers?.length) && (
        <p className='text-muted-foreground text-xs'>
          {t('Input context includes cached input.')}
        </p>
      )}
    </section>
  )
}

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

import {
  formatRetailPrice,
  hasRetailRate,
  type RetailPriceOptions,
} from '../lib/group-price'
import type { PricingModel } from '../types'

export function OfficialApiPrice(
  props: RetailPriceOptions & { model: PricingModel }
) {
  const { t, i18n } = useTranslation()
  const official = props.model.official_price
  if (
    !official ||
    official.price_unit !== 'million_tokens' ||
    !hasRetailRate(official.input_usd) ||
    !hasRetailRate(official.output_usd)
  ) {
    return null
  }

  let source: URL
  try {
    source = new URL(official.source)
  } catch {
    return null
  }
  if (source.protocol !== 'https:' || source.username || source.password) {
    return null
  }
  const updated = new Date(official.updated_at * 1000)
  if (!Number.isFinite(updated.getTime())) return null
  // Official USD reference prices are independent of platform funding discounts.
  const options = { tokenUnit: props.tokenUnit }

  return (
    <section className='text-muted-foreground space-y-2 text-xs'>
      <h3 className='font-medium'>{t('Official API price')}</h3>
      <p>
        {t('Input')} {formatRetailPrice(official.input_usd, options)} ·{' '}
        {t('Output')} {formatRetailPrice(official.output_usd, options)} ·{' '}
        {t(props.tokenUnit === 'K' ? 'Per 1K tokens' : 'Per 1M tokens')}
      </p>
      <p className='flex flex-wrap gap-x-3 gap-y-1'>
        <a
          href={source.href}
          target='_blank'
          rel='noopener noreferrer'
          className='underline underline-offset-2'
        >
          {source.hostname}
        </a>
        <span>
          {t('Updated')} {updated.toLocaleDateString(i18n.resolvedLanguage)}
        </span>
      </p>
    </section>
  )
}

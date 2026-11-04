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
import type { PricingGroupPrice } from '../types'

export function GroupToolPricing(
  props: RetailPriceOptions & { price: PricingGroupPrice }
) {
  const { t } = useTranslation()
  const tools = [
    { key: 'web_search', label: t('Web search') },
    { key: 'web_search_preview', label: t('Web search preview') },
    { key: 'file_search', label: t('File search') },
  ].filter((tool) => hasRetailRate(props.price.tools?.[tool.key]))

  if (props.price.unavailable || tools.length === 0) return null

  return (
    <div className='space-y-3 border-t px-3 py-3'>
      <div className='text-muted-foreground flex items-center justify-between gap-3 text-xs'>
        <span>{t('Tool prices')}</span>
        <span>{t('Per tool call')}</span>
      </div>
      <dl className='space-y-2 text-sm'>
        {tools.map((tool) => (
          <div key={tool.key} className='flex justify-between gap-3'>
            <dt>{tool.label}</dt>
            <dd className='font-mono'>
              {formatRetailPrice(props.price.tools?.[tool.key], props, false)}
            </dd>
          </div>
        ))}
      </dl>
    </div>
  )
}

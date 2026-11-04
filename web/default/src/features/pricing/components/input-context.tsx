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

import type { PricingTier } from '../types'

export function InputContext(props: {
  tier: PricingTier
  name?: 'short' | 'long'
}) {
  const { t, i18n } = useTranslation()
  const min = props.tier.min_input_tokens ?? 0
  const max = props.tier.max_input_tokens
  const format = (value: number) => value.toLocaleString(i18n.resolvedLanguage)
  let range = t('Default')
  if (max != null && min === 0) {
    range = t('Up to {{limit}} tokens', { limit: format(max) })
  } else if (max != null) {
    range = t('More than {{min}} and up to {{max}} tokens', {
      min: format(min - 1),
      max: format(max),
    })
  } else if (min > 0) {
    range = t('More than {{limit}} tokens', { limit: format(min - 1) })
  }

  return (
    <div className='space-y-0.5'>
      {props.name && (
        <div className='text-foreground text-xs font-medium'>
          {t(props.name === 'short' ? 'Short context' : 'Long context')}
        </div>
      )}
      <div>{range}</div>
    </div>
  )
}

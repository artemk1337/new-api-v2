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
import { WalletCards } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Skeleton } from '@/components/ui/skeleton'
import { formatBillingCurrencyFromUSD } from '@/lib/currency'
import { useSystemConfigStore } from '@/stores/system-config-store'

import type { UserWalletData } from '../types'

interface WalletStatsCardProps {
  user: UserWalletData | null
  loading?: boolean
}

export function WalletStatsCard(props: WalletStatsCardProps) {
  const { t } = useTranslation()
  const quotaPerUnit = useSystemConfigStore(
    (state) => state.config.currency.quotaPerUnit
  )
  if (props.loading) {
    return (
      <div className='overflow-hidden rounded-lg border'>
        <div className='px-4 py-4 sm:px-5 sm:py-5'>
          <Skeleton className='h-3.5 w-28' />
          <Skeleton className='mt-2 h-8 w-36' />
        </div>
      </div>
    )
  }

  return (
    <div className='bg-primary/[0.04] ring-primary/10 overflow-hidden rounded-lg border border-primary/25 ring-1'>
      <div className='px-4 py-4 sm:px-5 sm:py-5'>
        <div className='flex items-center gap-2'>
          <WalletCards className='text-muted-foreground/60 size-3.5 shrink-0' />
          <div className='text-primary truncate text-xs font-semibold tracking-wider uppercase'>
            {t('Current Balance')}
          </div>
        </div>
        <div className='text-foreground mt-1.5 text-3xl leading-none font-semibold tracking-tight break-all tabular-nums'>
          {formatBillingCurrencyFromUSD(
            (props.user?.quota ?? 0) / Math.max(quotaPerUnit, 1),
            {
              digitsLarge: 2,
              digitsSmall: 2,
              abbreviate: false,
              locale: 'en-US',
            }
          )}
        </div>
      </div>
    </div>
  )
}

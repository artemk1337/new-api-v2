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
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'

import type { DifferencesMap, RatioType } from '../types'
import { formatPricingNumber } from './pricing-format'
import { getPricingSyncPricePreview } from './pricing-sync-preview'
import {
  formatSyncValue,
  getSyncFieldLabel,
  type ResolutionsMap,
} from './upstream-ratio-sync-helpers'
import { usePricingSyncSnapshots } from './use-pricing-sync-snapshots'

type PricingSyncReviewProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  differences: DifferencesMap
  resolutions: ResolutionsMap
  isPending: boolean
  onApply: () => Promise<boolean>
}

export function PricingSyncReview(props: PricingSyncReviewProps) {
  const { t } = useTranslation()
  const { snapshots, quotaPerUnit } = usePricingSyncSnapshots()

  return (
    <AlertDialog open={props.open} onOpenChange={props.onOpenChange}>
      <AlertDialogContent className='max-h-[85dvh] max-w-[calc(100%-2rem)] sm:max-w-2xl'>
        <AlertDialogHeader>
          <AlertDialogTitle>{t('Review price changes')}</AlertDialogTitle>
          <AlertDialogDescription>
            {t(
              'Only selected values will be saved. Automatic update preferences will stay unchanged.'
            )}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <div className='min-h-0 space-y-4 overflow-y-auto'>
          {Object.entries(props.resolutions).map(([model, fields]) => (
            <section key={model} className='overflow-hidden rounded-lg border'>
              <h4 className='bg-muted/40 border-b px-3 py-2 text-sm font-medium break-all'>
                {model}
              </h4>
              <dl className='divide-y px-3'>
                {Object.entries(fields)
                  .filter(
                    ([field]) =>
                      field !== 'billing_mode' || !('billing_expr' in fields)
                  )
                  .map(([field, value]) => {
                    const current =
                      props.differences[model]?.[field as RatioType]?.current
                    const hasDefault =
                      field === 'image_ratio' ||
                      field === 'audio_ratio' ||
                      field === 'audio_completion_ratio'
                    let currentLabel = hasDefault
                      ? t('Default value: {{value}}', { value: '×1' })
                      : t('Not Set')
                    if (current !== null && current !== undefined) {
                      currentLabel = formatSyncValue(field, current)
                    }
                    let proposedLabel = formatSyncValue(field, value)
                    const preview = getPricingSyncPricePreview(
                      snapshots.get(model),
                      props.differences[model] ?? {},
                      fields,
                      field as RatioType,
                      null,
                      quotaPerUnit
                    )
                    const suffix =
                      field === 'model_price' ? ` / ${t('request')}` : ''
                    if (preview.current !== null) {
                      currentLabel = `$${formatPricingNumber(preview.current)}${suffix}`
                    }
                    if (preview.proposed !== null) {
                      proposedLabel = `$${formatPricingNumber(preview.proposed)}${suffix}`
                    }
                    return (
                      <div
                        key={field}
                        className='flex flex-wrap items-center justify-between gap-2 py-3 text-sm'
                      >
                        <dt>{getSyncFieldLabel(field, t)}</dt>
                        <dd className='flex min-w-0 items-center gap-2'>
                          <span
                            className='text-muted-foreground max-w-48 truncate font-mono'
                            title={String(current ?? '')}
                          >
                            {currentLabel}
                          </span>
                          <span
                            className='text-muted-foreground'
                            aria-hidden='true'
                          >
                            →
                          </span>
                          <span
                            className='max-w-64 truncate font-mono font-medium'
                            title={String(value)}
                          >
                            {proposedLabel}
                          </span>
                        </dd>
                      </div>
                    )
                  })}
              </dl>
            </section>
          ))}
          <p className='text-muted-foreground text-xs'>
            {t('Base prices in USD per 1M tokens, before group multipliers.')}
          </p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Switching billing type also removes incompatible pricing settings.'
            )}
          </p>
        </div>
        <AlertDialogFooter>
          <AlertDialogCancel disabled={props.isPending}>
            {t('Cancel')}
          </AlertDialogCancel>
          <AlertDialogAction
            disabled={props.isPending}
            onClick={async () => {
              if (await props.onApply()) props.onOpenChange(false)
            }}
          >
            {t('Apply Sync')}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

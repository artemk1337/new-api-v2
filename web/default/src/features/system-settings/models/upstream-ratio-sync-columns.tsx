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
import type { ColumnDef } from '@tanstack/react-table'
import { AlertTriangle } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import type { RatioType } from '../types'
import type { ModelPricingSnapshot } from './model-pricing-snapshots'
import { formatPricingNumber } from './pricing-format'
import { getPricingSyncPricePreview } from './pricing-sync-preview'
import {
  formatSyncValue,
  getPreferredSyncField,
  getSyncFieldLabel,
  isSelectableUpstreamValue,
  type ModelRow,
  type ResolutionsMap,
} from './upstream-ratio-sync-helpers'

export type PricingSyncFieldRow = ModelRow & {
  field: RatioType
}

export function useUpstreamRatioSyncColumns(
  upstreamNames: string[],
  resolutions: ResolutionsMap,
  isDisabled: boolean,
  snapshots: Map<string, ModelPricingSnapshot>,
  quotaPerUnit: number,
  onSelectValue: (
    model: string,
    ratioType: RatioType,
    value: number | string,
    sourceName: string
  ) => void,
  onUnselectValue: (model: string, ratioType: RatioType) => void,
  onBulkSelect: (upstreamName: string, rows: PricingSyncFieldRow[]) => void,
  onBulkUnselect: (upstreamName: string, rows: PricingSyncFieldRow[]) => void,
  autoSources: Array<{ id: number; name: string }> = [],
  onEnableAuto?: (model: string, channelID: number) => void
): ColumnDef<PricingSyncFieldRow>[] {
  const { t } = useTranslation()
  const [autoSelectionReset, setAutoSelectionReset] = useState(0)

  return useMemo<ColumnDef<PricingSyncFieldRow>[]>(() => {
    const upstreamName = upstreamNames[0] ?? ''
    const getPreview = (row: PricingSyncFieldRow) =>
      getPricingSyncPricePreview(
        snapshots.get(row.model),
        row.ratioTypes,
        resolutions[row.model] ?? {},
        row.field,
        upstreamName,
        quotaPerUnit
      )
    const canSelect = (row: PricingSyncFieldRow) => {
      if (
        getPreferredSyncField(row.ratioTypes, row.field, upstreamName) !==
          row.field ||
        !isSelectableUpstreamValue(
          row.ratioTypes[row.field]?.upstreams?.[upstreamName]
        )
      ) {
        return false
      }
      const preview = getPreview(row)
      return (
        preview.current === null ||
        preview.proposed === null ||
        preview.current !== preview.proposed
      )
    }
    const formatPrice = (field: RatioType, price: number) => {
      const value = `$${formatPricingNumber(price)}`
      return field === 'model_price' ? `${value} / ${t('request')}` : value
    }

    const columns: ColumnDef<PricingSyncFieldRow>[] = [
      {
        accessorKey: 'model',
        header: t('Model'),
        cell: ({ row }) => {
          return (
            <div className='flex min-w-0 items-center gap-2'>
              <span className='truncate font-medium' title={row.original.model}>
                {row.original.model}
              </span>
              {row.original.billingConflict && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger>
                      <AlertTriangle className='h-3.5 w-3.5 shrink-0 text-amber-500' />
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>
                        {t(
                          'This model has both fixed price and ratio billing conflicts'
                        )}
                      </p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
            </div>
          )
        },
      },
      {
        id: 'parameter',
        header: t('Parameter'),
        cell: ({ row }) => {
          const field = row.original.field
          let hint = ''
          if (field === 'image_ratio' || field === 'audio_ratio') {
            hint = t('Relative to the input token rate')
          }
          if (field === 'audio_completion_ratio') {
            hint = t('Relative to the input audio rate')
          }
          const difference = row.original.ratioTypes[field]
          const current = difference?.current
          const source = difference?.upstreams?.[upstreamName]
          const hasDefault =
            field === 'image_ratio' ||
            field === 'audio_ratio' ||
            field === 'audio_completion_ratio'
          let currentText = t('Not Set')
          if (current !== null && current !== undefined) {
            currentText = formatSyncValue(field, current)
          } else if (hasDefault) {
            currentText = t('Default value: {{value}}', { value: '×1' })
          }
          let sourceText = t('Not Set')
          if (source === 'same') {
            sourceText = t('Same as Local')
          } else if (source !== null && source !== undefined) {
            sourceText = formatSyncValue(field, source)
          }
          return (
            <div className='space-y-1'>
              <p className='text-sm'>{getSyncFieldLabel(field, t)}</p>
              <details className='text-muted-foreground text-xs'>
                <summary className='cursor-pointer'>
                  {t('Technical details')}
                </summary>
                <div className='mt-2 space-y-1'>
                  <p>
                    {t('Your current value')}: {currentText}
                  </p>
                  <p>
                    {t('Source value')}: {sourceText}
                  </p>
                  {hint && <p>{hint}</p>}
                </div>
              </details>
            </div>
          )
        },
      },
      {
        id: 'current',
        header: t('Current tariff'),
        cell: ({ row }) => {
          const field = row.original.field
          if (field === 'billing_expr' || field === 'billing_mode') {
            return (
              <span className='text-muted-foreground text-sm'>
                {t('Price depends on the billing formula')}
              </span>
            )
          }
          const preview = getPreview(row.original)
          if (preview.current === null) {
            return (
              <span className='text-muted-foreground text-sm'>
                {t('Price preview unavailable')}
              </span>
            )
          }
          return (
            <span className='font-mono text-sm tabular-nums'>
              {formatPrice(field, preview.current)}
            </span>
          )
        },
      },
      {
        id: 'upstream',
        header: t('Tariff after import'),
        cell: ({ row }) => {
          const { field, ratioTypes } = row.original
          if (
            getPreferredSyncField(ratioTypes, field, upstreamName) !== field
          ) {
            return <span className='text-muted-foreground'>—</span>
          }
          if (field === 'billing_expr' || field === 'billing_mode') {
            return (
              <span className='text-muted-foreground text-sm'>
                {t('Price depends on the billing formula')}
              </span>
            )
          }
          const diff = ratioTypes[field]
          const value = diff?.upstreams?.[upstreamName]
          const preview = getPreview(row.original)
          if (preview.proposed === null) {
            return (
              <span className='text-muted-foreground text-sm'>
                {value === 'same'
                  ? t('Same as Local')
                  : t('Price preview unavailable')}
              </span>
            )
          }
          let effect = ''
          if (
            preview.current !== null &&
            preview.current === preview.proposed
          ) {
            effect = t('No price change')
          } else if (preview.current !== null && preview.current > 0) {
            const percent = formatPricingNumber(
              Math.round(
                Math.abs((preview.proposed / preview.current - 1) * 100) * 100
              ) / 100
            )
            effect =
              preview.proposed < preview.current
                ? t('{{percent}}% cheaper', { percent })
                : t('{{percent}}% more expensive', { percent })
          }
          return (
            <div className='space-y-1'>
              <div className='flex min-w-0 items-center gap-2'>
                <span className='font-mono text-sm tabular-nums'>
                  {formatPrice(field, preview.proposed)}
                </span>
                {diff?.confidence?.[upstreamName] === false && (
                  <TooltipProvider>
                    <Tooltip>
                      <TooltipTrigger>
                        <AlertTriangle className='h-3.5 w-3.5 shrink-0 text-amber-500' />
                      </TooltipTrigger>
                      <TooltipContent>
                        <p>
                          {t('This data may be unreliable, use with caution')}
                        </p>
                      </TooltipContent>
                    </Tooltip>
                  </TooltipProvider>
                )}
              </div>
              {effect && (
                <p className='text-muted-foreground text-xs'>{effect}</p>
              )}
            </div>
          )
        },
      },
      {
        id: 'selection',
        header: ({ table }) => {
          const rows = table
            .getFilteredRowModel()
            .rows.map((row) => row.original)
          const selectable = rows.filter(canSelect)
          const selectedCount = selectable.filter(
            (row) =>
              resolutions[row.model]?.[row.field] ===
              row.ratioTypes[row.field]?.upstreams?.[upstreamName]
          ).length
          return (
            <div className='flex items-center gap-2'>
              {selectable.length > 0 && (
                <Checkbox
                  aria-label={t('Select all')}
                  checked={selectedCount === selectable.length}
                  indeterminate={
                    selectedCount > 0 && selectedCount < selectable.length
                  }
                  disabled={isDisabled}
                  onCheckedChange={(checked) => {
                    if (checked) onBulkSelect(upstreamName, selectable)
                    else onBulkUnselect(upstreamName, selectable)
                  }}
                />
              )}
              <span>{t('Take this price')}</span>
            </div>
          )
        },
        cell: ({ row }) => {
          const item = row.original
          if (!canSelect(item)) {
            return <span className='text-muted-foreground'>—</span>
          }
          const value = item.ratioTypes[item.field]?.upstreams?.[
            upstreamName
          ] as number | string
          return (
            <Checkbox
              aria-label={`${t('Take this price')}: ${item.model}, ${getSyncFieldLabel(item.field, t)}`}
              checked={resolutions[item.model]?.[item.field] === value}
              disabled={isDisabled}
              onCheckedChange={(checked) => {
                if (checked) {
                  onSelectValue(item.model, item.field, value, upstreamName)
                } else {
                  onUnselectValue(item.model, item.field)
                }
              }}
            />
          )
        },
      },
    ]

    if (onEnableAuto) {
      columns.splice(3, 0, {
        id: 'enable_auto',
        header: t('Automatic update'),
        cell: ({ row }) => (
          <Select
            key={`${row.original.model}-${autoSelectionReset}`}
            items={[
              { value: 'general', label: t('General rule') },
              ...autoSources.map((source) => ({
                value: `channel:${source.id}`,
                label: source.name,
              })),
            ]}
            onValueChange={(value) => {
              if (!value) return
              const [, channel] = String(value).split(':')
              onEnableAuto(row.original.model, channel ? Number(channel) : 0)
              setAutoSelectionReset((current) => current + 1)
            }}
            disabled={isDisabled}
          >
            <SelectTrigger className='min-w-40'>
              <SelectValue placeholder={t('Enable auto')} />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectItem value='general'>{t('General rule')}</SelectItem>
                {autoSources.map((source) => (
                  <SelectItem key={source.id} value={`channel:${source.id}`}>
                    {source.name}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        ),
      })
    }
    return columns
  }, [
    upstreamNames,
    resolutions,
    isDisabled,
    snapshots,
    quotaPerUnit,
    onSelectValue,
    onUnselectValue,
    onBulkSelect,
    onBulkUnselect,
    autoSources,
    onEnableAuto,
    autoSelectionReset,
    t,
  ])
}

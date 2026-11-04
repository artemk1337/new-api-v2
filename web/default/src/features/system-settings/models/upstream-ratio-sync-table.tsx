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
import { Loader2, Search } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTablePagination,
  DataTableView,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type { DifferencesMap, RatioType } from '../types'
import { RATIO_TYPE_OPTIONS } from './constants'
import { getPricingSyncPricePreview } from './pricing-sync-preview'
import {
  useUpstreamRatioSyncColumns,
  type PricingSyncFieldRow,
} from './upstream-ratio-sync-columns'
import {
  getDisplaySyncFields,
  getOrderedRatioTypes,
  getPreferredSyncField,
  isSelectableUpstreamValue,
  RATIO_SYNC_FIELDS,
  type ModelRow,
  type ResolutionsMap,
} from './upstream-ratio-sync-helpers'
import { usePricingSyncSnapshots } from './use-pricing-sync-snapshots'

type UpstreamRatioSyncTableProps = {
  differences: DifferencesMap
  resolutions: ResolutionsMap
  isDisabled: boolean
  isSyncing: boolean
  hasChecked: boolean
  onSelectValue: (
    model: string,
    ratioType: RatioType,
    value: number | string,
    sourceName: string
  ) => void
  onUnselectValue: (model: string, ratioType: RatioType) => void
  autoSources?: Array<{ id: number; name: string }>
  onEnableAuto?: (model: string, channelID: number) => void
  onBulkEnableAuto?: (models: string[]) => void
}

export function UpstreamRatioSyncTable({
  differences,
  resolutions,
  isDisabled,
  isSyncing,
  hasChecked,
  onSelectValue,
  onUnselectValue,
  autoSources = [],
  onEnableAuto,
  onBulkEnableAuto,
}: UpstreamRatioSyncTableProps) {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [ratioTypeFilter, setRatioTypeFilter] = useState<string>('')
  const [sourceName, setSourceName] = useState('')
  const { snapshots, quotaPerUnit } = usePricingSyncSnapshots()

  const dataSource = useMemo<ModelRow[]>(() => {
    return Object.entries(differences).map(([model, ratioTypes]) => {
      const hasPrice = 'model_price' in ratioTypes
      const hasOtherRatio = RATIO_SYNC_FIELDS.some((rt) => rt in ratioTypes)
      return {
        key: model,
        model,
        ratioTypes,
        billingConflict: hasPrice && hasOtherRatio,
      }
    })
  }, [differences])

  const filteredData = useMemo(() => {
    let data = dataSource

    if (search.trim()) {
      const lower = search.toLowerCase()
      data = data.filter((row) => row.model.toLowerCase().includes(lower))
    }

    if (ratioTypeFilter && ratioTypeFilter !== '__all__') {
      data = data.filter((row) => ratioTypeFilter in row.ratioTypes)
    }

    return data
  }, [dataSource, search, ratioTypeFilter])

  const upstreamNames = useMemo(() => {
    const set = new Set<string>()
    dataSource.forEach((row) => {
      getOrderedRatioTypes(row.ratioTypes).forEach((ratioType) => {
        Object.keys(row.ratioTypes[ratioType]?.upstreams || {}).forEach(
          (name) => set.add(name)
        )
      })
    })
    return [...set]
  }, [dataSource])

  const selectedSource = upstreamNames.includes(sourceName)
    ? sourceName
    : upstreamNames[0]

  const fieldRows = useMemo<PricingSyncFieldRow[]>(
    () =>
      filteredData.flatMap((row) =>
        getDisplaySyncFields(row.ratioTypes, ratioTypeFilter).map((field) => ({
          ...row,
          key: `${row.model}:${field}`,
          field,
        }))
      ),
    [filteredData, ratioTypeFilter]
  )

  const handleBulkSelect = useCallback(
    (upstream: string, rows: PricingSyncFieldRow[]) => {
      rows.forEach((row) => {
        const value = row.ratioTypes[row.field]?.upstreams?.[upstream]
        const preview = getPricingSyncPricePreview(
          snapshots.get(row.model),
          row.ratioTypes,
          resolutions[row.model] ?? {},
          row.field,
          upstream,
          quotaPerUnit
        )
        if (preview.current !== null && preview.current === preview.proposed) {
          return
        }
        if (
          getPreferredSyncField(row.ratioTypes, row.field, upstream) ===
            row.field &&
          isSelectableUpstreamValue(value)
        ) {
          onSelectValue(
            row.model,
            row.field,
            value as number | string,
            upstream
          )
        }
      })
    },
    [onSelectValue, snapshots, resolutions, quotaPerUnit]
  )

  const handleBulkUnselect = useCallback(
    (upstream: string, rows: PricingSyncFieldRow[]) => {
      rows.forEach((row) => {
        const value = row.ratioTypes[row.field]?.upstreams?.[upstream]
        if (
          isSelectableUpstreamValue(value) &&
          resolutions[row.model]?.[row.field] === value
        ) {
          onUnselectValue(row.model, row.field)
        }
      })
    },
    [onUnselectValue, resolutions]
  )

  const columns = useUpstreamRatioSyncColumns(
    selectedSource ? [selectedSource] : [],
    resolutions,
    isDisabled,
    snapshots,
    quotaPerUnit,
    onSelectValue,
    onUnselectValue,
    handleBulkSelect,
    handleBulkUnselect,
    autoSources,
    onEnableAuto
  )

  const { table } = useDataTable({
    data: fieldRows,
    columns,
    getRowId: (row) => row.key,
    initialPagination: { pageIndex: 0, pageSize: 10 },
    withFilteredRowModel: false,
    withSortedRowModel: false,
    withFacetedRowModel: false,
  })

  if (dataSource.length === 0) {
    if (isSyncing) {
      return (
        <div className='flex h-64 flex-col items-center justify-center gap-3 rounded-md border'>
          <Loader2 className='text-muted-foreground h-8 w-8 animate-spin' />
          <p className='text-muted-foreground text-sm'>
            {t('Fetching upstream prices...')}
          </p>
        </div>
      )
    }

    return (
      <div className='flex h-64 items-center justify-center rounded-md border'>
        <div className='text-center'>
          <p className='text-muted-foreground text-sm'>
            {hasChecked
              ? t('No upstream price differences found')
              : t(
                  'Check prices to compare your current settings with upstream sources.'
                )}
          </p>
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('Select sync channels to compare prices')}
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t('Base prices in USD per 1M tokens, before group multipliers.')}
      </p>
      <div className='bg-muted/30 flex flex-col gap-3 rounded-lg border p-3 sm:flex-row sm:items-center sm:justify-between'>
        <div className='space-y-1'>
          <p className='text-sm font-medium'>{t('Compare source')}</p>
          <p className='text-muted-foreground text-xs'>
            {t(
              'Choose an upstream source to compare with your current prices.'
            )}
          </p>
        </div>
        <Select
          items={upstreamNames.map((name) => ({ value: name, label: name }))}
          value={selectedSource ?? null}
          onValueChange={(value) => value && setSourceName(value)}
          disabled={isDisabled}
        >
          <SelectTrigger
            className='w-full sm:w-64'
            aria-label={t('Compare source')}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              {upstreamNames.map((name) => (
                <SelectItem key={name} value={name}>
                  {name}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
      </div>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-1/2 left-2 h-4 w-4 -translate-y-1/2' />
          <Input
            placeholder={t('Search model name...')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            disabled={isDisabled}
            className='ps-8'
          />
        </div>
        <Select
          items={[
            { value: '__all__', label: t('All Types') },
            ...RATIO_TYPE_OPTIONS.map((option) => ({
              value: option.value,
              label: t(option.label),
            })),
          ]}
          value={ratioTypeFilter}
          onValueChange={(v) => v !== null && setRatioTypeFilter(v)}
          disabled={isDisabled}
        >
          <SelectTrigger className='w-full sm:w-56'>
            <SelectValue placeholder={t('Filter by price field')} />
          </SelectTrigger>
          <SelectContent alignItemWithTrigger={false}>
            <SelectGroup>
              <SelectItem value='__all__'>{t('All Types')}</SelectItem>
              {RATIO_TYPE_OPTIONS.map((option) => (
                <SelectItem key={option.value} value={option.value}>
                  {t(option.label)}
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        {onBulkEnableAuto && filteredData.length > 0 && (
          <Button
            variant='secondary'
            disabled={isDisabled}
            onClick={() =>
              onBulkEnableAuto(filteredData.map((row) => row.model))
            }
          >
            {t('Enable auto for shown models')}
          </Button>
        )}
      </div>

      {onEnableAuto && (
        <div className='flex flex-wrap items-center gap-2 text-sm'>
          <span className='text-muted-foreground'>
            {t(
              'Enable automatic updates for a model using the general rule or a selected source.'
            )}
          </span>
        </div>
      )}

      <DataTableView
        table={table}
        containerClassName='rounded-md'
        tableContainerClassName='overflow-x-auto'
        tableClassName={
          onEnableAuto
            ? 'table-fixed min-w-[1040px]'
            : 'table-fixed min-w-[680px]'
        }
        colgroup={
          <colgroup>
            <col className='w-1/5' />
            <col className={onEnableAuto ? 'w-[15%]' : 'w-1/4'} />
            <col className='w-[15%]' />
            {onEnableAuto && <col className='w-1/5' />}
            <col className={onEnableAuto ? 'w-[15%]' : 'w-1/4'} />
            <col className='w-[15%]' />
          </colgroup>
        }
        getColumnClassName={() => 'align-top whitespace-normal'}
        getRowClassName={() => 'align-top'}
        emptyContent={t('No results found')}
        emptyCellClassName='h-24 text-center'
      />

      <DataTablePagination table={table} />
    </div>
  )
}

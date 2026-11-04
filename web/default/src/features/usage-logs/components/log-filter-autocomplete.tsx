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
import { Autocomplete } from '@base-ui/react/autocomplete'
import { useInfiniteQuery, useQueryClient } from '@tanstack/react-query'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { useDebounce } from '@/hooks/use-debounce'
import { ROLE } from '@/lib/roles'

import { getLogFilterOptions } from '../api'
import {
  logSuggestionQueryKey,
  logSuggestionScope,
  type LogFilterActor,
  type LogFilterField,
} from '../lib/suggestions'
import type { GetLogsParams, LogFilterOption } from '../types'
import { LogsFilterInput } from './logs-filter-toolbar'

export type LogFilterAutocompleteProps = {
  actor: LogFilterActor
  field: LogFilterField
  filters: GetLogsParams
  placeholder: string
  value: string
  masked?: boolean
  onValueChange: (value: string) => void
  onApply: () => void
}

export function LogFilterAutocomplete(props: LogFilterAutocompleteProps) {
  if (props.actor.role < ROLE.ADMIN || props.masked) {
    if (props.field === 'proxy_supplier_id' && props.actor.role < ROLE.ADMIN) {
      return null
    }
    return (
      <LogsFilterInput
        aria-label={props.placeholder}
        placeholder={props.placeholder}
        type={props.masked ? 'password' : 'text'}
        autoComplete={props.masked ? 'new-password' : 'off'}
        value={props.value}
        readOnly={props.field === 'proxy_supplier_id'}
        onChange={(event) => props.onValueChange(event.target.value)}
        onKeyDown={(event) => {
          if (event.key === 'Enter') props.onApply()
        }}
      />
    )
  }
  return (
    <LogFilterSuggestions
      key={`${props.actor.id}:${props.actor.role}:${props.field}`}
      {...props}
    />
  )
}

function LogFilterSuggestions(props: LogFilterAutocompleteProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const supplier = props.field === 'proxy_supplier_id'
  const initialSupplierText =
    props.value === '0'
      ? t('No recorded supplier')
      : props.value && `#${props.value}`
  const [supplierDraft, setSupplierDraft] = useState({
    source: props.value,
    text: initialSupplierText,
    query: '',
  })
  const currentSupplier =
    supplierDraft.source === props.value
      ? supplierDraft
      : { source: props.value, text: initialSupplierText, query: '' }
  const rawQuery = supplier ? currentSupplier.query : props.value
  const queryText = useDebounce(rawQuery, 300)
  const scope = logSuggestionScope(props.filters, props.field)
  const queryKey = logSuggestionQueryKey(
    props.actor,
    props.field,
    queryText,
    scope
  )
  const [open, setOpen] = useState(false)
  const highlighted = useRef<LogFilterOption | undefined>(undefined)
  const query = useInfiniteQuery({
    queryKey,
    initialPageParam: null as string | null,
    queryFn: ({ pageParam, signal }) =>
      getLogFilterOptions({
        field: props.field,
        query: queryText,
        cursor: pageParam,
        filters: scope,
        signal,
      }),
    getNextPageParam: (page) => page.next_cursor ?? undefined,
    enabled: open && rawQuery === queryText,
    retry: false,
  })
  const waiting = rawQuery !== queryText
  const items: LogFilterOption[] =
    supplier && rawQuery === ''
      ? [
          { value: '', label: t('All suppliers') },
          { value: '0', label: t('No recorded supplier') },
        ]
      : []
  const seen = new Set(items.map((item) => item.value))
  if (!waiting) {
    for (const page of query.data?.pages ?? []) {
      for (const item of page.items) {
        if (!seen.has(item.value)) {
          seen.add(item.value)
          items.push(item)
        }
      }
    }
  }
  let inputValue = props.value
  if (supplier) {
    inputValue = currentSupplier.text
    if (!currentSupplier.query && props.value) {
      inputValue =
        items.find((item) => item.value === props.value)?.label ?? inputValue
    }
  }

  return (
    <Autocomplete.Root
      items={items}
      filter={null}
      mode='none'
      value={inputValue}
      open={open}
      openOnInputClick
      autoHighlight='always'
      submitOnItemClick={false}
      itemToStringValue={(item) => (supplier ? item.label : item.value)}
      onItemHighlighted={(item) => {
        highlighted.current = item
      }}
      onOpenChange={(nextOpen) => {
        setOpen(nextOpen)
        if (!nextOpen) void queryClient.cancelQueries({ queryKey, exact: true })
      }}
      onValueChange={(value, details) => {
        if (details.reason === 'item-press') return
        void queryClient.cancelQueries({ queryKey, exact: true })
        if (supplier) {
          setSupplierDraft({ source: '', text: value, query: value })
          props.onValueChange('')
        } else {
          props.onValueChange(value)
        }
      }}
    >
      <Autocomplete.Input
        render={<LogsFilterInput />}
        aria-label={props.placeholder}
        placeholder={props.placeholder}
        autoComplete='off'
        onKeyDown={(event) => {
          if (event.key === 'Escape' && open) event.stopPropagation()
          if (event.key !== 'Enter') return
          if (supplier && waiting) {
            event.preventBaseUIHandler()
            event.preventDefault()
            event.stopPropagation()
            return
          }
          if (
            open &&
            items.some((item) => item.value === highlighted.current?.value)
          ) {
            event.stopPropagation()
            return
          }
          props.onApply()
        }}
      />
      <Autocomplete.Portal>
        <Autocomplete.Positioner
          sideOffset={4}
          className='pointer-events-auto z-100'
        >
          <Autocomplete.Popup className='bg-popover text-popover-foreground w-(--anchor-width) max-w-(--available-width) overflow-hidden rounded-lg border shadow-md'>
            <Autocomplete.List className='max-h-64 overflow-y-auto p-1'>
              {(item: LogFilterOption, index: number) => (
                <Autocomplete.Item
                  key={item.value}
                  value={item}
                  index={index}
                  className='data-highlighted:bg-accent data-highlighted:text-accent-foreground flex cursor-default items-center justify-between gap-2 rounded-sm px-2 py-1.5 text-sm outline-none'
                  onClick={() => {
                    if (supplier) {
                      setSupplierDraft({
                        source: item.value,
                        text: item.label,
                        query: '',
                      })
                    }
                    props.onValueChange(item.value)
                  }}
                >
                  <span className='min-w-0 truncate'>{item.label}</span>
                  {item.label !== item.value && item.value && (
                    <span className='text-muted-foreground shrink-0 text-xs'>
                      {item.value}
                    </span>
                  )}
                </Autocomplete.Item>
              )}
            </Autocomplete.List>
            {(waiting || query.isPending) && (
              <Autocomplete.Status className='text-muted-foreground px-3 py-2 text-xs'>
                {t('Loading...')}
              </Autocomplete.Status>
            )}
            {!waiting && query.isError && (
              <div className='space-y-1 px-2 pb-2 text-xs'>
                <p role='alert'>{t('Failed to load filter options')}</p>
                <Button
                  type='button'
                  variant='ghost'
                  size='sm'
                  onClick={() => void query.refetch()}
                >
                  {t('Retry')}
                </Button>
              </div>
            )}
            {!waiting && query.isSuccess && items.length === 0 && (
              <Autocomplete.Empty className='text-muted-foreground px-3 py-2 text-xs'>
                {t('No results found.')}
              </Autocomplete.Empty>
            )}
            {!waiting && query.hasNextPage && (
              <Button
                type='button'
                variant='ghost'
                className='w-full rounded-none'
                disabled={query.isFetchingNextPage}
                onMouseDown={(event) => event.preventDefault()}
                onClick={() => void query.fetchNextPage()}
              >
                {query.isFetchingNextPage ? t('Loading...') : t('Load more')}
              </Button>
            )}
          </Autocomplete.Popup>
        </Autocomplete.Positioner>
      </Autocomplete.Portal>
    </Autocomplete.Root>
  )
}

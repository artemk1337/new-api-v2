import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  forwardRef,
  useCallback,
  useEffect,
  useId,
  useImperativeHandle,
  useMemo,
  useState,
} from 'react'
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { getPricingSyncConfig, getPricingSyncModelState } from '../api'
import type { PricingSyncModelPreference } from '../types'
import {
  reconcilePricingSyncSourceDraft,
  type PricingSyncSourceDraft,
} from './pricing-sync-source-draft'

type PricingSyncModelSourceProps = {
  modelName: string
  disabled?: boolean
}

export type PricingSyncModelSourceHandle = {
  getDraft: () => PricingSyncModelPreference | null
  markDraftSaved: () => void
}

export const PricingSyncModelSource = forwardRef<
  PricingSyncModelSourceHandle,
  PricingSyncModelSourceProps
>(function PricingSyncModelSource(props, ref) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const selectID = useId()
  const [draft, setDraft] = useState<PricingSyncSourceDraft>({
    modelName: props.modelName,
    value: 'manual',
    dirty: false,
  })
  const configQuery = useQuery({
    queryKey: ['pricing-sync-config'],
    queryFn: getPricingSyncConfig,
  })
  const stateQuery = useQuery({
    queryKey: ['pricing-sync-model-state', props.modelName],
    queryFn: () => getPricingSyncModelState(props.modelName),
    enabled: props.modelName.trim().length > 0,
  })

  useEffect(() => {
    const state = stateQuery.data?.data
    let serverValue: string | undefined
    if (state?.model_name !== props.modelName) {
      serverValue = undefined
    } else if (state.mode === 'channel') {
      serverValue = `channel:${state.channel_id}`
    } else {
      serverValue = state.mode
    }
    setDraft((current) =>
      reconcilePricingSyncSourceDraft(current, props.modelName, serverValue)
    )
  }, [props.modelName, stateQuery.data])

  const sources = useMemo(
    () =>
      configQuery.data?.data.sources.filter((source) => source.enabled) ?? [],
    [configQuery.data]
  )
  const options = useMemo(
    () => [
      { value: 'manual', label: t('Manual price') },
      { value: 'general', label: t('General rule') },
      ...sources.map((source) => ({
        value: `channel:${source.channel_id}`,
        label: t('Channel #{{id}}', { id: source.channel_id }),
      })),
    ],
    [sources, t]
  )
  const state = stateQuery.data?.data
  const statusLabel = state
    ? {
        ready: t('Ready'),
        conflict: t('Conflict'),
        stale: t('Stale'),
        unavailable: t('Unavailable'),
      }[state.status]
    : ''
  const markDraftSaved = useCallback(() => {
    setDraft((current) => ({ ...current, dirty: false }))
    queryClient.invalidateQueries({
      queryKey: ['pricing-sync-model-state', props.modelName],
    })
  }, [props.modelName, queryClient])

  useImperativeHandle(
    ref,
    () => ({
      getDraft: () => {
        if (!draft.dirty || !props.modelName.trim()) return null
        const [mode, channel] = draft.value.split(':')
        return {
          model_name: props.modelName,
          mode: mode as PricingSyncModelPreference['mode'],
          channel_id: channel ? Number(channel) : 0,
        }
      },
      markDraftSaved,
    }),
    [draft, markDraftSaved, props.modelName]
  )

  if (!props.modelName.trim()) return null

  return (
    <div className='space-y-1'>
      <label htmlFor={selectID} className='text-sm font-medium'>
        {t('Price synchronization source')}
      </label>
      <Select
        items={options}
        value={draft.modelName === props.modelName ? draft.value : 'manual'}
        disabled={props.disabled}
        onValueChange={(next) => {
          if (!next) return
          setDraft({ modelName: props.modelName, value: next, dirty: true })
        }}
      >
        <SelectTrigger id={selectID}>
          <SelectValue />
        </SelectTrigger>
        <SelectContent alignItemWithTrigger={false}>
          <SelectGroup>
            <SelectItem value='manual'>{t('Manual price')}</SelectItem>
            <SelectItem value='general'>{t('General rule')}</SelectItem>
            {sources.map((source) => (
              <SelectItem
                key={source.channel_id}
                value={`channel:${source.channel_id}`}
              >
                {t('Channel #{{id}}', { id: source.channel_id })}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
      {state && (
        <div className='text-muted-foreground space-y-0.5 text-xs'>
          <p>
            {t('Status')}: {statusLabel}
          </p>
          {state.conflict_details && (
            <p>
              {t('Details')}: {state.conflict_details}
            </p>
          )}
          {state.last_applied_at && (
            <p>
              {t('Last updated:')}{' '}
              {new Date(state.last_applied_at * 1000).toLocaleString(
                i18n.language
              )}
            </p>
          )}
        </div>
      )}
    </div>
  )
})

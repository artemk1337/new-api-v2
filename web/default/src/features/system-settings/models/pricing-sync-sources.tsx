import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import type {
  PricingSyncConfig,
  PricingSyncSource,
  PricingSyncStrategy,
  UpstreamChannel,
} from '../types'
import {
  DEFAULT_ENDPOINT,
  ENDPOINT_OPTIONS,
  MODELS_DEV_PRESET_ENDPOINT,
  MODELS_DEV_PRESET_ID,
  OFFICIAL_CHANNEL_BASE_URL,
  OFFICIAL_CHANNEL_ENDPOINT,
  OFFICIAL_CHANNEL_ID,
  OPENROUTER_CHANNEL_TYPE,
  OPENROUTER_ENDPOINT,
} from './constants'

const intervalOptions = [
  { value: '0', label: 'No automatic update' },
  { value: '60', label: '1 minute' },
  { value: '600', label: '10 minutes' },
  { value: '1800', label: '30 minutes' },
  { value: '3600', label: '1 hour' },
]

type PricingSyncSourcesProps = {
  channels: UpstreamChannel[]
  value: PricingSyncConfig
  disabled?: boolean
  onChange: (value: PricingSyncConfig) => void
  onSave: () => void
}

function getDefaultEndpoint(channel: UpstreamChannel) {
  if (channel.id === OFFICIAL_CHANNEL_ID) {
    return `${OFFICIAL_CHANNEL_BASE_URL}${OFFICIAL_CHANNEL_ENDPOINT}`
  }
  if (channel.id === MODELS_DEV_PRESET_ID) return MODELS_DEV_PRESET_ENDPOINT
  if (channel.type === OPENROUTER_CHANNEL_TYPE) return OPENROUTER_ENDPOINT
  return DEFAULT_ENDPOINT
}

export function PricingSyncSources(props: PricingSyncSourcesProps) {
  const { t } = useTranslation()
  const sourceByID = useMemo(
    () =>
      new Map(props.value.sources.map((source) => [source.channel_id, source])),
    [props.value.sources]
  )

  const updateSource = (
    channel: UpstreamChannel,
    patch: Partial<PricingSyncSource>
  ) => {
    const current = sourceByID.get(channel.id) ?? {
      channel_id: channel.id,
      enabled: false,
      endpoint: getDefaultEndpoint(channel),
      interval_seconds: 0,
    }
    const next = { ...current, ...patch }
    const sources = props.value.sources.filter(
      (source) => source.channel_id !== channel.id
    )
    props.onChange({ ...props.value, sources: [...sources, next] })
  }

  const strategyOptions: Array<{ value: PricingSyncStrategy; label: string }> =
    [
      { value: 'highest', label: t('Use highest price') },
      { value: 'lowest', label: t('Use lowest price') },
      { value: 'average', label: t('Use average price') },
    ]

  return (
    <section className='space-y-4 rounded-lg border p-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <h3 className='font-medium'>{t('Select Sync Channels')}</h3>
          <p className='text-muted-foreground text-sm'>
            {t('Configure upstream price synchronization')}
          </p>
        </div>
        <Button onClick={props.onSave} disabled={props.disabled}>
          {t('Save settings')}
        </Button>
      </div>
      <div className='bg-muted/40 flex flex-col gap-2 rounded-md p-3 sm:flex-row sm:items-center sm:justify-between'>
        <div>
          <p className='text-sm font-medium'>{t('Price conflict rule')}</p>
          <p className='text-muted-foreground text-sm'>
            {t('When selected sources report different comparable prices')}
          </p>
        </div>
        <div className='shrink-0'>
          <Select
            items={strategyOptions}
            value={props.value.strategy}
            disabled={props.disabled}
            onValueChange={(value) =>
              value &&
              props.onChange({
                ...props.value,
                strategy: value as PricingSyncStrategy,
              })
            }
          >
            <SelectTrigger className='w-52'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent alignItemWithTrigger={false}>
              <SelectGroup>
                {strategyOptions.map((option) => (
                  <SelectItem key={option.value} value={option.value}>
                    {option.label}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className='overflow-x-auto rounded-md border'>
        <table className='w-full min-w-[760px] text-sm'>
          <thead className='bg-muted/50 text-muted-foreground text-left'>
            <tr>
              <th className='w-12 p-3' />
              <th className='p-3'>{t('Name')}</th>
              <th className='p-3'>{t('Base URL')}</th>
              <th className='p-3'>{t('Sync Endpoint')}</th>
              <th className='p-3'>{t('Automatic update')}</th>
            </tr>
          </thead>
          <tbody>
            {props.channels.map((channel) => {
              const source = sourceByID.get(channel.id)
              const enabled = source?.enabled ?? false
              const endpoint = source?.endpoint ?? getDefaultEndpoint(channel)
              return (
                <tr key={channel.id} className='border-t'>
                  <td className='p-3'>
                    <Checkbox
                      checked={enabled}
                      disabled={props.disabled}
                      onCheckedChange={(checked) =>
                        updateSource(channel, { enabled: !!checked })
                      }
                    />
                  </td>
                  <td className='p-3 font-medium'>{channel.name}</td>
                  <td
                    className='text-muted-foreground max-w-72 truncate p-3 font-mono text-xs'
                    title={channel.base_url}
                  >
                    {channel.base_url}
                  </td>
                  <td className='p-3'>
                    <div className='flex gap-2'>
                      <Select
                        items={ENDPOINT_OPTIONS.map((option) => ({
                          value: option.value,
                          label: option.label,
                        }))}
                        value={
                          ENDPOINT_OPTIONS.some(
                            (option) => option.value === endpoint
                          )
                            ? endpoint
                            : 'custom'
                        }
                        disabled={props.disabled || !enabled}
                        onValueChange={(value) =>
                          value &&
                          updateSource(channel, {
                            endpoint: value === 'custom' ? '' : value,
                          })
                        }
                      >
                        <SelectTrigger className='h-8 w-36'>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            {ENDPOINT_OPTIONS.map((option) => (
                              <SelectItem
                                key={option.value}
                                value={option.value}
                              >
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                      {!ENDPOINT_OPTIONS.some(
                        (option) => option.value === endpoint
                      ) && (
                        <Input
                          className='h-8 w-40 font-mono text-xs'
                          value={source?.endpoint ?? endpoint}
                          placeholder={t('/your/endpoint')}
                          disabled={props.disabled || !enabled}
                          onChange={(event) =>
                            updateSource(channel, {
                              endpoint: event.target.value,
                            })
                          }
                        />
                      )}
                    </div>
                  </td>
                  <td className='p-3'>
                    <Select
                      items={intervalOptions.map((option) => ({
                        value: option.value,
                        label: t(option.label),
                      }))}
                      value={String(source?.interval_seconds ?? 0)}
                      disabled={props.disabled || !enabled}
                      onValueChange={(value) =>
                        value &&
                        updateSource(channel, {
                          interval_seconds: Number(value),
                        })
                      }
                    >
                      <SelectTrigger className='h-8 w-40'>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          {intervalOptions.map((option) => (
                            <SelectItem key={option.value} value={option.value}>
                              {t(option.label)}
                            </SelectItem>
                          ))}
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </section>
  )
}

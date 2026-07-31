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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Code2, Palette } from 'lucide-react'
import { useEffect, useId, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  formatRateLimitDuration,
  getRateLimitActivationRefreshDelay,
  getRateLimitDuration,
  getRateLimitDurationUnit,
  getRateLimitPeriodState,
  parseRateLimitDuration,
  rateLimitDurationActivationUpdate,
  shouldSaveRateLimitDuration,
  type RateLimitDurationUnit,
} from './rate-limit-duration'
import { RateLimitVisualEditor } from './rate-limit-visual-editor'

const isValidJSON = (value: string | undefined) => {
  if (!value || value.trim() === '') return true
  try {
    const parsed = JSON.parse(value)
    if (typeof parsed !== 'object' || Array.isArray(parsed)) {
      return false
    }
    for (const [, val] of Object.entries(parsed)) {
      if (!Array.isArray(val) || val.length !== 2) return false
      if (typeof val[0] !== 'number' || typeof val[1] !== 'number') return false
      if (val[0] < 0 || val[1] < 1) return false
      if (val[0] > 2147483647 || val[1] > 2147483647) return false
    }
    return true
  } catch {
    return false
  }
}

const createRateLimitSchema = (t: (key: string) => string) =>
  z.object({
    ModelRequestRateLimitEnabled: z.boolean(),
    ModelRequestRateLimitDuration: z
      .string()
      .refine((value) => parseRateLimitDuration(value) !== null, {
        message: t('Enter a positive whole number'),
      }),
    ModelRequestRateLimitCount: z.number().min(0).max(100000000),
    ModelRequestRateLimitSuccessCount: z.number().min(1).max(100000000),
    ModelRequestRateLimitGroup: z
      .string()
      .optional()
      .refine(isValidJSON, {
        message: t('Invalid JSON format or values out of allowed range'),
      }),
  })

type RateLimitFormValues = z.infer<ReturnType<typeof createRateLimitSchema>>

type RateLimitSectionProps = {
  defaultValues: RateLimitFormValues & {
    ModelRequestRateLimitDurationActivationAt: number
    ModelRequestRateLimitDurationActive: boolean
    ModelRequestRateLimitDurationActivated: boolean
    ModelRequestRateLimitDurationStaged: boolean
    ModelRequestRateLimitDurationMinutes: number
  }
}

export function RateLimitSection({ defaultValues }: RateLimitSectionProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const updateOption = useUpdateOption()
  const [useVisualEditor, setUseVisualEditor] = useState(true)
  const durationInputId = useId()
  const durationActivationId = useId()

  const rateLimitSchema = createRateLimitSchema(t)

  const form = useForm<RateLimitFormValues>({
    resolver: zodResolver(rateLimitSchema),
    mode: 'onChange', // Enable real-time validation
    defaultValues,
  })

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const durationValue = form.watch('ModelRequestRateLimitDuration')
  const periodState = getRateLimitPeriodState(
    defaultValues.ModelRequestRateLimitDuration,
    defaultValues.ModelRequestRateLimitDurationMinutes,
    defaultValues.ModelRequestRateLimitDurationActivated,
    defaultValues.ModelRequestRateLimitDurationActive,
    defaultValues.ModelRequestRateLimitDurationStaged,
    durationValue
  )
  let durationActivationDescription = t(
    'Activation is one-way and only safe after all replicas are updated. Check the staged period before activating it.'
  )
  if (defaultValues.ModelRequestRateLimitDurationActive) {
    durationActivationDescription = t(
      'The staged period is active and cannot be deactivated.'
    )
  } else if (periodState.activationPending) {
    durationActivationDescription = t(
      'Activation is scheduled and cannot be cancelled.'
    )
  } else if (
    periodState.isDraft ||
    !defaultValues.ModelRequestRateLimitDurationStaged
  ) {
    durationActivationDescription = t('Save the period before activating it.')
  }
  const activationTime =
    defaultValues.ModelRequestRateLimitDurationActivationAt > 0
      ? new Date(
          defaultValues.ModelRequestRateLimitDurationActivationAt * 1000
        ).toLocaleString(i18n.resolvedLanguage)
      : null

  useEffect(() => {
    if (!periodState.activationPending) return

    const initialDelay = getRateLimitActivationRefreshDelay(
      defaultValues.ModelRequestRateLimitDurationActivationAt,
      Date.now()
    )
    if (initialDelay === null) return

    let cancelled = false
    let timer: number

    const refresh = async () => {
      await queryClient.invalidateQueries({
        queryKey: ['system-options'],
        refetchType: 'active',
      })
      if (cancelled) return

      const retryDelay = getRateLimitActivationRefreshDelay(
        defaultValues.ModelRequestRateLimitDurationActivationAt,
        Date.now(),
        true
      )
      if (retryDelay !== null) {
        timer = window.setTimeout(refresh, retryDelay)
      }
    }

    timer = window.setTimeout(refresh, initialDelay)

    return () => {
      cancelled = true
      window.clearTimeout(timer)
    }
  }, [
    defaultValues.ModelRequestRateLimitDurationActivationAt,
    periodState.activationPending,
    queryClient,
  ])

  const onSubmit = async (values: RateLimitFormValues) => {
    const updates = Object.entries(values).filter(([key, value]) => {
      if (key === 'ModelRequestRateLimitDuration') {
        return shouldSaveRateLimitDuration(
          value as string,
          defaultValues.ModelRequestRateLimitDuration,
          defaultValues.ModelRequestRateLimitDurationStaged
        )
      }
      return value !== defaultValues[key as keyof RateLimitFormValues]
    })

    for (const [key, value] of updates) {
      await updateOption.mutateAsync({ key, value: value ?? '' })
    }
  }

  return (
    <SettingsSection title={t('Rate Limiting')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save rate limits'
          />
          <FormField
            control={form.control}
            name='ModelRequestRateLimitEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable rate limiting')}</FormLabel>
                  <FormDescription>
                    {t(
                      'This controls model request rate limiting. Web/API route throttling is configured by environment variables and may still return 429.'
                    )}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <div className='grid gap-4 md:grid-cols-3'>
            <FormField
              control={form.control}
              name='ModelRequestRateLimitDuration'
              render={({ field }) => {
                const parsedDuration = parseRateLimitDuration(field.value)
                const fallbackDuration = getRateLimitDuration(
                  defaultValues.ModelRequestRateLimitDuration,
                  defaultValues.ModelRequestRateLimitDurationMinutes
                )
                const duration = parsedDuration ?? {
                  value: 0,
                  unit: getRateLimitDurationUnit(
                    field.value,
                    fallbackDuration.unit
                  ),
                }

                return (
                  <FormItem>
                    <FormLabel htmlFor={durationInputId}>
                      {t('Limit period')}
                    </FormLabel>
                    <div className='flex items-center gap-2'>
                      <FormControl>
                        <Input
                          id={durationInputId}
                          type='number'
                          min={1}
                          step={1}
                          value={parsedDuration?.value ?? ''}
                          onChange={(e) => {
                            if (e.target.value === '') {
                              field.onChange('')
                              return
                            }
                            field.onChange(
                              formatRateLimitDuration({
                                value: Number(e.target.value),
                                unit: duration.unit,
                              })
                            )
                          }}
                        />
                      </FormControl>
                      <Select
                        value={duration.unit}
                        onValueChange={(value) =>
                          field.onChange(
                            formatRateLimitDuration({
                              value: duration.value,
                              unit: value as RateLimitDurationUnit,
                            })
                          )
                        }
                      >
                        <SelectTrigger
                          aria-label={t('Limit period unit')}
                          className='w-28'
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent alignItemWithTrigger={false}>
                          <SelectGroup>
                            <SelectItem value='s'>{t('seconds')}</SelectItem>
                            <SelectItem value='m'>{t('minutes')}</SelectItem>
                            <SelectItem value='h'>{t('hours')}</SelectItem>
                          </SelectGroup>
                        </SelectContent>
                      </Select>
                    </div>
                    <FormDescription>
                      {t('Time window for rate limiting')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )
              }}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max requests per period')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={0}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 0)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Including failed requests, 0 = unlimited')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name='ModelRequestRateLimitSuccessCount'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Max successful requests')}</FormLabel>
                  <FormControl>
                    <div className='flex items-center gap-2'>
                      <Input
                        type='number'
                        min={1}
                        max={100000000}
                        step={1}
                        {...field}
                        onChange={(e) =>
                          field.onChange(Number.parseInt(e.target.value) || 1)
                        }
                      />
                      <span className='text-muted-foreground text-sm'>
                        {t('times')}
                      </span>
                    </div>
                  </FormControl>
                  <FormDescription>
                    {t('Only successful requests')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          </div>

          <div className='bg-muted/40 space-y-3 rounded-lg border p-4'>
            <div className='space-y-1 text-sm'>
              <p>
                {t('Effective period: {{duration}}', {
                  duration: periodState.effectiveDuration,
                })}
              </p>
              {!defaultValues.ModelRequestRateLimitDurationActive && (
                <>
                  <p>
                    {t('Staged period: {{duration}}', {
                      duration: periodState.stagedDuration,
                    })}
                  </p>
                  <p className='text-amber-700 dark:text-amber-400'>
                    {t('The staged period is not active yet.')}
                  </p>
                </>
              )}
              {periodState.activationPending && (
                <p className='text-amber-700 dark:text-amber-400'>
                  {activationTime
                    ? t('Scheduled activation: {{time}} (local time)', {
                        time: activationTime,
                      })
                    : t('Activation is scheduled.')}{' '}
                  {t(
                    'The legacy period remains effective until the server confirms activation.'
                  )}
                </p>
              )}
              {periodState.isDraft && (
                <p className='text-muted-foreground'>
                  {t(
                    'Unsaved draft: {{duration}}. The effective period is unchanged.',
                    { duration: durationValue || '—' }
                  )}
                </p>
              )}
            </div>

            <SettingsSwitchItem>
              <SettingsSwitchContent>
                <Label htmlFor={durationActivationId}>
                  {t('Activate staged rate-limit period')}
                </Label>
                <p className='text-muted-foreground text-sm'>
                  {durationActivationDescription}
                </p>
              </SettingsSwitchContent>
              <Switch
                id={durationActivationId}
                checked={defaultValues.ModelRequestRateLimitDurationActivated}
                disabled={!periodState.canActivate || updateOption.isPending}
                onCheckedChange={(checked) => {
                  if (checked) {
                    updateOption.mutate(rateLimitDurationActivationUpdate)
                  }
                }}
              />
            </SettingsSwitchItem>
          </div>

          <FormField
            control={form.control}
            name='ModelRequestRateLimitGroup'
            render={({ field }) => (
              <FormItem>
                <div className='flex items-center justify-between'>
                  <FormLabel>{t('Group-based rate limits')}</FormLabel>
                  <Button
                    type='button'
                    variant='outline'
                    size='sm'
                    onClick={() => setUseVisualEditor(!useVisualEditor)}
                  >
                    {useVisualEditor ? (
                      <>
                        <Code2 className='mr-2 h-4 w-4' />
                        {t('JSON Mode')}
                      </>
                    ) : (
                      <>
                        <Palette className='mr-2 h-4 w-4' />
                        {t('Visual Mode')}
                      </>
                    )}
                  </Button>
                </div>
                <FormControl>
                  {useVisualEditor ? (
                    <RateLimitVisualEditor
                      value={field.value || ''}
                      onChange={field.onChange}
                    />
                  ) : (
                    <Textarea
                      rows={8}
                      placeholder={`{\n  "default": [200, 100],\n  "vip": [0, 1000]\n}`}
                      className='font-mono text-sm'
                      {...field}
                    />
                  )}
                </FormControl>
                {!useVisualEditor && (
                  <FormDescription>
                    <div className='space-y-1 text-xs'>
                      <p className='font-semibold'>{t('Format:')}</p>
                      <ul className='list-inside list-disc space-y-0.5 pl-2'>
                        <li>
                          {t('JSON object:')}{' '}
                          {`{"groupName": [maxRequests, maxSuccess]}`}
                        </li>
                        <li>
                          {t('Example:')}{' '}
                          {`{"default": [200, 100], "vip": [0, 1000]}`}
                        </li>
                        <li>
                          {t(
                            'maxRequests ≥ 0, maxSuccess ≥ 1, both ≤ 2,147,483,647'
                          )}
                        </li>
                        <li>
                          {t(
                            'Group config overrides global limits, shares the same period'
                          )}
                        </li>
                      </ul>
                    </div>
                  </FormDescription>
                )}
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

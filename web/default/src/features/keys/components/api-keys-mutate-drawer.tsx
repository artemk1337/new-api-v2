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
import { useQuery } from '@tanstack/react-query'
import {
  AlertTriangle,
  ChevronDown,
  Info,
  KeyRound,
  Settings2,
  WalletCards,
} from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useForm, type SubmitErrorHandler } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { DateTimePicker } from '@/components/datetime-picker'
import {
  SideDrawerSection,
  SideDrawerSectionHeader,
  sideDrawerContentClassName,
  sideDrawerFooterClassName,
  sideDrawerFormClassName,
  sideDrawerHeaderClassName,
  sideDrawerSwitchItemClassName,
} from '@/components/drawer-layout'
import { MultiSelect } from '@/components/multi-select'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
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
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { getUserModels, getUserGroups } from '@/lib/api'
import { ModelMappingEditor } from '@/features/channels/components/model-mapping-editor'
import { validateModelMappingJson } from '@/features/channels/lib/model-mapping-validation'
import { getCurrencyDisplay, getCurrencyLabel } from '@/lib/currency'
import { cn } from '@/lib/utils'

import { createApiKey, fetchTokenKey, updateApiKey, getApiKey } from '../api'
import { ERROR_MESSAGES, SUCCESS_MESSAGES } from '../constants'
import {
  getApiKeyFormSchema,
  type ApiKeyFormValues,
  getApiKeyFormDefaultValues,
  resolveAutoGroupCandidates,
  transformFormDataToPayload,
  transformApiKeyToFormDefaults,
} from '../lib'
import type { ApiKey } from '../types'
import {
  ApiKeyGroupCombobox,
  type ApiKeyGroupOption,
} from './api-key-group-combobox'
import { useApiKeys } from './api-keys-provider'
import { ApiKeyCreatedDialog } from './dialogs/api-key-created-dialog'
import { normalizeApiKey } from './dialogs/api-key-created-dialog-utils'

type ApiKeyMutateDrawerProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  currentRow?: ApiKey
}

export function ApiKeysMutateDrawer({
  open,
  onOpenChange,
  currentRow,
}: ApiKeyMutateDrawerProps) {
  const { t } = useTranslation()
  const isUpdate = !!currentRow
  const { triggerRefresh } = useApiKeys()
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [isAutoHelpOpen, setIsAutoHelpOpen] = useState(false)
  const [createdKey, setCreatedKey] = useState('')
  const [createdTokenId, setCreatedTokenId] = useState<number | null>(null)
  const [createdKeyResolved, setCreatedKeyResolved] = useState(false)
  const [createdKeyError, setCreatedKeyError] = useState('')
  const [isRetryingCreatedKey, setIsRetryingCreatedKey] = useState(false)
  const [createdDialogOpen, setCreatedDialogOpen] = useState(false)

  // Fetch models
  const { data: modelsData } = useQuery({
    queryKey: ['user-models'],
    queryFn: getUserModels,
    enabled: open,
    staleTime: 0,
  })

  // Fetch groups
  const { data: groupsData } = useQuery({
    queryKey: ['user-groups'],
    queryFn: getUserGroups,
    enabled: open,
    staleTime: 0,
  })

  const groupOptionsReady = groupsData?.success === true
  const models = modelsData?.data || []
  const groupsRaw = groupsData?.data
  const concreteGroups = useMemo<ApiKeyGroupOption[]>(
    () =>
      Object.entries(groupsRaw ?? {})
        .filter(([key]) => key !== 'auto')
        .map(([key, info]) => ({
          value: key,
          label: info.name || key,
          desc: info.desc || info.name || key,
          ratio: info.ratio,
        })),
    [groupsRaw]
  )
  const autoGroupValues = useMemo(
    () =>
      resolveAutoGroupCandidates(
        concreteGroups.map((group) => group.value),
        groupsData?.auto_groups ?? []
      ),
    [concreteGroups, groupsData?.auto_groups]
  )
  const autoCandidateGroups = useMemo(() => {
    const concreteByValue = new Map(
      concreteGroups.map((group) => [group.value, group])
    )
    return autoGroupValues
      .map((group) => concreteByValue.get(group))
      .filter((group): group is ApiKeyGroupOption => group !== undefined)
  }, [autoGroupValues, concreteGroups])
  const autoAvailable = autoCandidateGroups.length > 0
  const availableGroups = useMemo<ApiKeyGroupOption[]>(() => {
    if (!autoAvailable) return concreteGroups
    return [
      {
        value: 'auto',
        label: 'Auto',
        desc: t('Iterates through available groups in ascending price order.'),
        dynamicRatio: true,
      },
      ...concreteGroups,
    ]
  }, [autoAvailable, concreteGroups, t])
  const schema = getApiKeyFormSchema(t)

  const form = useForm<ApiKeyFormValues>({
    resolver: zodResolver(schema),
    defaultValues: getApiKeyFormDefaultValues(),
  })

  // Load existing data when updating
  useEffect(() => {
    let ignore = false

    if (open && isUpdate && currentRow) {
      getApiKey(currentRow.id)
        .then((result) => {
          if (!ignore && result.success && result.data) {
            form.reset(transformApiKeyToFormDefaults(result.data))
          }
        })
        .catch(() => {
          if (!ignore) {
            toast.error(t(ERROR_MESSAGES.UNEXPECTED))
          }
        })
    } else if (open && !isUpdate) {
      form.reset(getApiKeyFormDefaultValues())
    }

    return () => {
      ignore = true
    }
  }, [open, isUpdate, currentRow, form, t])

  const selectedGroup = form.watch('group')
  const autoGroupMode = form.watch('auto_group_mode')
  const selectedCandidates = form.watch('auto_group_candidates')
  const unlimitedQuota = form.watch('unlimited_quota')

  const availableGroupValues = useMemo(
    () => new Set(availableGroups.map((group) => group.value)),
    [availableGroups]
  )
  const autoCandidateValues = useMemo(
    () => new Set(autoGroupValues),
    [autoGroupValues]
  )
  const groups = useMemo<ApiKeyGroupOption[]>(() => {
    if (
      !groupOptionsReady ||
      !selectedGroup ||
      availableGroupValues.has(selectedGroup)
    ) {
      return availableGroups
    }
    const unavailableGroup = {
      value: selectedGroup,
      label: `${selectedGroup} (${t('Unavailable')})`,
      desc: t('This group is no longer available. Select another group.'),
    }
    return selectedGroup === 'auto'
      ? [unavailableGroup, ...availableGroups]
      : [...availableGroups, unavailableGroup]
  }, [
    availableGroups,
    availableGroupValues,
    groupOptionsReady,
    selectedGroup,
    t,
  ])
  const unavailableCandidates = useMemo(
    () =>
      selectedCandidates.filter(
        (group) => group === 'auto' || !autoCandidateValues.has(group)
      ),
    [autoCandidateValues, selectedCandidates]
  )
  const candidateOptions = useMemo(() => {
    const concreteOptions = autoCandidateGroups.map((group) => ({
      value: group.value,
      label: group.label,
    }))
    const missingOptions = unavailableCandidates.map((group) => ({
      value: group,
      label: `${group} (${t('Unavailable')})`,
    }))
    return [...concreteOptions, ...missingOptions]
  }, [autoCandidateGroups, unavailableCandidates, t])

  useEffect(() => {
    if (selectedGroup === 'auto') return
    if (selectedCandidates.length > 0) {
      form.setValue('auto_group_candidates', [])
    }
    if (autoGroupMode !== 'all') {
      form.setValue('auto_group_mode', 'all')
    }
  }, [autoGroupMode, form, selectedCandidates.length, selectedGroup])

  const onSubmit = async (data: ApiKeyFormValues) => {
    const modelMappingValidation = validateModelMappingJson(data.model_mapping)
    if (!modelMappingValidation.valid) {
      form.setError('model_mapping', {
        message: t(modelMappingValidation.error || 'Invalid model mapping'),
      })
      return
    }

    if (!availableGroupValues.has(data.group)) {
      form.setError('group', {
        message: t('This group is no longer available. Select another group.'),
      })
      toast.error(t('Please remove unavailable groups before saving'))
      return
    }

    if (
      data.group === 'auto' &&
      data.auto_group_mode === 'specific' &&
      data.auto_group_candidates.some(
        (group) => group === 'auto' || !autoCandidateValues.has(group)
      )
    ) {
      form.setError('auto_group_candidates', {
        message: t('Please remove unavailable groups before saving'),
      })
      toast.error(t('Please remove unavailable groups before saving'))
      return
    }

    setIsSubmitting(true)
    try {
      const basePayload = transformFormDataToPayload(data)

      if (isUpdate && currentRow) {
        const result = await updateApiKey({
          ...basePayload,
          id: currentRow.id,
        })
        if (result.success) {
          toast.success(t(SUCCESS_MESSAGES.API_KEY_UPDATED))
          onOpenChange(false)
          triggerRefresh()
        } else {
          toast.error(result.message || t(ERROR_MESSAGES.UPDATE_FAILED))
        }
      } else {
        // Create mode - handle batch creation
        const count = data.tokenCount || 1
        let successCount = 0
        let newlyCreatedKey = ''
        let newlyCreatedTokenId: number | null = null
        let keyResolutionError = ''

        for (let i = 0; i < count; i++) {
          const result = await createApiKey({
            ...basePayload,
            name:
              i === 0 && data.name
                ? data.name
                : `${data.name || 'default'}-${Math.random().toString(36).slice(2, 8)}`,
          })
          if (result.success) {
            successCount++
            if (count === 1 && result.data?.id) {
              newlyCreatedTokenId = result.data.id
              newlyCreatedKey = result.data.key
                ? normalizeApiKey(result.data.key)
                : 'sk-********'
              try {
                const keyResult = await fetchTokenKey(result.data.id)
                if (keyResult.success && keyResult.data?.key) {
                  newlyCreatedKey = normalizeApiKey(keyResult.data.key)
                } else {
                  keyResolutionError =
                    keyResult.message ||
                    t(
                      'The API key was created, but its full value could not be loaded.'
                    )
                }
              } catch {
                keyResolutionError = t(
                  'The API key was created, but its full value could not be loaded.'
                )
              }
            }
          } else {
            toast.error(result.message || t(ERROR_MESSAGES.CREATE_FAILED))
            break
          }
        }

        if (successCount > 0) {
          if (count === 1 && newlyCreatedTokenId) {
            setCreatedTokenId(newlyCreatedTokenId)
            setCreatedKey(newlyCreatedKey)
            setCreatedKeyResolved(!keyResolutionError)
            setCreatedKeyError(keyResolutionError)
            setCreatedDialogOpen(true)
          } else {
            toast.success(
              t('Successfully created {{count}} API Key(s)', {
                count: successCount,
              })
            )
          }
          onOpenChange(false)
          triggerRefresh()
        }
      }
    } catch {
      toast.error(t(ERROR_MESSAGES.UNEXPECTED))
    } finally {
      setIsSubmitting(false)
    }
  }

  const onInvalid: SubmitErrorHandler<ApiKeyFormValues> = () => {
    toast.error(t('Please fix the highlighted fields before saving'))
  }

  const retryCreatedKey = async () => {
    if (!createdTokenId) return

    setIsRetryingCreatedKey(true)
    try {
      const result = await fetchTokenKey(createdTokenId)
      if (result.success && result.data?.key) {
        const resolvedKey = normalizeApiKey(result.data.key)
        setCreatedKey(resolvedKey)
        setCreatedKeyResolved(true)
        setCreatedKeyError('')
      } else {
        setCreatedKeyResolved(false)
        setCreatedKeyError(
          result.message ||
            t(
              'The API key was created, but its full value could not be loaded.'
            )
        )
      }
    } catch {
      setCreatedKeyResolved(false)
      setCreatedKeyError(
        t('The API key was created, but its full value could not be loaded.')
      )
    } finally {
      setIsRetryingCreatedKey(false)
    }
  }

  const handleSetExpiry = (months: number, days: number, hours: number) => {
    if (months === 0 && days === 0 && hours === 0) {
      form.setValue('expired_time', undefined)
      return
    }

    const now = new Date()
    now.setMonth(now.getMonth() + months)
    now.setDate(now.getDate() + days)
    now.setHours(now.getHours() + hours)

    form.setValue('expired_time', now)
  }

  const { meta: currencyMeta } = getCurrencyDisplay()
  const currencyLabel = getCurrencyLabel()
  const tokensOnly = currencyMeta.kind === 'tokens'
  const quotaLabel = t('Quota ({{currency}})', { currency: currencyLabel })
  const quotaPlaceholder = tokensOnly
    ? t('Enter quota in tokens')
    : t('Enter quota in {{currency}}', { currency: currencyLabel })

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        onOpenChange(v)
        if (!v) {
          form.reset()
        }
      }}
    >
      <SheetContent
        className={sideDrawerContentClassName('max-w-none sm:!max-w-[620px]')}
      >
        <SheetHeader className={sideDrawerHeaderClassName()}>
          <SheetTitle>
            {isUpdate ? t('Update API Key') : t('Create API Key')}
          </SheetTitle>
          <SheetDescription>
            {isUpdate
              ? t('Update the API key by providing necessary info.')
              : t('Add a new API key by providing necessary info.')}
          </SheetDescription>
        </SheetHeader>
        <Form {...form}>
          <form
            id='api-key-form'
            onSubmit={form.handleSubmit(onSubmit, onInvalid)}
            className={sideDrawerFormClassName('gap-5')}
          >
            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Basic Information')}
                description={t('Set API key basic information')}
                icon={<KeyRound className='size-4' />}
              />
              <FormField
                control={form.control}
                name='name'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Name')}</FormLabel>
                    <FormControl>
                      <Input {...field} placeholder={t('Enter a name')} />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='group'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Group')}</FormLabel>
                    <FormControl>
                      <ApiKeyGroupCombobox
                        options={groups}
                        value={field.value}
                        disabled={!groupOptionsReady}
                        onValueChange={(value) => {
                          field.onChange(value)
                          form.clearErrors('group')
                        }}
                        placeholder={t('Select a group')}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Group affects channel stability and model availability.'
                      )}
                    </FormDescription>
                    <FormMessage />
                    {groupOptionsReady &&
                      !availableGroupValues.has(selectedGroup) && (
                        <Alert variant='destructive'>
                          <AlertTriangle aria-hidden='true' />
                          <AlertTitle>{t('Unavailable group')}</AlertTitle>
                          <AlertDescription>
                            {t(
                              'This group is no longer available. Select another group.'
                            )}
                          </AlertDescription>
                        </Alert>
                      )}
                  </FormItem>
                )}
              />

              {selectedGroup === 'auto' &&
                groupOptionsReady &&
                autoAvailable && (
                  <div className='space-y-3 rounded-lg border p-3'>
                    <FormField
                      control={form.control}
                      name='auto_group_mode'
                      render={({ field }) => (
                        <FormItem>
                          <div className='flex items-center gap-1'>
                            <FormLabel>
                              {t('Groups available to Auto')}
                            </FormLabel>
                            <Popover
                              open={isAutoHelpOpen}
                              onOpenChange={setIsAutoHelpOpen}
                            >
                              <PopoverTrigger
                                onKeyDown={(event) => {
                                  if (
                                    event.key === 'Escape' &&
                                    isAutoHelpOpen
                                  ) {
                                    event.preventDefault()
                                    event.stopPropagation()
                                    setIsAutoHelpOpen(false)
                                  }
                                }}
                                render={
                                  <Button
                                    type='button'
                                    variant='ghost'
                                    size='icon'
                                    className='size-5'
                                  />
                                }
                              >
                                <span className='sr-only'>
                                  {t('Learn more')}
                                </span>
                                <Info className='size-4' />
                              </PopoverTrigger>
                              <PopoverContent
                                side='top'
                                align='start'
                                className='text-muted-foreground w-80 space-y-3'
                                onKeyDown={(event) => {
                                  if (event.key === 'Escape') {
                                    event.preventDefault()
                                    event.stopPropagation()
                                    setIsAutoHelpOpen(false)
                                  }
                                }}
                              >
                                <p>
                                  {t(
                                    'Auto tries the cheapest selected group first and switches only after a guaranteed non-billable error.'
                                  )}
                                </p>
                                <p>
                                  {t(
                                    'Before the request, Auto reserves the amount required by the most expensive selected group. Only the actual cost is charged.'
                                  )}
                                </p>
                                <p>
                                  {t(
                                    'If the reserve is too high, top up your balance or limit Auto to cheaper groups.'
                                  )}
                                </p>
                              </PopoverContent>
                            </Popover>
                          </div>
                          <FormControl>
                            <div className='grid grid-cols-2 gap-2'>
                              <Button
                                type='button'
                                variant={
                                  field.value === 'all' ? 'default' : 'outline'
                                }
                                aria-pressed={field.value === 'all'}
                                onClick={() => {
                                  field.onChange('all')
                                  form.setValue('auto_group_candidates', [])
                                  form.clearErrors('auto_group_candidates')
                                }}
                              >
                                {t('All groups')}
                              </Button>
                              <Button
                                type='button'
                                variant={
                                  field.value === 'specific'
                                    ? 'default'
                                    : 'outline'
                                }
                                aria-pressed={field.value === 'specific'}
                                onClick={() => field.onChange('specific')}
                              >
                                {t('Specific groups')}
                              </Button>
                            </div>
                          </FormControl>
                        </FormItem>
                      )}
                    />

                    {autoGroupMode === 'specific' && (
                      <FormField
                        control={form.control}
                        name='auto_group_candidates'
                        render={({ field }) => (
                          <FormItem>
                            <FormLabel>{t('Auto group candidates')}</FormLabel>
                            <FormControl>
                              <MultiSelect
                                options={candidateOptions}
                                selected={field.value}
                                onChange={(values) => {
                                  field.onChange(
                                    values.filter((value) => value !== 'auto')
                                  )
                                  form.clearErrors('auto_group_candidates')
                                }}
                                placeholder={t('Select groups for Auto')}
                              />
                            </FormControl>
                            <FormMessage />
                          </FormItem>
                        )}
                      />
                    )}

                    {unavailableCandidates.length > 0 && (
                      <Alert variant='destructive'>
                        <AlertTriangle aria-hidden='true' />
                        <AlertTitle>{t('Unavailable groups')}</AlertTitle>
                        <AlertDescription>
                          {t(
                            'These saved groups are no longer available: {{groups}}. Remove them before saving.',
                            { groups: unavailableCandidates.join(', ') }
                          )}
                        </AlertDescription>
                      </Alert>
                    )}
                  </div>
                )}

              <FormField
                control={form.control}
                name='expired_time'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Expiration Time')}</FormLabel>
                    <div className='grid gap-2 sm:grid-cols-[minmax(0,1fr)_auto] sm:items-center'>
                      <FormControl>
                        <DateTimePicker
                          value={field.value}
                          onChange={field.onChange}
                          placeholder={t('Never expires')}
                          className='min-w-0 [&_input[type=time]]:w-24 sm:[&_input[type=time]]:w-32'
                        />
                      </FormControl>
                      <div className='grid grid-cols-4 gap-2 sm:flex'>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 0)}
                        >
                          {t('Never')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(1, 0, 0)}
                        >
                          {t('1 Month')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 1, 0)}
                        >
                          {t('1 Day')}
                        </Button>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          className='px-2 text-xs sm:px-3 sm:text-sm'
                          onClick={() => handleSetExpiry(0, 0, 1)}
                        >
                          {t('1 Hour')}
                        </Button>
                      </div>
                    </div>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {!isUpdate && (
                <FormField
                  control={form.control}
                  name='tokenCount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('Quantity')}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          min='1'
                          placeholder={t('Number of keys to create')}
                          onChange={(e) =>
                            field.onChange(parseInt(e.target.value, 10) || 1)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Create multiple API keys at once (random suffix will be added to names)'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </SideDrawerSection>

            <SideDrawerSection>
              <SideDrawerSectionHeader
                title={t('Quota Settings')}
                description={t('Set quota amount and limits')}
                icon={<WalletCards className='size-4' />}
              />
              {!unlimitedQuota && (
                <FormField
                  control={form.control}
                  name='remain_quota_dollars'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{quotaLabel}</FormLabel>
                      <FormControl>
                        <Input
                          {...field}
                          type='number'
                          step={tokensOnly ? 1 : 0.01}
                          placeholder={quotaPlaceholder}
                          onChange={(e) =>
                            field.onChange(parseFloat(e.target.value) || 0)
                          }
                        />
                      </FormControl>
                      <FormDescription>
                        {tokensOnly
                          ? t('Enter the quota amount in tokens')
                          : t('Enter the quota amount in {{currency}}', {
                              currency: currencyLabel,
                            })}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}

              <FormField
                control={form.control}
                name='unlimited_quota'
                render={({ field }) => (
                  <FormItem className={sideDrawerSwitchItemClassName()}>
                    <div className='flex flex-col gap-0.5'>
                      <FormLabel className='text-sm'>
                        {t('Unlimited Quota')}
                      </FormLabel>
                      <FormDescription className='text-xs'>
                        {t('Enable unlimited quota for this API key')}
                      </FormDescription>
                    </div>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </FormItem>
                )}
              />
            </SideDrawerSection>

            <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
              <SideDrawerSection>
                <CollapsibleTrigger
                  render={
                    <button
                      type='button'
                      className='hover:bg-muted/40 flex w-full items-center gap-3 rounded-md py-1.5 text-left transition-colors'
                    />
                  }
                >
                  <SideDrawerSectionHeader
                    className='flex-1'
                    title={t('Advanced Settings')}
                    description={t('Set API key access restrictions')}
                    icon={<Settings2 className='size-4' />}
                  />
                  <ChevronDown
                    className={cn(
                      'text-muted-foreground size-4 shrink-0 transition-transform',
                      advancedOpen && 'rotate-180'
                    )}
                  />
                </CollapsibleTrigger>
                <CollapsibleContent>
                  <div className='flex flex-col gap-4 pt-2'>
                    <FormField
                      control={form.control}
                      name='model_limits'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Limits')}</FormLabel>
                          <FormControl>
                            <MultiSelect
                              options={models.map((m) => ({
                                label: m,
                                value: m,
                              }))}
                              selected={field.value}
                              onChange={field.onChange}
                              placeholder={t(
                                'Select models (empty for allow all)'
                              )}
                            />
                          </FormControl>
                          <FormDescription>
                            {t('Limit which models can be used with this key')}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='model_mapping'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>{t('Model Mapping')}</FormLabel>
                          <FormControl>
                            <ModelMappingEditor
                              value={field.value}
                              onChange={field.onChange}
                              disabled={isSubmitting}
                              sourceModelOptions={models}
                              targetModelOptions={models}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Map this key\'s requested model names to available models before channel routing.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />

                    <FormField
                      control={form.control}
                      name='allow_ips'
                      render={({ field }) => (
                        <FormItem>
                          <FormLabel>
                            {t('IP Whitelist (supports CIDR)')}
                          </FormLabel>
                          <FormControl>
                            <Textarea
                              {...field}
                              className='min-h-20 resize-none'
                              placeholder={t(
                                'One IP per line (empty for no restriction)'
                              )}
                              rows={3}
                            />
                          </FormControl>
                          <FormDescription>
                            {t(
                              'Do not over-trust this feature. IP may be spoofed. Please use with nginx, CDN and other gateways.'
                            )}
                          </FormDescription>
                          <FormMessage />
                        </FormItem>
                      )}
                    />
                  </div>
                </CollapsibleContent>
              </SideDrawerSection>
            </Collapsible>
          </form>
        </Form>
        <SheetFooter className={sideDrawerFooterClassName()}>
          <SheetClose
            render={<Button variant='outline' className='w-full sm:w-auto' />}
          >
            {t('Close')}
          </SheetClose>
          <Button
            type='button'
            onClick={form.handleSubmit(onSubmit, onInvalid)}
            disabled={isSubmitting || !groupOptionsReady}
            className='w-full sm:w-auto'
          >
            {isSubmitting ? t('Saving...') : t('Save changes')}
          </Button>
        </SheetFooter>
      </SheetContent>
      <ApiKeyCreatedDialog
        open={createdDialogOpen}
        tokenKey={createdKey}
        keyResolved={createdKeyResolved}
        keyError={createdKeyError}
        retrying={isRetryingCreatedKey}
        onRetry={() => void retryCreatedKey()}
        onOpenChange={(nextOpen) => {
          setCreatedDialogOpen(nextOpen)
          if (!nextOpen) {
            setCreatedKey('')
            setCreatedTokenId(null)
            setCreatedKeyResolved(false)
            setCreatedKeyError('')
          }
        }}
      />
    </Sheet>
  )
}

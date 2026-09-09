/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
import { useEffect, useMemo, useState } from 'react'
import * as z from 'zod'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'

import { Checkbox } from '@/components/ui/checkbox'
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
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  accountVerificationProviders,
  parseAccountVerificationProviders,
  type AccountVerificationProvider,
} from './account-verification-options'

type AccountVerificationDefaults = {
  AccountVerificationEnabled: boolean
  AccountVerificationProviders: string
  AccountVerificationFreezeDelayMinutes: number
}

const schema = z
  .object({
    enabled: z.boolean(),
    providers: z.array(z.enum(accountVerificationProviders)),
    delayMinutes: z.number().int().min(1).max(43200),
  })
  .refine((value) => !value.enabled || value.providers.length > 0, {
    message: 'Select at least one verification provider',
    path: ['providers'],
  })

type FormValues = z.infer<typeof schema>

const providerLabels: Record<AccountVerificationProvider, string> = {
  email: 'Email',
  telegram: 'Telegram',
  github: 'GitHub',
}

type Props = { defaultValues: AccountVerificationDefaults }

export function AccountVerificationSection({ defaultValues }: Props) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const [unit, setUnit] = useState<'minutes' | 'hours'>(() =>
    defaultValues.AccountVerificationFreezeDelayMinutes >= 60 &&
    defaultValues.AccountVerificationFreezeDelayMinutes % 60 === 0
      ? 'hours'
      : 'minutes'
  )
  const formDefaults = useMemo<FormValues>(
    () => ({
      enabled: defaultValues.AccountVerificationEnabled,
      providers: parseAccountVerificationProviders(
        defaultValues.AccountVerificationProviders
      ),
      delayMinutes: defaultValues.AccountVerificationFreezeDelayMinutes,
    }),
    [defaultValues]
  )
  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  useEffect(() => {
    setUnit(
      formDefaults.delayMinutes >= 60 && formDefaults.delayMinutes % 60 === 0
        ? 'hours'
        : 'minutes'
    )
  }, [formDefaults.delayMinutes])

  const onSubmit = async (values: FormValues) => {
    const updates: Array<{ key: string; value: string | boolean | number }> = []
    const providerValue = values.providers.join(',')
    if (values.enabled !== defaultValues.AccountVerificationEnabled) {
      updates.push({ key: 'AccountVerificationEnabled', value: values.enabled })
    }
    if (providerValue !== defaultValues.AccountVerificationProviders) {
      updates.push({ key: 'AccountVerificationProviders', value: providerValue })
    }
    if (
      values.delayMinutes !== defaultValues.AccountVerificationFreezeDelayMinutes
    ) {
      updates.push({
        key: 'AccountVerificationFreezeDelayMinutes',
        value: values.delayMinutes,
      })
    }
    for (const update of updates) await updateOption.mutateAsync(update)
  }

  const unitFactor = unit === 'hours' ? 60 : 1

  return (
    <SettingsSection title={t('Account Verification')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />

          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Require account verification')}</FormLabel>
                  <FormDescription>
                    {t('Require users to verify their account after registration.')}
                  </FormDescription>
                </SettingsSwitchContent>
                <FormControl>
                  <Switch checked={field.value} onCheckedChange={field.onChange} />
                </FormControl>
              </SettingsSwitchItem>
            )}
          />

          <FormField
            control={form.control}
            name='providers'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Verification providers')}</FormLabel>
                <div className='grid gap-3 sm:grid-cols-3'>
                  {accountVerificationProviders.map((provider) => {
                    const checked = field.value.includes(provider)
                    return (
                      <label
                        key={provider}
                        className='border-input hover:bg-muted/40 flex cursor-pointer items-center gap-2 rounded-lg border p-3 text-sm'
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(next) => {
                            const nextValue = next
                              ? [...field.value, provider]
                              : field.value.filter((item) => item !== provider)
                            field.onChange([...new Set(nextValue)])
                          }}
                        />
                        {t(providerLabels[provider])}
                      </label>
                    )
                  })}
                </div>
                <FormDescription>
                  {t('Users can verify with any selected provider.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='delayMinutes'
            render={({ field, fieldState }) => (
              <FormItem>
                <FormLabel>{t('Verification freeze delay')}</FormLabel>
                <div className='flex items-center gap-2'>
                  <FormControl>
                    <Input
                      type='number'
                      min={1}
                      max={unit === 'hours' ? 720 : 43200}
                      step={1}
                      value={Math.round(field.value / unitFactor)}
                      onChange={(event) =>
                        field.onChange(
                          Math.max(1, Number.parseInt(event.target.value, 10) || 1) *
                            unitFactor
                        )
                      }
                    />
                  </FormControl>
                  <Select
                    value={unit}
                    onValueChange={(value) => {
                      if (!value) return
                      const nextUnit = value as 'minutes' | 'hours'
                      const nextFactor = nextUnit === 'hours' ? 60 : 1
                      const nextValue = Math.max(
                        1,
                        Math.round(form.getValues('delayMinutes') / nextFactor)
                      )
                      form.setValue('delayMinutes', nextValue * nextFactor, {
                        shouldDirty: true,
                        shouldValidate: true,
                      })
                      setUnit(nextUnit)
                    }}
                  >
                    <SelectTrigger className='w-28' aria-label={t('Delay unit')}>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectGroup>
                        <SelectItem value='minutes'>{t('minutes')}</SelectItem>
                        <SelectItem value='hours'>{t('hours')}</SelectItem>
                      </SelectGroup>
                    </SelectContent>
                  </Select>
                </div>
                <FormDescription>
                  {t('After this delay, unverified accounts are frozen. They are deleted after 30 days.')}
                </FormDescription>
                <FormMessage>{fieldState.error?.message}</FormMessage>
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

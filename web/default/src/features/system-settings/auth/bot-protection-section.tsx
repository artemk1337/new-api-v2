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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from '@/components/ui/field'
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
import { Switch } from '@/components/ui/switch'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  createBotProtectionSchema,
  getBotProtectionOptionUpdates,
  type BotProtectionFormValues,
} from './bot-protection-options'

type BotProtectionSectionProps = {
  defaultValues: BotProtectionFormValues
}

export function BotProtectionSection({
  defaultValues,
}: BotProtectionSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const botProtectionSchema = createBotProtectionSchema(t)

  const form = useForm<BotProtectionFormValues>({
    resolver: zodResolver(botProtectionSchema),
    defaultValues,
  })
  const registrationRateLimitEnabled = form.watch(
    'RegistrationRateLimitEnabled'
  )

  useEffect(() => {
    form.reset(defaultValues)
  }, [defaultValues, form])

  const onSubmit = async (data: BotProtectionFormValues) => {
    const updates = getBotProtectionOptionUpdates(data, defaultValues)

    for (const update of updates) {
      await updateOption.mutateAsync(update)
    }
  }

  return (
    <SettingsSection title={t('Bot Protection')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />

          <FieldSet className='rounded-lg border p-4'>
            <FieldLegend>{t('Registration rate limiting')}</FieldLegend>
            <FieldDescription>
              {t(
                'Limits are enforced per client IP. Ensure trusted proxy settings preserve the real visitor IP.'
              )}
            </FieldDescription>

            <FieldGroup className='gap-4'>
              <FormField
                control={form.control}
                name='RegistrationRateLimitEnabled'
                render={({ field, fieldState }) => (
                  <Field
                    orientation='horizontal'
                    data-invalid={fieldState.invalid}
                  >
                    <FieldContent>
                      <FieldLabel htmlFor='registration-rate-limit-enabled'>
                        {t('Enable registration rate limiting')}
                      </FieldLabel>
                      <FieldDescription>
                        {t(
                          'Reject excessive signup attempts with HTTP 429 before creating an account.'
                        )}
                      </FieldDescription>
                    </FieldContent>
                    <Switch
                      id='registration-rate-limit-enabled'
                      checked={field.value}
                      onCheckedChange={field.onChange}
                      aria-invalid={fieldState.invalid}
                    />
                  </Field>
                )}
              />

              <FieldGroup className='grid gap-4 md:grid-cols-3'>
                <FormField
                  control={form.control}
                  name='RegistrationRateLimitAttempts'
                  render={({ field, fieldState }) => (
                    <Field
                      data-invalid={fieldState.invalid}
                      data-disabled={!registrationRateLimitEnabled}
                    >
                      <FieldLabel htmlFor='registration-rate-limit-attempts'>
                        {t('Registration attempts')}
                      </FieldLabel>
                      <Input
                        id='registration-rate-limit-attempts'
                        type='number'
                        min={1}
                        max={1000}
                        step={1}
                        disabled={!registrationRateLimitEnabled}
                        aria-invalid={fieldState.invalid}
                        {...field}
                        onChange={(event) =>
                          field.onChange(
                            Number.parseInt(event.target.value, 10) || 1
                          )
                        }
                      />
                      <FieldDescription>
                        {t(
                          'Maximum signup attempts per IP in the configured window. Default: 10.'
                        )}
                      </FieldDescription>
                      <FieldError errors={[fieldState.error]} />
                    </Field>
                  )}
                />

                <FormField
                  control={form.control}
                  name='RegistrationRateLimitSuccesses'
                  render={({ field, fieldState }) => (
                    <Field
                      data-invalid={fieldState.invalid}
                      data-disabled={!registrationRateLimitEnabled}
                    >
                      <FieldLabel htmlFor='registration-rate-limit-successes'>
                        {t('Successful registrations')}
                      </FieldLabel>
                      <Input
                        id='registration-rate-limit-successes'
                        type='number'
                        min={1}
                        max={1000}
                        step={1}
                        disabled={!registrationRateLimitEnabled}
                        aria-invalid={fieldState.invalid}
                        {...field}
                        onChange={(event) =>
                          field.onChange(
                            Number.parseInt(event.target.value, 10) || 1
                          )
                        }
                      />
                      <FieldDescription>
                        {t(
                          'Maximum accounts created per IP in the configured window. Default: 3.'
                        )}
                      </FieldDescription>
                      <FieldError errors={[fieldState.error]} />
                    </Field>
                  )}
                />

                <FormField
                  control={form.control}
                  name='RegistrationRateLimitWindowMinutes'
                  render={({ field, fieldState }) => (
                    <Field
                      data-invalid={fieldState.invalid}
                      data-disabled={!registrationRateLimitEnabled}
                    >
                      <FieldLabel htmlFor='registration-rate-limit-window'>
                        {t('Registration window (minutes)')}
                      </FieldLabel>
                      <Input
                        id='registration-rate-limit-window'
                        type='number'
                        min={1}
                        max={1440}
                        step={1}
                        disabled={!registrationRateLimitEnabled}
                        aria-invalid={fieldState.invalid}
                        {...field}
                        onChange={(event) =>
                          field.onChange(
                            Number.parseInt(event.target.value, 10) || 1
                          )
                        }
                      />
                      <FieldDescription>
                        {t(
                          'Time window shared by attempt and success limits. Default: 24 hours.'
                        )}
                      </FieldDescription>
                      <FieldError errors={[fieldState.error]} />
                    </Field>
                  )}
                />
              </FieldGroup>
            </FieldGroup>
          </FieldSet>

          <FormField
            control={form.control}
            name='TurnstileCheckEnabled'
            render={({ field }) => (
              <SettingsSwitchItem>
                <SettingsSwitchContent>
                  <FormLabel>{t('Enable Turnstile')}</FormLabel>
                  <FormDescription>
                    {t(
                      'Protect login and registration with Cloudflare Turnstile'
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

          <FormField
            control={form.control}
            name='TurnstileSiteKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Site Key')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Your Turnstile site key')}
                    autoComplete='off'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='TurnstileSecretKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Secret Key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    placeholder={t('Your Turnstile secret key')}
                    autoComplete='new-password'
                    {...field}
                  />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

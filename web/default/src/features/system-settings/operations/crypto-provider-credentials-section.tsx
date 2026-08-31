/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

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

import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'
import { shouldUpdateCryptoProviderCredential } from './crypto-provider-credentials'

const cryptoProviderCredentialsSchema = z.object({
  USDTTRC20APIKey: z.string(),
  USDTTONAPIKey: z.string(),
  USDTSolanaAPIKey: z.string(),
})

type CryptoProviderCredentialsFormValues = z.infer<
  typeof cryptoProviderCredentialsSchema
>

type CryptoProviderCredentialsSectionProps = {
  defaultValues: CryptoProviderCredentialsFormValues
}

export function CryptoProviderCredentialsSection({
  defaultValues,
}: CryptoProviderCredentialsSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const form = useForm<CryptoProviderCredentialsFormValues>({
    resolver: zodResolver(cryptoProviderCredentialsSchema),
    defaultValues,
  })

  useResetForm(form, defaultValues)

  const onSubmit = async (values: CryptoProviderCredentialsFormValues) => {
    const credentials = [
      ['USDTTRC20APIKey', values.USDTTRC20APIKey, defaultValues.USDTTRC20APIKey],
      ['USDTTONAPIKey', values.USDTTONAPIKey, defaultValues.USDTTONAPIKey],
      ['USDTSolanaAPIKey', values.USDTSolanaAPIKey, defaultValues.USDTSolanaAPIKey],
    ] as const
    for (const [key, value, initial] of credentials) {
      const currentValue = value.trim()
      if (shouldUpdateCryptoProviderCredential(currentValue, initial)) {
        await updateOption.mutateAsync({ key, value: currentValue })
      }
    }
  }

  return (
    <SettingsSection title={t('Crypto provider credentials')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)} autoComplete='off'>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save settings'
          />
          <FormField
            control={form.control}
            name='USDTTRC20APIKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('TronGrid API key')}</FormLabel>
                <FormControl>
                  <Input
                    type='password'
                    autoComplete='new-password'
                    placeholder={t('Leave blank unless updating')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Read-only TronGrid access is required to verify TRON transfers. It is never used to move funds.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='USDTTONAPIKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Toncenter API key (TON)')}</FormLabel>
                <FormControl><Input type='password' autoComplete='new-password' placeholder={t('Leave blank unless updating')} {...field} /></FormControl>
                <FormDescription>{t('Read-only Toncenter access verifies USDT TON transfers. It is never used to move funds.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='USDTSolanaAPIKey'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Solana RPC API key')}</FormLabel>
                <FormControl><Input type='password' autoComplete='new-password' placeholder={t('Leave blank unless updating')} {...field} /></FormControl>
                <FormDescription>{t('Read-only Solana RPC access verifies USDT transfers. It is never used to move funds.')}</FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

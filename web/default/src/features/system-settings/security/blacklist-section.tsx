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

For commercial licensing, please contact support@quantumnous.com
*/
import { useEffect, useMemo, useRef } from 'react'
import * as z from 'zod'
import { zodResolver } from '@hookform/resolvers/zod'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Textarea } from '@/components/ui/textarea'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import { isValidIpOrCidr, normalizeIpBlacklist } from './blacklist-validation'

const ipBlacklistKey = 'security.ip_blacklist' as const

type BlacklistSectionProps = {
  defaultValues: {
    [ipBlacklistKey]: string[]
  }
}

export function BlacklistSection(props: BlacklistSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const defaultBlacklist = props.defaultValues[ipBlacklistKey]
  const baselineRef = useRef(defaultBlacklist)
  const schema = useMemo(
    () =>
      z.object({
        ip_blacklist: z.string().superRefine((value, context) => {
          const invalidEntry = normalizeIpBlacklist(value).find(
            (entry) => !isValidIpOrCidr(entry)
          )
          if (invalidEntry) {
            context.addIssue({
              code: 'custom',
              message: t('Enter one valid IP address or CIDR range per line'),
            })
          }
        }),
      }),
    [t]
  )
  type FormValues = z.infer<typeof schema>

  const form = useForm<FormValues>({
    resolver: zodResolver(schema),
    defaultValues: {
      ip_blacklist: defaultBlacklist.join('\n'),
    },
  })

  useEffect(() => {
    baselineRef.current = defaultBlacklist
    form.reset({
      ip_blacklist: defaultBlacklist.join('\n'),
    })
  }, [defaultBlacklist, form])

  const onSubmit = async (values: FormValues) => {
    const entries = normalizeIpBlacklist(values.ip_blacklist)
    if (JSON.stringify(entries) === JSON.stringify(baselineRef.current)) {
      toast.info(t('No changes to save'))
      return
    }

    await updateOption.mutateAsync({
      key: ipBlacklistKey,
      value: JSON.stringify(entries),
    })
    baselineRef.current = entries
  }

  return (
    <SettingsSection title={t('Blacklist')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
            saveLabel='Save IP blacklist'
          />
          <FormField
            control={form.control}
            name='ip_blacklist'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('IP')}</FormLabel>
                <FormControl>
                  <Textarea
                    rows={8}
                    placeholder={t('IP blacklist example')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('One IP or CIDR range per line')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

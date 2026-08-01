import * as z from 'zod'
import type { Resolver } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { useTranslation } from 'react-i18next'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FormDirtyIndicator } from '../components/form-dirty-indicator'
import { FormNavigationGuard } from '../components/form-navigation-guard'
import { SettingsForm } from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useSettingsForm } from '../hooks/use-settings-form'
import { useUpdateOption } from '../hooks/use-update-option'

const schema = z.object({
  provider: z.enum(['cbr', 'bybit_p2p']),
  updateInterval: z.enum(['minute', 'hour', 'day']),
})

type CurrencyExchangeRateFormValues = z.infer<typeof schema>

type CurrencyExchangeRateSectionProps = {
  defaultValues: CurrencyExchangeRateFormValues
}

export function CurrencyExchangeRateSection(
  props: CurrencyExchangeRateSectionProps
) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const { form, handleSubmit, handleReset, isDirty, isSubmitting } =
    useSettingsForm<CurrencyExchangeRateFormValues>({
      resolver: zodResolver(schema) as Resolver<CurrencyExchangeRateFormValues>,
      defaultValues: props.defaultValues,
      onSubmit: async (_data, changedFields) => {
        for (const [key, value] of Object.entries(changedFields)) {
          await updateOption.mutateAsync({
            key: `currency_exchange_rate.${
              key === 'updateInterval' ? 'update_interval' : key
            }`,
            value: String(value),
          })
        }
      },
    })

  return (
    <>
      <FormNavigationGuard when={isDirty} />

      <SettingsSection title={t('Exchange Rate Updates')}>
        <Form {...form}>
          <SettingsForm onSubmit={handleSubmit}>
            <SettingsPageFormActions
              onSave={handleSubmit}
              onReset={handleReset}
              isSaving={updateOption.isPending || isSubmitting}
              isResetDisabled={!isDirty}
            />
            <FormDirtyIndicator isDirty={isDirty} />
            <div className='grid gap-6 sm:grid-cols-2'>
              <FormField
                control={form.control}
                name='provider'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Provider')}</FormLabel>
                    <Select
                      items={[
                        {
                          value: 'cbr',
                          label: t('Central Bank of Russia (USD/RUB)'),
                        },
                        {
                          value: 'bybit_p2p',
                          label: t('Bybit P2P (USDT/RUB)'),
                        },
                      ]}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Provider')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='cbr'>
                            {t('Central Bank of Russia (USD/RUB)')}
                          </SelectItem>
                          <SelectItem value='bybit_p2p'>
                            {t('Bybit P2P (USDT/RUB)')}
                          </SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {t('Provider for the selected currency pair')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />

              <FormField
                control={form.control}
                name='updateInterval'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Update interval')}</FormLabel>
                    <Select
                      items={[
                        { value: 'minute', label: t('1 minute') },
                        { value: 'hour', label: t('1 hour') },
                        { value: 'day', label: t('1 Day') },
                      ]}
                      value={field.value}
                      onValueChange={field.onChange}
                    >
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder={t('Update interval')} />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent alignItemWithTrigger={false}>
                        <SelectGroup>
                          <SelectItem value='minute'>
                            {t('1 minute')}
                          </SelectItem>
                          <SelectItem value='hour'>{t('1 hour')}</SelectItem>
                          <SelectItem value='day'>{t('1 Day')}</SelectItem>
                        </SelectGroup>
                      </SelectContent>
                    </Select>
                    <FormDescription>
                      {t('How often to fetch the selected currency pair')}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </SettingsForm>
        </Form>
      </SettingsSection>
    </>
  )
}

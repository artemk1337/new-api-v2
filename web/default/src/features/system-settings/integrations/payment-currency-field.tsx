import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'

import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { getPlatformCurrencies } from '../general/platform-currencies-api'

type PaymentCurrencyFieldProps = {
  value: string
  fixedCurrency?: string
  onChange?: (value: string) => void
}

/** Currency control for a provider integration tab.
 * Fixed providers expose the same selector as configurable providers, but the
 * list is constrained to the gateway's canonical settlement currency. This
 * keeps the setting visible in every gateway tab without allowing an invalid
 * currency to be persisted. Waffo passes no fixedCurrency and gets all active
 * registry currencies.
 */
export function PaymentCurrencyField({
  value,
  fixedCurrency,
  onChange,
}: PaymentCurrencyFieldProps) {
  const { t } = useTranslation()
  const currenciesQuery = useQuery({
    queryKey: ['platform-currencies', 'enabled'],
    queryFn: () => getPlatformCurrencies(),
    staleTime: 60_000,
  })
  const currencies = (currenciesQuery.data?.data ?? []).filter(
    (currency) => currency.enabled
  )
  const resolvedValue = (fixedCurrency ?? value ?? '').toUpperCase()
  const availableCurrencies = fixedCurrency
    ? currencies.filter(
        (currency) =>
          currency.code.trim().toUpperCase() === resolvedValue
      )
    : currencies
  const currencyOptions = availableCurrencies.map((currency) => ({
    value: currency.code,
    label: `${currency.code} (${currency.symbol})`,
  }))
  if (
    resolvedValue &&
    !currencyOptions.some((currency) => currency.value === resolvedValue)
  ) {
    currencyOptions.unshift({
      value: resolvedValue,
      label: `${resolvedValue} (${t('Disabled')})`,
    })
  }

  return (
    <div className='grid gap-1.5'>
      <span className='text-sm font-medium'>{t('Payment currency')}</span>
      <Select
        items={currencyOptions}
        value={resolvedValue}
        onValueChange={(next) => {
          if (next && !fixedCurrency) onChange?.(next.toUpperCase())
        }}
        disabled={Boolean(fixedCurrency)}
      >
        <SelectTrigger>
          <SelectValue placeholder={t('Select currency')} />
        </SelectTrigger>
        <SelectContent>
          {currencyOptions.map((currency) => (
            <SelectItem key={currency.value} value={currency.value}>
              {currency.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <span className='text-muted-foreground text-xs'>
        {t('The provider will charge in this currency using its configured USD rate.')}
      </span>
    </div>
  )
}

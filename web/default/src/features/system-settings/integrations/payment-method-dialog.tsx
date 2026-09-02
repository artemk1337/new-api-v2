import { zodResolver } from '@hookform/resolvers/zod'
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
import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
import { ReactIconByName } from '@/components/react-icon-by-name'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Combobox } from '@/components/ui/combobox'
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
  isConfiguredTopupGroup,
  type TopupGroupOption,
} from './payment-method-group'
import { BUILT_IN_PAYMENT_ICONS } from './payment-method-icons'
import {
  getPaymentMethodMinimumCurrency,
  hasEditablePaymentMethodMinimum,
} from './payment-method-minimum'
import {
  getPaymentTypeOptions,
  isCryptoPaymentType,
  MANUAL_TRANSFER_PAYMENT_TYPE,
  normalizePaymentMethodType,
} from './payment-method-options'

export type { TopupGroupOption } from './payment-method-group'

const createPaymentMethodDialogSchema = (
  t: (key: string) => string,
  topupGroups: TopupGroupOption[]
) =>
  z
    .object({
      name: z.string().min(1, t('Payment method name is required')),
      type: z.string().min(1, t('Payment type is required')),
      icon: z.string().optional(),
      description: z.string().max(500).optional(),
      contact_url: z.string().optional(),
      admin_only: z.boolean(),
      min_topup: z
        .string()
        .refine(
          (value) =>
            value.trim() === '' ||
            (/^\d+(?:\.\d+)?$/.test(value.trim()) && Number(value) > 0),
          t('Enter a positive minimum amount or leave blank for the default')
        ),
      pending_ttl_minutes: z
        .string()
        .refine(
          (value) =>
            value.trim() === '' ||
            (/^\d+$/.test(value.trim()) && Number(value) > 0),
          t(
            'Enter a positive whole number of minutes or leave blank for the default'
          )
        ),
      topup_group: z
        .string()
        .refine(
          (value) => isConfiguredTopupGroup(value, topupGroups),
          t('Select a configured top-up coefficient group')
        ),
    })
    .superRefine((values, context) => {
      if (
        normalizePaymentMethodType(values.type) ===
          MANUAL_TRANSFER_PAYMENT_TYPE &&
        !isSafeMessengerURL(values.contact_url ?? '')
      ) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['contact_url'],
          message: t('Enter a valid HTTPS messenger link'),
        })
      }
      if (
        isDirectCryptoPaymentType(values.type) &&
        values.min_topup.trim() !== '' &&
        Number(values.min_topup) < 10
      ) {
        context.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['min_topup'],
          message: t('Crypto minimum top-up must be at least 10 USDT'),
        })
      }
    })

type PaymentMethodDialogFormValues = z.infer<
  ReturnType<typeof createPaymentMethodDialogSchema>
>

const PAYMENT_METHOD_FORM_ID = 'payment-method-form'

function isDirectCryptoPaymentType(type: string): boolean {
  return isCryptoPaymentType(type)
}

export type PaymentMethodData = {
  name: string
  type: string
  icon?: string
  /** Persisted PayMethods historically stores scalar flags as strings. */
  admin_only?: boolean | 'true' | 'false'
  AdminOnly?: boolean | 'true' | 'false'
  min_topup?: string
  pending_ttl_minutes?: string
  color?: string
  description?: string
  contact_url?: string
  topup_group?: string
}

function isSafeMessengerURL(value: string): boolean {
  try {
    const url = new URL(value.trim())
    return (
      url.protocol === 'https:' &&
      !url.username &&
      !url.password &&
      !url.port &&
      url.pathname !== '/' &&
      [
        't.me',
        'telegram.me',
        'wa.me',
        'api.whatsapp.com',
        'm.me',
        'discord.com',
        'discord.gg',
        'vk.me',
      ].includes(
        url.hostname.toLowerCase()
      )
    )
  } catch {
    return false
  }
}

type PaymentMethodDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: PaymentMethodData) => void
  editData?: PaymentMethodData | null
  topupGroups: TopupGroupOption[]
  /** Settlement currency used by the legacy Waffo gateway. */
  waffoCurrency?: string
  defaultPendingTtlMinutes?: number
  availableIcons?: string[]
}

const PAYMENT_TYPE_ICON_NAMES: Record<string, string> = {
  alipay: 'SiAlipay',
  manual_transfer: 'LuExternalLink',
  crypto_direct: 'LuWalletCards',
  nowpayments: 'LuBitcoin',
  stripe: 'SiStripe',
  waffo: 'LuCreditCard',
  waffo_pancake: 'LuCreditCard',
  wxpay: 'SiWechat',
}

const getDefaultIconName = (type: string) => PAYMENT_TYPE_ICON_NAMES[type] ?? ''

function isAdminOnly(value: PaymentMethodData['admin_only']): boolean {
  return value === true || value === 'true'
}

export function PaymentMethodDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  topupGroups,
  waffoCurrency,
  defaultPendingTtlMinutes = 1440,
  availableIcons = [...BUILT_IN_PAYMENT_ICONS],
}: PaymentMethodDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData
  const paymentMethodDialogSchema = createPaymentMethodDialogSchema(
    t,
    topupGroups
  )
  const paymentTypeOptions = getPaymentTypeOptions(t)
  const getPaymentTypeOption = (value: string) =>
    paymentTypeOptions.find((option) => option.value === value)

  const form = useForm<PaymentMethodDialogFormValues>({
    resolver: zodResolver(paymentMethodDialogSchema),
    defaultValues: {
      name: '',
      type: '',
      icon: '',
      description: '',
      contact_url: '',
      admin_only: false,
      min_topup: '',
      pending_ttl_minutes: '',
      topup_group: '',
    },
  })

  const iconValue = form.watch('icon')
  const paymentType = form.watch('type')
  const isDirectCrypto = isDirectCryptoPaymentType(paymentType)
  const isManualTransfer =
    normalizePaymentMethodType(paymentType) === MANUAL_TRANSFER_PAYMENT_TYPE
  const minimumCurrency = getPaymentMethodMinimumCurrency(
    paymentType,
    waffoCurrency
  )
  const hasEditableMinimum = hasEditablePaymentMethodMinimum(paymentType)
  const minimumInputFloor = isDirectCrypto ? '10' : '0.01'
  const effectiveDefaultPendingTtlMinutes =
    paymentType === 'yookassa_sbp' ? 15 : defaultPendingTtlMinutes
  useEffect(() => {
    if (editData) {
      const normalizedType = normalizePaymentMethodType(editData.type)
      form.reset({
        name: isCryptoPaymentType(normalizedType) ? 'Crypto' : editData.name,
        type: normalizedType,
        icon: editData.icon ?? getDefaultIconName(normalizedType),
        description: editData.description ?? '',
        contact_url: editData.contact_url ?? '',
        admin_only: isAdminOnly(editData.admin_only ?? editData.AdminOnly),
        min_topup: editData.min_topup ?? '',
        pending_ttl_minutes: editData.pending_ttl_minutes ?? '',
        topup_group: editData.topup_group ?? '',
      })
    } else {
      form.reset({
        name: '',
        type: '',
        icon: '',
        description: '',
        contact_url: '',
        admin_only: false,
        min_topup: '',
        pending_ttl_minutes: '',
        topup_group: '',
      })
    }
  }, [editData, form, open])

  const handleSubmit = (values: PaymentMethodDialogFormValues) => {
    const data: PaymentMethodData = {
      name: isDirectCryptoPaymentType(values.type) ? 'Crypto' : values.name,
      type: normalizePaymentMethodType(values.type),
      admin_only: values.admin_only,
    }
    if (values.icon && values.icon.trim() !== '') {
      data.icon = values.icon.trim()
    }
    if (values.description?.trim()) {
      data.description = values.description.trim()
    }
    if (isManualTransfer) {
      data.contact_url = values.contact_url?.trim()
    }
    if (
      (hasEditableMinimum || isDirectCrypto) &&
      values.min_topup &&
      values.min_topup.trim() !== ''
    ) {
      data.min_topup = values.min_topup
    }
    if (values.pending_ttl_minutes.trim() !== '') {
      data.pending_ttl_minutes = values.pending_ttl_minutes.trim()
    }
    data.topup_group = values.topup_group.trim()
    onSave(data)
    form.reset()
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEditMode ? t('Edit payment method') : t('Add payment method')}
      description={t('Configure a payment method for user recharge options.')}
      contentClassName='sm:max-w-[500px]'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            type='button'
            variant='outline'
            onClick={() => onOpenChange(false)}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form={PAYMENT_METHOD_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={PAYMENT_METHOD_FORM_ID}
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('e.g., Alipay, WeChat')}
                    disabled={isDirectCrypto}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Display name for this payment method.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='type'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Payment type')}</FormLabel>
                <FormControl>
                  <Combobox
                    options={paymentTypeOptions}
                    value={field.value}
                    onValueChange={(value) => {
                      if (value === null) return
                      const currentIcon = form.getValues('icon')?.trim()
                      const currentName = form.getValues('name')?.trim()
                      const previousOption = getPaymentTypeOption(field.value)
                      const nextOption = getPaymentTypeOption(value)

                      field.onChange(value)
                      if (
                        nextOption?.iconName &&
                        (!currentIcon ||
                          currentIcon === previousOption?.iconName)
                      ) {
                        form.setValue('icon', nextOption.iconName, {
                          shouldDirty: true,
                        })
                      }
                      if (
                        nextOption?.name &&
                        (!currentName || currentName === previousOption?.name)
                      ) {
                        form.setValue('name', nextOption.name, {
                          shouldDirty: true,
                        })
                      }
                    }}
                    placeholder={t('Select or enter payment type')}
                    searchPlaceholder={t('Search payment types...')}
                  />
                </FormControl>
                <FormDescription className='leading-relaxed'>
                  {t(
                    'Used to decide the payment flow. Built-in keys include stripe for Stripe and waffo_pancake for Waffo Pancake; other values are sent to Epay as the type parameter.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='description'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Description')}</FormLabel>
                <FormControl>
                  <Input
                    placeholder={t('Optional payment method description')}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t('Shown to users below the payment method name.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {isManualTransfer && (
            <FormField
              control={form.control}
              name='contact_url'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Messenger link')}</FormLabel>
                  <FormControl>
                    <Input
                      type='url'
                      placeholder='https://t.me/manager'
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Supported HTTPS hosts: Telegram, WhatsApp, Messenger, Discord, VK.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}

          <FormField
            control={form.control}
            name='icon'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Icon')}</FormLabel>
                <FormControl>
                  <div className='flex items-center gap-2'>
                    <Popover>
                      <PopoverTrigger
                        render={
                          <Button
                            type='button'
                            variant='outline'
                            className='w-full justify-start gap-2'
                          />
                        }
                      >
                        {iconValue ? (
                          <ReactIconByName
                            name={iconValue}
                            className='size-5'
                          />
                        ) : (
                          <span className='text-muted-foreground'>
                            {t('Choose an icon')}
                          </span>
                        )}
                        {iconValue && <span>{iconValue}</span>}
                      </PopoverTrigger>
                      <PopoverContent className='w-72'>
                        <div className='grid grid-cols-5 gap-2'>
                          {availableIcons.map((iconName) => (
                            <Button
                              key={iconName}
                              type='button'
                              variant={
                                iconValue === iconName ? 'default' : 'outline'
                              }
                              size='icon'
                              title={iconName}
                              aria-label={iconName}
                              onClick={() => field.onChange(iconName)}
                            >
                              <ReactIconByName
                                name={iconName}
                                className='size-5'
                              />
                            </Button>
                          ))}
                        </div>
                      </PopoverContent>
                    </Popover>
                  </div>
                </FormControl>
                <FormDescription>
                  {t('Choose an icon from the curated library.')}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          {(hasEditableMinimum || isDirectCrypto) && (
            <FormField
              control={form.control}
              name='min_topup'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>
                    {t('Minimum top-up (optional)')} ({minimumCurrency})
                  </FormLabel>
                  <FormControl>
                    <Input
                      type='number'
                      min={minimumInputFloor}
                      step='0.01'
                      placeholder={isDirectCrypto ? '10' : t('e.g., 50')}
                      {...field}
                    />
                  </FormControl>
                  <FormDescription>
                    {t('Optional minimum recharge amount for this method.')} (
                    {minimumCurrency})
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <FormField
            control={form.control}
            name='topup_group'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Top-up coefficient group')} *</FormLabel>
                <FormControl>
                  <Combobox
                    options={topupGroups
                      .filter((group) => group.name.trim())
                      .map((group) => ({
                        label: `${group.name} (${group.ratio})`,
                        value: group.name,
                      }))}
                    value={field.value}
                    onValueChange={(value) => field.onChange(value ?? '')}
                    placeholder={t('Use user group coefficient')}
                    searchPlaceholder={t('Search top-up groups...')}
                  />
                </FormControl>
                <FormDescription>
                  {t('Select a configured top-up coefficient group')}.{' '}
                  {t(
                    'Commission coefficient: values up to 1 have no commission; values above 1 are shown as a percentage commission.'
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='pending_ttl_minutes'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Payment waiting TTL (minutes)')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    min='1'
                    step='1'
                    placeholder={t('Default: {{minutes}} minutes', {
                      minutes: effectiveDefaultPendingTtlMinutes,
                    })}
                    {...field}
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Leave blank to use the default payment waiting TTL of {{minutes}} minutes.',
                    { minutes: effectiveDefaultPendingTtlMinutes }
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='admin_only'
            render={({ field }) => (
              <FormItem className='flex items-start gap-3 space-y-0 rounded-md border p-3'>
                <FormControl>
                  <Checkbox
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
                <div className='space-y-1'>
                  <FormLabel>{t('Only for admins')}</FormLabel>
                  <FormDescription>
                    {t('Regular users will not see this payment method.')}
                  </FormDescription>
                </div>
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}

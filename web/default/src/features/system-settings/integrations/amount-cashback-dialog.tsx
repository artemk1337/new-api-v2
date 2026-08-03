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
import { useEffect, useMemo } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import * as z from 'zod'

import { Dialog } from '@/components/dialog'
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

const createAmountCashbackDialogSchema = (t: (key: string) => string) =>
  z.object({
    amount: z
      .number()
      .nonnegative(t('Amount must be 0 or greater'))
      .finite(t('Amount must be a finite number')),
    cashbackPercent: z
      .number()
      .min(0, t('Cashback percentage must be 0 or greater'))
      .max(100, t('Cashback percentage must be ≤ 100')),
  })

type AmountCashbackDialogFormValues = z.infer<
  ReturnType<typeof createAmountCashbackDialogSchema>
>

const AMOUNT_CASHBACK_FORM_ID = 'amount-cashback-form'

export type AmountCashbackData = {
  amount: number
  cashbackPercent: number
}

type AmountCashbackDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSave: (data: AmountCashbackData) => void
  editData?: AmountCashbackData | null
  tokenAmounts: boolean
}

export function AmountCashbackDialog({
  open,
  onOpenChange,
  onSave,
  editData,
  tokenAmounts,
}: AmountCashbackDialogProps) {
  const { t } = useTranslation()
  const isEditMode = !!editData
  const amountCashbackDialogSchema = createAmountCashbackDialogSchema(t)

  const form = useForm<AmountCashbackDialogFormValues>({
    resolver: zodResolver(amountCashbackDialogSchema),
    defaultValues: {
      amount: 0,
      cashbackPercent: 0,
    },
  })

  const cashbackPercent = form.watch('cashbackPercent')

  const cashbackPreview = useMemo(() => cashbackPercent || 0, [cashbackPercent])

  useEffect(() => {
    if (editData) {
      form.reset(editData)
    } else {
      form.reset({
        amount: 0,
        cashbackPercent: 0,
      })
    }
  }, [editData, form, open])

  const handleSubmit = (values: AmountCashbackDialogFormValues) => {
    onSave({
      amount: values.amount,
      cashbackPercent: values.cashbackPercent,
    })
    form.reset()
    onOpenChange(false)
  }

  return (
    <Dialog
      open={open}
      onOpenChange={onOpenChange}
      title={isEditMode ? t('Edit cashback tier') : t('Add cashback tier')}
      description={t(
        'Set a cashback percentage for a specific recharge amount threshold.'
      )}
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
          <Button type='submit' form={AMOUNT_CASHBACK_FORM_ID}>
            {isEditMode ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id={AMOUNT_CASHBACK_FORM_ID}
          onSubmit={form.handleSubmit(handleSubmit)}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='amount'
            render={({ field }) => (
              <FormItem>
                <FormLabel>
                  {tokenAmounts
                    ? t('Recharge Amount (Tokens)')
                    : t('Recharge Amount (USD)')}
                </FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    step='any'
                    min='0'
                    placeholder={t('e.g., 100')}
                    {...field}
                    onChange={(e) =>
                      field.onChange(Number.parseFloat(e.target.value) || 0)
                    }
                    disabled={isEditMode}
                  />
                </FormControl>
                <FormDescription>
                  {isEditMode
                    ? t('Amount cannot be changed when editing.')
                    : t(
                        'Minimum recharge amount to qualify for this cashback.'
                      )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />

          <FormField
            control={form.control}
            name='cashbackPercent'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Cashback Percentage')}</FormLabel>
                <FormControl>
                  <Input
                    type='number'
                    step='0.01'
                    min='0'
                    max='100'
                    placeholder={t('e.g., 1')}
                    {...field}
                    onChange={(e) =>
                      field.onChange(Number.parseFloat(e.target.value) || 0)
                    }
                  />
                </FormControl>
                <FormDescription>
                  {t(
                    'Percentage credited to the balance after a successful topup.'
                  )}
                  {cashbackPreview > 0 && (
                    <span className='ml-1 font-medium text-green-600 dark:text-green-400'>
                      = {cashbackPreview}%
                    </span>
                  )}
                </FormDescription>
                <FormMessage />
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}

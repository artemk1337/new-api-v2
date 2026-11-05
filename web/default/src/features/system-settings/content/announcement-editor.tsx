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
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { z } from 'zod'

import { DateTimePicker } from '@/components/datetime-picker'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { isTelegramPostUrl } from '@/lib/announcement-telegram'

import type { Announcement } from './announcements-api'

export function AnnouncementEditor(props: {
  announcement: Announcement | null
  onClose: () => void
  onSubmit: (announcement: Announcement) => Promise<void>
}) {
  const { t } = useTranslation()
  const schema = z
    .object({
      content: z
        .string()
        .trim()
        .min(1, t('Content is required'))
        .refine(
          (content) => [...content].length <= 5000,
          t('Announcement content must not exceed 5000 characters')
        ),
      publishDate: z
        .string()
        .datetime({ offset: true, message: t('Publish date is required') }),
      extra: z.string().max(100, t('Extra must be less than 100 characters')),
      telegramMode: z.enum(['platform', 'link', 'bot']),
      telegramText: z.string(),
      telegramUrl: z.string(),
    })
    .superRefine((values, ctx) => {
      if (
        values.telegramMode === 'bot' &&
        (!values.telegramText.trim() || values.telegramText.length > 4096)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['telegramText'],
          message: t('Enter Telegram text between 1 and 4096 characters'),
        })
      }
      if (
        values.telegramMode === 'link' &&
        !isTelegramPostUrl(values.telegramUrl)
      ) {
        ctx.addIssue({
          code: 'custom',
          path: ['telegramUrl'],
          message: t('Enter a public Telegram post link'),
        })
      }
    })
  type Values = z.infer<typeof schema>
  const form = useForm<Values>({
    resolver: zodResolver(schema),
    defaultValues: {
      content: props.announcement?.content || '',
      publishDate: props.announcement?.publishDate || new Date().toISOString(),
      extra: props.announcement?.extra || '',
      telegramMode: props.announcement?.telegramMode || 'platform',
      telegramText: props.announcement?.telegramText || '',
      telegramUrl: props.announcement?.telegramUrl || '',
    },
  })
  const mode = useWatch({ control: form.control, name: 'telegramMode' })
  const text = useWatch({ control: form.control, name: 'telegramText' })
  const modes = [
    { value: 'platform', label: t('Platform only') },
    { value: 'link', label: t('Link to a Telegram post') },
    { value: 'bot', label: t('Publish with Telegram bot') },
  ]

  async function submit(values: Values) {
    await props.onSubmit({
      ...props.announcement,
      ...values,
      id: props.announcement?.id || crypto.randomUUID(),
      type: props.announcement?.type || 'default',
      telegramUrl: values.telegramMode === 'platform' ? '' : values.telegramUrl,
    })
  }

  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !form.formState.isSubmitting) props.onClose()
      }}
      title={
        props.announcement ? t('Edit Announcement') : t('Add Announcement')
      }
      contentClassName='max-w-2xl'
      contentHeight='auto'
      bodyClassName='space-y-4'
      footer={
        <>
          <Button
            variant='outline'
            disabled={form.formState.isSubmitting}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button
            type='submit'
            form='announcement-form'
            disabled={form.formState.isSubmitting}
          >
            {props.announcement ? t('Update') : t('Add')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form id='announcement-form' onSubmit={form.handleSubmit(submit)}>
          <fieldset
            disabled={form.formState.isSubmitting}
            className='space-y-4'
          >
            <FormField
              control={form.control}
              name='content'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Platform announcement')}</FormLabel>
                  <FormControl>
                    <Textarea rows={4} {...field} />
                  </FormControl>
                  <FormDescription>
                    {t('Maximum 5000 characters. Supports Markdown and HTML.')}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='publishDate'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Publish Date')}</FormLabel>
                  <FormControl>
                    <DateTimePicker
                      value={field.value ? new Date(field.value) : undefined}
                      onChange={(date) =>
                        field.onChange(date?.toISOString() || '')
                      }
                      placeholder={t('Select publish date')}
                    />
                  </FormControl>
                  <FormDescription>
                    {t(
                      'Publication starts when the announcement is saved. This date is a label.'
                    )}
                  </FormDescription>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='extra'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Extra Notes (Optional)')}</FormLabel>
                  <FormControl>
                    <Input {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />
            <FormField
              control={form.control}
              name='telegramMode'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Telegram publication')}</FormLabel>
                  <Select
                    items={modes}
                    value={field.value}
                    onValueChange={field.onChange}
                  >
                    <FormControl>
                      <SelectTrigger className='w-full'>
                        <SelectValue />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {modes.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />
            {mode === 'link' && (
              <FormField
                control={form.control}
                name='telegramUrl'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Telegram post link')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder='https://t.me/channel/123'
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
            {mode === 'bot' && (
              <FormField
                control={form.control}
                name='telegramText'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Telegram post text')}</FormLabel>
                    <FormControl>
                      <Textarea rows={7} {...field} />
                    </FormControl>
                    <FormDescription className='flex flex-wrap justify-between gap-2'>
                      <span>
                        {t('Plain text, links and emoji. No attachments.')}
                      </span>
                      <span
                        className={text.length > 4096 ? 'text-destructive' : ''}
                      >
                        {text.length} / 4096
                      </span>
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}
          </fieldset>
        </form>
      </Form>
    </Dialog>
  )
}

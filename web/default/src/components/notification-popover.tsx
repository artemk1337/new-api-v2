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
import '@/styles/notifications.css'
import { Bell, Megaphone, X } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { AnnouncementContent } from '@/components/announcement-content'
import { AnnouncementTelegramLink } from '@/components/announcement-telegram-link'
import { RichContent } from '@/components/rich-content'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import {
  Popover,
  PopoverContent,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface AnnouncementItem {
  isUnread?: boolean
  id?: number | string
  type?: string
  content?: string
  extra?: string
  publishDate?: string | Date
  telegramUrl?: string
}

interface NotificationPopoverProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  unreadCount: number
  announcements: AnnouncementItem[]
  loading: boolean
  className?: string
}

function getPublishDate(item: AnnouncementItem): Date | null {
  if (!item.publishDate) return null
  const date = new Date(item.publishDate)
  return Number.isNaN(date.getTime()) ? null : date
}

function getAnnouncementRenderKey(item: AnnouncementItem): string {
  if (item.id !== undefined && item.id !== null) return `id:${item.id}`
  return JSON.stringify(item)
}

export function NotificationPopover(props: NotificationPopoverProps) {
  const { t, i18n } = useTranslation()
  const locale = i18n.resolvedLanguage || i18n.language
  const monthFormat = new Intl.DateTimeFormat(locale, {
    month: 'long',
    year: 'numeric',
  })
  const dayFormat = new Intl.DateTimeFormat(locale, {
    day: 'numeric',
    month: 'long',
  })
  const announcements = [...props.announcements].sort(
    (a, b) =>
      (getPublishDate(b)?.getTime() ?? 0) - (getPublishDate(a)?.getTime() ?? 0)
  )

  return (
    <Popover open={props.open} onOpenChange={props.onOpenChange}>
      <PopoverTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            className={cn('relative size-9', props.className)}
            aria-label={t('Notifications')}
          />
        }
      >
        <Bell className='size-[1.2rem]' />
        {props.unreadCount > 0 ? (
          <Badge
            variant='destructive'
            className='absolute -top-1 -right-1 flex h-5 min-w-5 items-center justify-center px-1 text-[10px] font-semibold tabular-nums'
          >
            {props.unreadCount > 99 ? '99+' : props.unreadCount}
          </Badge>
        ) : null}
      </PopoverTrigger>
      <PopoverContent
        align='end'
        sideOffset={12}
        collisionPadding={8}
        className='updates-popover w-[min(37rem,calc(100vw-1rem))] gap-0 overflow-hidden rounded-2xl p-0 shadow-xl'
      >
        <PopoverHeader className='flex-row items-center justify-between border-b px-5 py-4 sm:px-6'>
          <PopoverTitle className='text-xl font-semibold tracking-tight'>
            {t('Platform updates')}
          </PopoverTitle>
          <Button
            variant='ghost'
            size='icon'
            aria-label={t('Close')}
            onClick={() => props.onOpenChange(false)}
          >
            <X className='text-muted-foreground size-4' />
          </Button>
        </PopoverHeader>
        <div className='max-h-[min(70dvh,36rem,calc(var(--available-height)-5rem))] overflow-y-auto overscroll-contain px-5 pb-5 sm:px-6'>
          {props.loading || announcements.length === 0 ? (
            <Empty className='min-h-48 border-0'>
              <EmptyHeader>
                <EmptyMedia variant='icon'>
                  <Megaphone />
                </EmptyMedia>
                <EmptyTitle>
                  {props.loading
                    ? t('Loading...')
                    : t('No system announcements')}
                </EmptyTitle>
              </EmptyHeader>
            </Empty>
          ) : (
            <ol>
              {announcements.map((item, index) => {
                const date = getPublishDate(item)
                const month = date ? monthFormat.format(date) : ''
                const previousDate =
                  index > 0 ? getPublishDate(announcements[index - 1]) : null
                const startsMonth =
                  month !==
                  (previousDate ? monthFormat.format(previousDate) : '')
                return (
                  <li
                    key={getAnnouncementRenderKey(item)}
                    className='update-item'
                  >
                    {startsMonth ? (
                      <h3
                        className={cn(
                          'text-muted-foreground pt-3 pb-4 text-xs font-medium capitalize',
                          index > 0 && 'border-t mt-2 pt-5'
                        )}
                      >
                        {month}
                      </h3>
                    ) : null}
                    <article
                      data-unread={item.isUnread || undefined}
                      className={cn(
                        'update-article min-w-0',
                        item.isUnread && 'update-unread'
                      )}
                    >
                      <div className='text-muted-foreground mb-2 flex flex-wrap items-center gap-2 text-xs'>
                        {date ? (
                          <time dateTime={date.toISOString()}>
                            {dayFormat.format(date)}
                          </time>
                        ) : null}
                        {item.isUnread ? (
                          <span className='update-new inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-[11px] font-semibold'>
                            <span className='size-1.5 rounded-full bg-current' />
                            {t('Unread update')}
                          </span>
                        ) : null}
                        {item.type === 'ongoing' ? (
                          <Badge variant='secondary'>{t('Ongoing')}</Badge>
                        ) : null}
                        {item.type === 'warning' ? (
                          <Badge variant='secondary'>{t('Important')}</Badge>
                        ) : null}
                        {item.type === 'error' ? (
                          <Badge variant='destructive'>{t('Error')}</Badge>
                        ) : null}
                      </div>
                      <AnnouncementContent content={item.content || ''} />
                      <AnnouncementTelegramLink url={item.telegramUrl} />
                      {item.extra ? (
                        <RichContent
                          breaks
                          content={item.extra}
                          className='text-muted-foreground mt-2 text-sm leading-relaxed'
                        />
                      ) : null}
                    </article>
                  </li>
                )
              })}
            </ol>
          )}
        </div>
      </PopoverContent>
    </Popover>
  )
}

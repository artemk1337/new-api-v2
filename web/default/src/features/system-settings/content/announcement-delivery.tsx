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
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { isTelegramPostUrl } from '@/lib/announcement-telegram'

import type { Announcement } from './announcements-api'

export function AnnouncementDelivery(props: {
  announcement: Announcement
  disabled: boolean
  onRetry: (announcement: Announcement) => void
}) {
  const { t } = useTranslation()
  const item = props.announcement
  if (item.telegramMode === 'platform') {
    return <span className='text-muted-foreground'>{t('Platform only')}</span>
  }
  const statuses: Record<string, string> = {
    pending: t('Telegram publication queued'),
    sending: t('Publishing to Telegram'),
    sent: t('Published to Telegram'),
    failed: t('Telegram publication failed'),
    unknown: t('Telegram publication result unknown'),
    cancelled: t('Telegram publication cancelled'),
  }
  const canRetry =
    item.telegramMode === 'bot' &&
    ['failed', 'unknown'].includes(item.telegramStatus || '')
  return (
    <div className='flex max-w-xs flex-col items-start gap-1.5 text-sm'>
      {item.telegramMode === 'bot' && (
        <span role='status'>
          {statuses[item.telegramStatus || ''] ||
            t('Telegram publication after saving')}
        </span>
      )}
      {isTelegramPostUrl(item.telegramUrl) && (
        <a
          href={item.telegramUrl}
          target='_blank'
          rel='noopener noreferrer'
          className='text-primary underline underline-offset-4'
        >
          {t('More in Telegram')}
        </a>
      )}
      {item.telegramMode === 'bot' && item.telegramError && (
        <span className='text-destructive break-words'>
          {t(item.telegramError)}
        </span>
      )}
      {canRetry && (
        <Button
          variant='outline'
          size='sm'
          disabled={props.disabled}
          onClick={() => props.onRetry(item)}
        >
          {t('Retry Telegram publication')}
        </Button>
      )}
    </div>
  )
}

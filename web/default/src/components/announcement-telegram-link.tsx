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
import { ArrowUpRight } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { isTelegramPostUrl } from '@/lib/announcement-telegram'

export function AnnouncementTelegramLink(props: { url?: string }) {
  const { t } = useTranslation()
  if (!isTelegramPostUrl(props.url)) return null

  return (
    <Button
      variant='outline'
      size='sm'
      className='mt-3'
      render={<a href={props.url} target='_blank' rel='noopener noreferrer' />}
    >
      {t('More in Telegram')}
      <ArrowUpRight aria-hidden='true' />
    </Button>
  )
}

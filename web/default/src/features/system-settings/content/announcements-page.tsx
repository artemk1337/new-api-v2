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
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { SectionPageLayout } from '@/components/layout'
import { getOptionValue, useSystemOptions } from '../hooks/use-system-options'
import { AnnouncementsSection } from './announcements-section'

const defaultAnnouncementSettings = {
  'console_setting.announcements': '[]',
  'console_setting.announcements_enabled': true,
}

export function AnnouncementsPage() {
  const { t } = useTranslation()
  const { data, isLoading } = useSystemOptions()

  const settings = useMemo(() => {
    const settings = getOptionValue(data?.data, defaultAnnouncementSettings)
    const hasCurrentAnnouncements = data?.data?.some(
      (option) => option.key === 'console_setting.announcements'
    )

    if (hasCurrentAnnouncements) return settings

    const legacyAnnouncements = data?.data?.find(
      (option) => option.key === 'Announcements'
    )

    return legacyAnnouncements
      ? {
          ...settings,
          'console_setting.announcements': legacyAnnouncements.value,
        }
      : settings
  }, [data?.data])

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('News')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        {isLoading ? (
          <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
            {t('Loading announcements...')}
          </div>
        ) : (
          <AnnouncementsSection
            enabled={settings['console_setting.announcements_enabled']}
            data={settings['console_setting.announcements']}
          />
        )}
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

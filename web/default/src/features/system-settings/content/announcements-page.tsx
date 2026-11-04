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

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'

import { getOptionValue, useSystemOptions } from '../hooks/use-system-options'
import { useAnnouncementSettings } from './announcements-api'
import { AnnouncementsSection } from './announcements-section'

export function AnnouncementsPage() {
  const { t } = useTranslation()
  const options = useSystemOptions()
  const settings = useAnnouncementSettings()
  const enabled = getOptionValue(options.data?.data, {
    'console_setting.announcements_enabled': true,
  })['console_setting.announcements_enabled']

  function renderContent() {
    if (settings.isError && !settings.data) {
      return (
        <div role='alert' className='space-y-3'>
          <p>{t('Failed to load announcements')}</p>
          <Button variant='outline' onClick={() => void settings.refetch()}>
            {t('Retry')}
          </Button>
        </div>
      )
    }
    if (settings.isLoading || options.isLoading) {
      return (
        <div className='text-muted-foreground flex min-h-40 items-center justify-center text-sm'>
          {t('Loading announcements...')}
        </div>
      )
    }
    if (!settings.data) return null
    return (
      <AnnouncementsSection
        enabled={enabled}
        settings={settings.data}
        onRefresh={async () => {
          const result = await settings.refetch()
          if (result.isError) throw result.error
        }}
      />
    )
  }
  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('News')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>{renderContent()}</SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

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

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'
import { cn } from '@/lib/utils'

import apiKeysMarkdown from './content/api-keys.md?raw'
import { DOCS_SECTIONS } from './docs-sections'

function SectionLinks(props: { className?: string }) {
  const { t } = useTranslation()

  return (
    <nav
      aria-label={t('Guide sections')}
      className={cn('space-y-0.5', props.className)}
    >
      {DOCS_SECTIONS.map((section) => (
        <a
          key={section.id}
          href={`#${section.id}`}
          className='text-muted-foreground hover:bg-muted hover:text-foreground block rounded-md px-3 py-1.5 text-sm transition-colors'
        >
          {section.title}
        </a>
      ))}
    </nav>
  )
}

function MobileSectionLinks() {
  const { t } = useTranslation()

  return (
    <details className='border-border bg-card mb-6 rounded-lg border p-3 lg:hidden'>
      <summary className='cursor-pointer text-sm font-semibold'>
        {t('Guide navigation')}
      </summary>
      <SectionLinks className='mt-3' />
    </details>
  )
}

export function DocsPage() {
  const { t } = useTranslation()

  return (
    <PublicLayout>
      <MobileSectionLinks />
      <div className='mx-auto grid max-w-6xl items-start gap-8 lg:grid-cols-[14rem_minmax(0,1fr)]'>
        <aside className='sticky top-24 hidden max-h-[calc(100vh-7rem)] overflow-y-auto lg:block'>
          <p className='text-muted-foreground mb-2 px-3 text-xs font-semibold tracking-wide uppercase'>
            {t('Guide sections')}
          </p>
          <SectionLinks />
        </aside>
        <article className='min-w-0 rounded-xl'>
          <RichContent
            mode='markdown'
            content={apiKeysMarkdown}
            className='prose-neutral dark:prose-invert max-w-none'
          />
        </article>
      </div>
    </PublicLayout>
  )
}

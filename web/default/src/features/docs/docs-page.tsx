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
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PublicLayout } from '@/components/layout'
import { RichContent } from '@/components/rich-content'

import { useApplicationGuides } from './application-guides'
import { ApplicationArticle } from './applications-guide'
import {
  CLAUDE_SECTION_IDS,
  guideSections,
  withoutGuideHeading,
} from './docs-content'
import { DocsNavigation } from './docs-navigation'
import { DOCS_SECTIONS } from './docs-sections'
import { QuickStartGuide } from './quick-start-guide'

const LEGACY_DOCUMENTS: Record<string, string> = {
  'initial-setup': 'quick-start',
  'desktop-apps': 'desktop-cherry-studio',
  'mobile-apps': 'mobile-chatbox',
  'codex-cli': 'desktop-codex-cli',
  'openai-compatible': 'desktop-continue',
  cursor: 'desktop-cursor',
}

export function DocsPage() {
  const { t } = useTranslation()
  const apps = useApplicationGuides()
  const [hash, setHash] = useState(() => window.location.hash.slice(1))
  const articleRef = useRef<HTMLHeadingElement>(null)
  const mobileMenuRef = useRef<HTMLDetailsElement>(null)
  const requestedId = LEGACY_DOCUMENTS[hash] ?? hash
  const desktopApp = apps.desktop.find(
    (entry) => `desktop-${entry.id}` === requestedId
  )
  const mobileApp = apps.mobile.find(
    (entry) => `mobile-${entry.id}` === requestedId
  )
  const app = desktopApp ?? mobileApp
  const claudeSection = CLAUDE_SECTION_IDS.includes(requestedId)
  const section = DOCS_SECTIONS.find(
    (entry) => entry.id === requestedId && guideSections[entry.id]
  )
  const selectedId =
    app || claudeSection || section ? requestedId : 'quick-start'
  const selectedApp = claudeSection
    ? apps.desktop.find((entry) => entry.id === 'claude-code')
    : app
  let title = section?.title.replace(/^\d+\.\s*/, '') ?? t('Guide sections')
  let category =
    selectedId === 'quick-start' ? t('Connection guide') : t('General guide')
  if (selectedApp) {
    title = claudeSection ? title : selectedApp.name
    category = `${t('Application setup')} / ${selectedId.startsWith('mobile-') ? t('Mobile apps') : t('Desktop apps')}`
  } else if (!section) {
    title = DOCS_SECTIONS[0].title
  }

  useEffect(() => {
    function readHash(): void {
      setHash(window.location.hash.slice(1))
    }
    window.addEventListener('hashchange', readHash)
    return () => window.removeEventListener('hashchange', readHash)
  }, [])

  useEffect(() => {
    articleRef.current?.scrollIntoView({ block: 'start' })
    if (mobileMenuRef.current) mobileMenuRef.current.open = false
  }, [selectedId])

  function closeMobileMenu(): void {
    if (mobileMenuRef.current) mobileMenuRef.current.open = false
    articleRef.current?.focus({ preventScroll: true })
    articleRef.current?.scrollIntoView({ block: 'start' })
  }

  const generalContent = guideSections[selectedId] ?? ''
  let article = (
    <RichContent
      mode='markdown'
      content={withoutGuideHeading(generalContent)}
      className='prose-neutral dark:prose-invert max-w-none'
    />
  )
  if (selectedId === 'quick-start') {
    article = <QuickStartGuide />
  } else if (selectedApp) {
    article = (
      <ApplicationArticle
        app={selectedApp}
        content={claudeSection ? guideSections[selectedId] : undefined}
      />
    )
  }

  const navigation = (
    <DocsNavigation
      desktop={apps.desktop}
      mobile={apps.mobile}
      selectedId={selectedId}
      onNavigate={closeMobileMenu}
    />
  )

  return (
    <PublicLayout>
      <div className='mx-auto grid max-w-6xl items-start gap-8 lg:grid-cols-[16rem_minmax(0,1fr)]'>
        <aside className='sticky top-24 hidden max-h-[calc(100vh-7rem)] overflow-y-auto pr-3 lg:block'>
          {navigation}
        </aside>
        <div className='min-w-0'>
          <details
            ref={mobileMenuRef}
            className='border-border bg-card mb-6 rounded-lg border p-3 lg:hidden'
          >
            <summary className='cursor-pointer text-sm font-semibold'>
              {t('Guide navigation')}
            </summary>
            <div className='mt-3'>{navigation}</div>
          </details>
          <article
            aria-labelledby='docs-article-title'
            className='min-w-0 pb-10'
          >
            <p className='text-muted-foreground mb-3 text-xs'>{category}</p>
            <h1
              id='docs-article-title'
              ref={articleRef}
              tabIndex={-1}
              className='mb-6 scroll-mt-24 text-2xl font-semibold tracking-tight outline-none sm:text-3xl'
            >
              {title}
            </h1>
            {article}
          </article>
        </div>
      </div>
    </PublicLayout>
  )
}

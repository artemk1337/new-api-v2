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
import {
  AppWindow,
  BookOpen,
  ChevronRight,
  Monitor,
  Lightbulb,
  Rocket,
  Smartphone,
  type LucideIcon,
} from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { cn } from '@/lib/utils'

import type { ApplicationGuide } from './application-guides'
import {
  CLAUDE_SECTION_IDS,
  DESKTOP_SECTION_IDS,
  INSTALLATION_SECTIONS,
} from './docs-content'
import { DOCS_SECTIONS } from './docs-sections'
import type { RecommendationGuide } from './recommendation-guides'

type DocsNavigationProps = {
  desktop: ApplicationGuide[]
  mobile: ApplicationGuide[]
  recommendations: RecommendationGuide[]
  selectedId: string
  onNavigate?: () => void
}

function DocsNavGroup(props: {
  title: string
  icon?: LucideIcon
  active: boolean
  children: ReactNode
}) {
  return (
    <Collapsible defaultOpen={props.active}>
      <CollapsibleTrigger className='group/docs-trigger text-muted-foreground hover:text-foreground hover:bg-muted/60 focus-visible:ring-ring flex w-full items-center gap-2.5 rounded-lg px-2.5 py-2 text-left text-[13px] font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none'>
        {props.icon && (
          <props.icon
            className='size-4 shrink-0 opacity-70'
            aria-hidden='true'
          />
        )}
        <span className='min-w-0 flex-1 leading-5'>{props.title}</span>
        <ChevronRight
          className='size-3.5 shrink-0 opacity-50 transition-transform duration-300 ease-out group-data-[panel-open]/docs-trigger:rotate-90 motion-reduce:transition-none'
          aria-hidden='true'
        />
      </CollapsibleTrigger>
      <CollapsibleContent className='CollapsibleContent'>
        <div className='border-border/60 my-1 ml-4 space-y-0.5 border-l pl-2'>
          {props.children}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

export function DocsNavigation(props: DocsNavigationProps) {
  const { t } = useTranslation()
  const claudeLinks = [
    { id: 'claude-code', title: t('Connection') },
    { id: 'claude-code-vscode', title: 'VsCode' },
  ]

  const generalSections = DOCS_SECTIONS.filter(
    (section) =>
      section.id !== 'quick-start' && !DESKTOP_SECTION_IDS.includes(section.id)
  )

  const claudeActive = CLAUDE_SECTION_IDS.includes(props.selectedId)
  const desktopActive =
    claudeActive ||
    props.desktop.some((app) => `desktop-${app.id}` === props.selectedId)
  const mobileActive = props.mobile.some(
    (app) => `mobile-${app.id}` === props.selectedId
  )
  const generalActive = generalSections.some(
    (section) => section.id === props.selectedId
  )

  function sectionLink(id: string, title: string) {
    return (
      <a
        key={id}
        href={`#${id}`}
        aria-current={props.selectedId === id ? 'page' : undefined}
        onClick={props.onNavigate}
        className={cn(
          'hover:bg-muted/60 hover:text-foreground focus-visible:ring-ring flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-[13px] leading-5 transition-colors focus-visible:ring-2 focus-visible:outline-none',
          props.selectedId === id
            ? 'bg-muted text-foreground font-medium'
            : 'text-muted-foreground'
        )}
      >
        {id === 'quick-start' && (
          <Rocket className='size-4 shrink-0 opacity-70' aria-hidden='true' />
        )}
        {title.replace(/^\d+\.\s*/, '')}
      </a>
    )
  }

  return (
    <nav
      key={props.selectedId}
      aria-label={t('Guide sections')}
      className='bg-card/40 space-y-1 rounded-xl border p-2'
    >
      {sectionLink('quick-start', DOCS_SECTIONS[0].title)}
      <DocsNavGroup
        title={t('Application setup')}
        icon={AppWindow}
        active={desktopActive || mobileActive}
      >
        <DocsNavGroup
          title={t('Desktop apps')}
          icon={Monitor}
          active={desktopActive}
        >
          {props.desktop.map((app) => {
            if (app.id === 'claude-code') {
              return (
                <DocsNavGroup
                  key={app.id}
                  title={app.name}
                  active={claudeActive}
                >
                  <DocsNavGroup
                    title={t('Installation')}
                    active={
                      props.selectedId === 'claude-code-install' ||
                      INSTALLATION_SECTIONS.some(
                        (section) => section.id === props.selectedId
                      )
                    }
                  >
                    {INSTALLATION_SECTIONS.map((section) =>
                      sectionLink(section.id, section.title)
                    )}
                  </DocsNavGroup>
                  {claudeLinks.map((section) =>
                    sectionLink(section.id, section.title)
                  )}
                </DocsNavGroup>
              )
            }
            return sectionLink(`desktop-${app.id}`, app.name)
          })}
        </DocsNavGroup>
        <DocsNavGroup
          title={t('Mobile apps')}
          icon={Smartphone}
          active={mobileActive}
        >
          {props.mobile.map((app) => sectionLink(`mobile-${app.id}`, app.name))}
        </DocsNavGroup>
      </DocsNavGroup>
      <DocsNavGroup
        title={t('Recommendations')}
        icon={Lightbulb}
        active={props.recommendations.some(
          (guide) => guide.id === props.selectedId
        )}
      >
        {props.recommendations
          .filter((guide) => !guide.group)
          .map((guide) => sectionLink(guide.id, guide.title))}
        <DocsNavGroup
          title={t('Code indexers')}
          active={props.recommendations.some(
            (guide) =>
              guide.group === 'indexers' && guide.id === props.selectedId
          )}
        >
          {props.recommendations
            .filter((guide) => guide.group === 'indexers')
            .map((guide) => sectionLink(guide.id, guide.title))}
        </DocsNavGroup>
      </DocsNavGroup>
      <DocsNavGroup
        title={t('General guide')}
        icon={BookOpen}
        active={generalActive}
      >
        {generalSections.map((section) =>
          sectionLink(section.id, section.title)
        )}
      </DocsNavGroup>
    </nav>
  )
}

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
import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { RichContent } from '@/components/rich-content'
import { buttonVariants } from '@/components/ui/button'

import type { ApplicationGuide } from './application-guides'
import { withoutGuideHeading } from './docs-content'

export function ApplicationArticle(props: {
  app: ApplicationGuide
  content?: string
}) {
  const { t } = useTranslation()
  return (
    <div>
      <div className='mb-5 flex flex-wrap items-center justify-between gap-3'>
        <p className='text-muted-foreground text-sm'>{props.app.platform}</p>
        <a
          href={props.app.download}
          target='_blank'
          rel='noopener noreferrer'
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          {t('Download app')}
          <ExternalLink className='size-3.5' aria-hidden='true' />
        </a>
      </div>
      {props.app.steps.length > 0 && (
        <ol className='mb-6 space-y-3'>
          {props.app.steps.map((step, index) => (
            <li key={step} className='flex gap-3 text-sm leading-6'>
              <span
                className='bg-muted text-muted-foreground mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-xs font-medium'
                aria-hidden='true'
              >
                {index + 1}
              </span>
              <span>{step}</span>
            </li>
          ))}
        </ol>
      )}
      {(props.content || props.app.content) && (
        <RichContent
          mode='markdown'
          content={withoutGuideHeading(
            props.content ?? props.app.content ?? ''
          )}
          className='prose-neutral dark:prose-invert mb-6 max-w-none'
        />
      )}
      {props.app.fields.length > 0 && (
        <ConnectionSettings fields={props.app.fields} />
      )}
      <div className='text-muted-foreground mt-4 space-y-3 text-xs leading-5'>
        {props.app.note && <p>{props.app.note}</p>}
        <a
          className='hover:text-foreground inline-flex items-center gap-1 underline underline-offset-4'
          href={props.app.source}
          target='_blank'
          rel='noopener noreferrer'
        >
          {t('Official setup guide')}
          <ExternalLink className='size-3' aria-hidden='true' />
        </a>
      </div>
    </div>
  )
}

export function ConnectionSettings(props: {
  fields: ApplicationGuide['fields']
}) {
  const { t } = useTranslation()
  return (
    <dl className='overflow-hidden rounded-lg border'>
      {props.fields.map((field) => (
        <div
          key={field.label}
          className='grid gap-1 border-b px-3 py-2.5 last:border-b-0 sm:grid-cols-[9rem_minmax(0,1fr)] sm:items-center sm:gap-4'
        >
          <dt className='text-muted-foreground text-xs'>{field.label}</dt>
          <dd className='flex min-w-0 items-center justify-between gap-2 text-sm'>
            <code className='min-w-0 text-xs leading-5 wrap-anywhere'>
              {field.value}
            </code>
            {(field.value.startsWith('https://') ||
              field.value.startsWith('/')) && (
              <CopyButton
                value={field.value}
                size='sm'
                aria-label={t('Copy {{field}}', {
                  field: field.label,
                })}
              />
            )}
          </dd>
        </div>
      ))}
    </dl>
  )
}

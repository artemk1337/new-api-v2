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
import { ChevronDown } from 'lucide-react'
import { useId, useLayoutEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { RichContent } from '@/components/rich-content'
import { Button } from '@/components/ui/button'
import { splitAnnouncementContent } from '@/lib/announcement-content'
import { cn } from '@/lib/utils'

export function AnnouncementContent(props: { content: string }) {
  const { t } = useTranslation()
  const id = useId()
  const { title, body } = splitAnnouncementContent(props.content)
  const previewRef = useRef<HTMLDivElement>(null)
  const [expanded, setExpanded] = useState(false)
  const [overflows, setOverflows] = useState(false)

  useLayoutEffect(() => {
    const preview = previewRef.current
    if (!preview) return
    const measure = () =>
      setOverflows(preview.scrollHeight > preview.clientHeight + 1)
    const observer = new ResizeObserver(measure)
    observer.observe(preview)
    measure()
    return () => observer.disconnect()
  }, [props.content])

  return (
    <div>
      {title ? (
        <RichContent
          content={title}
          className='announcement-title mb-3 text-base leading-snug font-semibold tracking-tight [&_p]:m-0'
        />
      ) : null}
      <div id={id}>
        <div ref={previewRef} className={cn(!expanded && 'line-clamp-4')}>
          <RichContent
            breaks
            content={body}
            className='announcement-body text-muted-foreground text-sm leading-relaxed [&_h1]:text-base [&_h2]:text-base [&_h3]:text-base [&_p]:my-2 [&_strong]:font-semibold'
          />
        </div>
      </div>
      {overflows || expanded ? (
        <Button
          variant='ghost'
          size='sm'
          className='text-muted-foreground hover:text-foreground mt-2 -ml-2 gap-1.5'
          aria-expanded={expanded}
          aria-controls={id}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? t('Collapse update') : t('Read update')}
          <ChevronDown
            className={cn(
              'size-3.5 transition-transform motion-reduce:transition-none',
              expanded && 'rotate-180'
            )}
          />
        </Button>
      ) : null}
    </div>
  )
}

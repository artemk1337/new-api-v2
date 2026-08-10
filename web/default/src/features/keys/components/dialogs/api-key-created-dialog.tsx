/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { Check, Copy, Eye, EyeOff } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  CodeBlock,
  CodeBlockCopyButton,
} from '@/components/ai-elements/code-block'
import { Dialog } from '@/components/dialog'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useStatus } from '@/hooks/use-status'
import { copyToClipboard } from '@/lib/copy-to-clipboard'

import {
  buildApiKeySnippets,
  maskApiKey,
  resolveServerAddress,
  type ApiKeySnippetLanguage,
} from './api-key-created-dialog-utils'

const snippetLabels: Record<ApiKeySnippetLanguage, string> = {
  curl: 'cURL',
  python: 'Python',
  node: 'Node.js',
  go: 'Go',
}

const snippetLanguages: ApiKeySnippetLanguage[] = [
  'curl',
  'python',
  'node',
  'go',
]

const snippetHighlightLanguages = {
  curl: 'bash',
  python: 'python',
  node: 'javascript',
  go: 'go',
} as const

type ApiKeyCreatedDialogProps = {
  open: boolean
  tokenKey: string
  keyResolved: boolean
  keyError?: string
  retrying?: boolean
  onRetry?: () => void
  onOpenChange: (open: boolean) => void
}

export function ApiKeyCreatedDialog(props: ApiKeyCreatedDialogProps) {
  const { t } = useTranslation()
  const { status } = useStatus()
  const [keyVisible, setKeyVisible] = useState(false)
  const [language, setLanguage] = useState<ApiKeySnippetLanguage>('curl')

  useEffect(() => {
    setKeyVisible(false)
    setLanguage('curl')
  }, [props.tokenKey])

  const baseUrl = useMemo(() => {
    const fallback =
      typeof window === 'undefined'
        ? 'http://localhost'
        : window.location.origin
    return `${resolveServerAddress(status, fallback)}/v1`
  }, [status])
  const snippets = useMemo(
    () => buildApiKeySnippets(props.tokenKey, baseUrl),
    [baseUrl, props.tokenKey]
  )

  const copyValue = async (value: string) => {
    const copied = await copyToClipboard(value)
    if (copied) toast.success(t('Copied!'))
    else toast.error(t('Copy failed'))
  }

  return (
    <Dialog
      open={props.open}
      onOpenChange={(open) => {
        if (!open) {
          setKeyVisible(false)
          props.onOpenChange(false)
        }
      }}
      title={
        <span className='flex items-center gap-2'>
          <Check className='size-6 text-emerald-500' aria-hidden='true' />
          {t('API Key')}
        </span>
      }
      contentClassName='sm:max-w-3xl'
      contentHeight='auto'
    >
      <div className='space-y-6'>
        <div className='bg-muted/30 flex items-center gap-2 rounded-lg border p-3'>
          <code className='min-w-0 flex-1 truncate font-mono text-sm'>
            {keyVisible ? props.tokenKey : maskApiKey(props.tokenKey)}
          </code>
          <Button
            type='button'
            variant='ghost'
            size='icon-sm'
            aria-label={keyVisible ? t('Hide') : t('Show')}
            onClick={() => setKeyVisible((visible) => !visible)}
          >
            {keyVisible ? (
              <EyeOff aria-hidden='true' />
            ) : (
              <Eye aria-hidden='true' />
            )}
          </Button>
          {props.keyResolved && (
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Copy API key')}
              onClick={() => void copyValue(props.tokenKey)}
            >
              <Copy aria-hidden='true' />
            </Button>
          )}
        </div>

        {!props.keyResolved && (
          <Alert variant='destructive'>
            <AlertDescription>
              {props.keyError ||
                t(
                  'The API key was created, but its full value could not be loaded.'
                )}
              <Button
                type='button'
                variant='outline'
                size='sm'
                className='mt-3'
                disabled={props.retrying || !props.onRetry}
                onClick={props.onRetry}
              >
                {props.retrying ? t('Loading...') : t('Retry')}
              </Button>
            </AlertDescription>
          </Alert>
        )}

        <section className='space-y-3'>
          <h3 className='text-lg font-semibold'>
            {t('Base URL (API address)')}
          </h3>
          <div className='flex items-center gap-2 rounded-lg border px-3 py-2'>
            <code className='min-w-0 flex-1 truncate font-mono text-sm'>
              {baseUrl}
            </code>
            <Button
              type='button'
              variant='ghost'
              size='icon-sm'
              aria-label={t('Copy URL')}
              onClick={() => void copyValue(baseUrl)}
            >
              <Copy aria-hidden='true' />
            </Button>
          </div>
        </section>

        {props.keyResolved && (
          <section className='space-y-3'>
            <h3 className='text-lg font-semibold'>{t('Code samples')}</h3>
            <Tabs
              value={language}
              onValueChange={(value) =>
                setLanguage(value as ApiKeySnippetLanguage)
              }
            >
              <TabsList className='h-auto w-full flex-col items-stretch overflow-visible sm:h-8 sm:flex-row sm:items-center'>
                {snippetLanguages.map((item) => (
                  <TabsTrigger key={item} value={item}>
                    {snippetLabels[item]}
                  </TabsTrigger>
                ))}
              </TabsList>
              {snippetLanguages.map((item) => (
                <TabsContent key={item} value={item} className='mt-3'>
                  <CodeBlock
                    code={snippets[item]}
                    bodyClassName='overflow-visible [&_.cm-content]:min-w-0 [&_.cm-content]:break-words [&_.cm-content]:pl-4 [&_.cm-content]:whitespace-pre-wrap [&_.cm-scroller]:overflow-visible'
                    enableCollapse={false}
                    language={snippetHighlightLanguages[item]}
                  >
                    <CodeBlockCopyButton />
                  </CodeBlock>
                </TabsContent>
              ))}
            </Tabs>
          </section>
        )}
      </div>
    </Dialog>
  )
}

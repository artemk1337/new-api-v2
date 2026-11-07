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
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { ConnectionSettings } from './applications-guide'
import { DOCS_URLS } from './docs-sections'

export function QuickStartGuide() {
  const { t } = useTranslation()
  const steps: { text: string; to?: '/dashboard' | '/wallet' | '/keys' }[] = [
    { text: t('Sign in to your account.'), to: '/dashboard' },
    { text: t('Top up your balance in Wallet.'), to: '/wallet' },
    { text: t('Create and copy a key in API Keys.'), to: '/keys' },
    { text: t('Enter the settings below in your app and choose a model.') },
  ]
  const fields = [
    { label: t('Provider'), value: 'OpenAI API compatible' },
    { label: 'Base URL', value: DOCS_URLS.openAI },
    { label: 'API Key', value: t('Your key from the API Keys page') },
    { label: 'Model ID', value: t('Exact model ID from /v1/models') },
  ]

  return (
    <div>
      <ol className='mb-6 space-y-3'>
        {steps.map((step, index) => (
          <li key={step.text} className='flex gap-3 text-sm leading-6'>
            <span
              className='bg-muted text-muted-foreground mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full text-xs font-medium'
              aria-hidden='true'
            >
              {index + 1}
            </span>
            {step.to ? (
              <Link
                to={step.to}
                className='decoration-border hover:decoration-foreground underline underline-offset-4'
              >
                {step.text}
              </Link>
            ) : (
              <span>{step.text}</span>
            )}
          </li>
        ))}
      </ol>
      <ConnectionSettings fields={fields} />
      <div className='text-muted-foreground mt-4 space-y-2 text-xs leading-5'>
        <p>{t('For Claude Code, use the address without /v1.')}</p>
        <p>{t('Keep your API key private.')}</p>
      </div>
    </div>
  )
}

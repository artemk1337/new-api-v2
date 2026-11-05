import { expect, test } from 'bun:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'

import type { SystemStatus } from '@/features/auth/types'

import { useDashboardContentVisibility } from './use-status-data'

function ContentPanels() {
  const { apiInfo } = useDashboardContentVisibility()
  return apiInfo ? createElement('section', null, 'API Info') : null
}

test.each<{ name: string; status: SystemStatus; visible: boolean }>([
  {
    name: 'hides API info when the address list is empty',
    status: { api_info_enabled: true, api_info: [] },
    visible: false,
  },
  {
    name: 'hides API info when the address list is missing',
    status: { api_info_enabled: true },
    visible: false,
  },
  {
    name: 'shows API info when an address is configured',
    status: {
      api_info_enabled: true,
      api_info: [{ url: 'https://api.example.com' }],
    },
    visible: true,
  },
  {
    name: 'keeps configured API info hidden when disabled',
    status: {
      api_info_enabled: false,
      api_info: [{ url: 'https://api.example.com' }],
    },
    visible: false,
  },
])('$name', ({ status, visible }) => {
  const client = new QueryClient()
  client.setQueryData(['status'], status)

  const html = renderToStaticMarkup(
    createElement(QueryClientProvider, { client }, createElement(ContentPanels))
  )

  expect(html.includes('API Info')).toBe(visible)
  client.clear()
})

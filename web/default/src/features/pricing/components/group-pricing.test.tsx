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
import { describe, expect, test } from 'bun:test'

import { createInstance } from 'i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { I18nextProvider } from 'react-i18next'

import type { PricingGroupPrice, PricingModel } from '../types'
import { GroupPricingSection } from './group-pricing'
import { ModelPriceSummary } from './model-price-summary'
import { OfficialApiPrice } from './official-api-price'

const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })

function render(element: React.ReactNode): string {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>{element}</I18nextProvider>
  )
}

const model: PricingModel = {
  id: 1,
  model_name: 'test-model',
  quota_type: 0,
  model_ratio: 999,
  completion_ratio: 999,
  billing_expr: 'private-procurement-expression',
  enable_groups: ['1', '2'],
  group_prices: [
    {
      group_id: 1,
      group: '1',
      group_name: 'Coding',
      billing_mode: 'ratio',
      price_unit: 'million_tokens',
      input_usd: 1,
      output_usd: 20,
      cache_read_usd: 0,
      cache_write_usd: 1.25,
    },
    {
      group_id: 2,
      group: '2',
      group_name: 'Chat',
      billing_mode: 'tiered_expr',
      price_unit: 'million_tokens',
      input_usd: 2,
      output_usd: 5,
      cache_read_usd: 0.2,
      tiers: [
        {
          min_input_tokens: 0,
          max_input_tokens: 100000,
          input_usd: 2,
          output_usd: 5,
          cache_read_usd: 0.2,
        },
        {
          min_input_tokens: 100001,
          input_usd: 4,
          output_usd: 10,
          cache_write_1h_usd: 8,
        },
      ],
    },
  ],
}

describe('public group pricing', () => {
  test('summary renders one group pair without a global baseline or expression', () => {
    const html = render(<ModelPriceSummary model={model} tokenUnit='M' />)
    expect(html).toContain('Coding')
    expect(html).toContain('$1')
    expect(html).toContain('$20')
    expect(html).not.toContain('$5')
    expect(html).not.toContain('999')
    expect(html).not.toContain('private-procurement-expression')
    expect(html).not.toContain('line-through')
  })

  test('details render every group and its own cache and tier components', () => {
    const html = render(
      <GroupPricingSection model={model} tokenUnit='M' groupFilter='2' />
    )
    expect(html).toContain('Coding')
    expect(html).toContain('Chat')
    expect(html).toContain('$0')
    expect(html).toContain('$1.25')
    expect(html).toContain('$0.2')
    expect(html).toContain('$8')
    expect(html).toContain('More than 100,000 tokens')
    expect(html).toContain('Short context')
    expect(html).toContain('Long context')
    expect(html).not.toContain('private-procurement-expression')
    expect(html).not.toContain('Base Price')
  })

  test('request prices retain their own unit when token unit changes', () => {
    const fixed: PricingModel = {
      ...model,
      group_prices: [
        {
          group_id: 1,
          group: '1',
          group_name: 'Images',
          billing_mode: 'fixed',
          price_unit: 'request',
          price_usd: 0.05,
        },
      ],
    }
    const html = render(<ModelPriceSummary model={fixed} tokenUnit='K' />)
    expect(html).toContain('$0.05')
    expect(html).toContain('request')
    expect(html).not.toContain('1K')
  })

  test('request-dependent prices stay variable and never expose expressions', () => {
    const variable: PricingModel = {
      ...model,
      group_prices: [
        {
          group_id: 1,
          group: '1',
          group_name: 'Variable',
          billing_mode: 'tiered_expr',
          price_unit: 'million_tokens',
        },
      ],
    }
    const html = render(<GroupPricingSection model={variable} tokenUnit='M' />)
    expect(html).toContain('Price depends on request conditions')
    expect(html).not.toContain('$0')
    expect(html).not.toContain('private-procurement-expression')
  })

  test('each group shows its own tool fees per call without a token divisor', () => {
    const tools: PricingModel = {
      ...model,
      group_prices: model.group_prices?.map((price, index) => ({
        ...price,
        tools: {
          web_search: index === 0 ? 0.02 : 0.04,
          web_search_preview: index === 0 ? 0 : null,
          file_search: index === 0 ? 0.01 : 0.03,
          'private-procurement-field': 999,
        },
      })),
    }
    const html = render(<GroupPricingSection model={tools} tokenUnit='K' />)
    for (const amount of ['$0.02', '$0.04', '$0.01', '$0.03']) {
      expect(html).toContain(amount)
    }
    expect(html.match(/Per tool call/g)).toHaveLength(2)
    expect(html.match(/Web search preview/g)).toHaveLength(1)
    expect(html).toContain('Web search preview</dt><dd class="font-mono">$0')
    expect(html).not.toContain('private-procurement-field')
    expect(html).not.toContain('$0.00002')
  })

  test('tool fees apply the display funding conversion once', () => {
    const tools: PricingModel = {
      ...model,
      group_prices: model.group_prices?.slice(0, 1).map((price) => ({
        ...price,
        tools: { web_search: 0.02 },
      })),
    }
    const html = render(
      <GroupPricingSection
        model={tools}
        tokenUnit='K'
        showRechargePrice
        priceRate={3}
        usdExchangeRate={2}
      />
    )
    expect(html).toContain('Web search</dt><dd class="font-mono">$0.03')
  })

  test('variable tariffs show tool fees while unavailable tariffs hide them', () => {
    const tools: PricingModel = {
      ...model,
      group_prices: model.group_prices?.map(
        (price, index): PricingGroupPrice => {
          if (index === 1) {
            return { ...price, unavailable: true, tools: { web_search: 0.99 } }
          }
          return {
            ...price,
            billing_mode: 'tiered_expr',
            input_usd: undefined,
            output_usd: undefined,
            cache_read_usd: undefined,
            cache_write_usd: undefined,
            tools: { file_search: 0.03 },
          }
        }
      ),
    }
    const html = render(<GroupPricingSection model={tools} tokenUnit='M' />)
    expect(html).toContain('Price depends on request conditions')
    expect(html).toContain('File search</dt><dd class="font-mono">$0.03')
    expect(html).not.toContain('Web search')
    expect(html).not.toContain('$0.99')
  })

  test('does not display source tier labels when context bounds are unknown', () => {
    const generic: PricingModel = {
      ...model,
      group_prices: [
        {
          group_id: 1,
          group: '1',
          group_name: 'Generic',
          billing_mode: 'tiered_expr',
          price_unit: 'million_tokens',
          tiers: [
            { label: 'private-provider-step_1', input_usd: 1, output_usd: 2 },
          ],
        },
      ],
    }
    const html = render(<GroupPricingSection model={generic} tokenUnit='M' />)
    expect(html).toContain('Default')
    expect(html).not.toContain('private-provider-step_1')
    expect(html).not.toContain('Short context')
  })

  test('shows a sourced official reference separately from group prices', () => {
    const official = {
      input_usd: 3,
      output_usd: 15,
      source: 'https://example.com/official-pricing',
      updated_at: 1793743200,
      price_unit: 'million_tokens',
    }
    const reference: PricingModel = { ...model, official_price: official }
    const html = render(<OfficialApiPrice model={reference} tokenUnit='M' />)
    expect(html).toContain('Official API price')
    expect(html).toContain('$3')
    expect(html).toContain('$15')
    expect(html).toContain('https://example.com/official-pricing')
    expect(html).not.toContain('Benefit')
    expect(render(<OfficialApiPrice model={model} tokenUnit='M' />)).toBe('')
    expect(
      render(
        <OfficialApiPrice
          model={{
            ...reference,
            official_price: {
              ...official,
              source: 'javascript:alert(1)',
            },
          }}
          tokenUnit='M'
        />
      )
    ).toBe('')
  })
})

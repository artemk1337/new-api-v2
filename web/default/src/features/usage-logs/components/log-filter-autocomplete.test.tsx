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
import { afterEach, beforeEach, describe, expect, jest, test } from 'bun:test'

import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { AxiosResponse, InternalAxiosRequestConfig } from 'axios'
import { createInstance } from 'i18next'
import { useState } from 'react'
import { I18nextProvider } from 'react-i18next'

import { api } from '@/lib/api'

import type { LogFilterAutocompleteProps } from './log-filter-autocomplete'

const { act, cleanup, fireEvent, render, screen } =
  await import('@testing-library/react')
const { LogFilterAutocomplete } = await import('./log-filter-autocomplete')

const i18n = createInstance()
await i18n.init({ lng: 'en', resources: { en: { translation: {} } } })
const originalAdapter = api.defaults.adapter
const requests: {
  config: InternalAxiosRequestConfig
  respond: (items: unknown[], cursor?: string | null) => void
}[] = []
let client: QueryClient

beforeEach(() => {
  jest.useFakeTimers()
  requests.length = 0
  client = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  })
  api.defaults.adapter = (config) =>
    new Promise<AxiosResponse>((resolve) => {
      requests.push({
        config,
        respond: (items, cursor = null) =>
          resolve({
            config,
            data: { success: true, data: { items, next_cursor: cursor } },
            status: 200,
            statusText: 'OK',
            headers: {},
          }),
      })
    })
})

afterEach(() => {
  cleanup()
  client.clear()
  api.defaults.adapter = originalAdapter
  jest.useRealTimers()
})

function Harness(
  props: Omit<LogFilterAutocompleteProps, 'onValueChange'> & {
    onChange?: (value: string) => void
  }
) {
  const [value, setValue] = useState(props.value)
  return (
    <LogFilterAutocomplete
      {...props}
      value={value}
      onValueChange={(next) => {
        setValue(next)
        props.onChange?.(next)
      }}
    />
  )
}

const defaults = {
  actor: { id: 1, role: 100 },
  field: 'model_name' as const,
  filters: { start_timestamp: 100, end_timestamp: 200, model_name: 'old' },
  placeholder: 'Model Name',
  value: '',
  onApply: () => {},
}

function view(props: Partial<Parameters<typeof Harness>[0]> = {}) {
  return (
    <I18nextProvider i18n={i18n}>
      <QueryClientProvider client={client}>
        <Harness {...defaults} {...props} />
      </QueryClientProvider>
    </I18nextProvider>
  )
}

async function flush() {
  await act(async () => {})
  await act(async () => {
    jest.advanceTimersByTime(0)
  })
}

async function respond(
  index: number,
  items: unknown[],
  cursor: string | null = null
) {
  await act(async () => {
    requests[index].respond(items, cursor)
  })
  await flush()
}

describe('admin log filter autocomplete', () => {
  test('opens an empty input, loads all pages, and selects without applying the form', async () => {
    const changes: string[] = []
    let applications = 0
    render(
      view({
        onChange: (value) => changes.push(value),
        onApply: () => applications++,
      })
    )
    const input = screen.getByRole('combobox', { name: 'Model Name' })
    fireEvent.mouseDown(input, { button: 0 })
    await flush()
    expect(requests).toHaveLength(1)
    expect(requests[0].config.params).toEqual({
      start_timestamp: 100,
      end_timestamp: 200,
      field: 'model_name',
      q: '',
      cursor: undefined,
      limit: 50,
    })
    await respond(0, [{ value: 'model-a', label: 'Model A' }], '50')
    fireEvent.click(screen.getByRole('button', { name: 'Load more' }))
    await flush()
    expect(requests[1].config.params.cursor).toBe('50')
    await respond(1, [
      { value: 'model-a', label: 'Model A' },
      { value: 'model-b', label: 'Model B' },
    ])
    expect(screen.getAllByRole('option')).toHaveLength(2)
    fireEvent.keyDown(input, { key: 'Enter' })
    await flush()
    expect(changes).toContain('model-a')
    expect(applications).toBe(0)
    fireEvent.input(input, { target: { value: 'gpt-%' } })
    fireEvent.keyDown(input, { key: 'Escape' })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(changes).toContain('gpt-%')
    expect(applications).toBe(1)
  })

  test('debounces for 300ms, cancels old requests, and hides late options', async () => {
    render(view())
    const input = screen.getByRole('combobox')
    fireEvent.mouseDown(input, { button: 0 })
    await flush()
    fireEvent.input(input, { target: { value: 'alpha' } })
    await flush()
    expect(requests[0].config.signal?.aborted).toBe(true)
    await act(async () => {
      jest.advanceTimersByTime(200)
    })
    expect(requests.map((request) => request.config.params.q)).toEqual([''])
    await act(async () => {
      jest.advanceTimersByTime(100)
    })
    await flush()
    expect(requests[1].config.params.q).toBe('alpha')
    fireEvent.input(input, { target: { value: 'beta' } })
    await flush()
    expect(requests[1].config.signal?.aborted).toBe(true)
    await respond(1, [{ value: 'alpha-secret', label: 'Alpha secret' }])
    expect(screen.queryByText('Alpha secret')).toBeNull()
    await act(async () => {
      jest.advanceTimersByTime(300)
    })
    await flush()
    await respond(2, [{ value: 'beta', label: 'Beta result' }])
    expect(screen.getByRole('option', { name: /Beta result/ })).toBeTruthy()
  })

  test('does not share cached suggestions across actors and cancels on masking', async () => {
    const rendered = render(
      view({ field: 'token_name', placeholder: 'Token Name' })
    )
    fireEvent.mouseDown(screen.getByRole('combobox'), { button: 0 })
    await flush()
    await respond(0, [{ value: 'private-token', label: 'Private token' }])
    expect(screen.getByRole('option', { name: /Private token/ })).toBeTruthy()
    rendered.rerender(
      view({
        actor: { id: 2, role: 100 },
        field: 'token_name',
        placeholder: 'Token Name',
      })
    )
    expect(screen.queryByText('Private token')).toBeNull()
    fireEvent.mouseDown(screen.getByRole('combobox'), { button: 0 })
    await flush()
    expect(requests).toHaveLength(2)
    rendered.rerender(
      view({
        actor: { id: 2, role: 100 },
        field: 'token_name',
        placeholder: 'Token Name',
        masked: true,
      })
    )
    await flush()
    expect(requests[1].config.signal?.aborted).toBe(true)
    expect(screen.getByLabelText('Token Name').getAttribute('type')).toBe(
      'password'
    )
    expect(
      screen.getByLabelText('Token Name').getAttribute('autocomplete')
    ).toBe('new-password')
    await respond(1, [{ value: 'late-secret', label: 'Late secret' }])
    expect(screen.queryByText('Late secret')).toBeNull()
    expect(screen.queryByRole('combobox')).toBeNull()
  })

  test('keeps ordinary user inputs plain and hides supplier controls and requests', async () => {
    const rendered = render(view({ actor: { id: 3, role: 1 } }))
    fireEvent.click(screen.getByRole('textbox', { name: 'Model Name' }))
    fireEvent.input(screen.getByRole('textbox'), { target: { value: '%' } })
    await act(async () => {
      jest.advanceTimersByTime(300)
    })
    expect(screen.queryByRole('combobox')).toBeNull()
    expect(requests).toHaveLength(0)
    rendered.rerender(
      view({
        actor: { id: 3, role: 1 },
        field: 'proxy_supplier_id',
        placeholder: 'Proxy supplier',
      })
    )
    expect(screen.queryByLabelText('Proxy supplier')).toBeNull()
    expect(requests).toHaveLength(0)
  })

  test('offers static unknown provenance and keeps the selected supplier name outside a page', async () => {
    const changes: string[] = []
    const supplierProps = {
      field: 'proxy_supplier_id' as const,
      placeholder: 'Proxy supplier',
      filters: { proxy_supplier_id: 0 },
      onChange: (value: string) => changes.push(value),
    }
    const rendered = render(view(supplierProps))
    const input = screen.getByRole('combobox', { name: 'Proxy supplier' })
    fireEvent.mouseDown(input, { button: 0 })
    await flush()
    expect(requests[0].config.url).toBe('/api/log/supplier-choices')
    expect(requests[0].config.params.proxy_supplier_id).toBeUndefined()
    await respond(0, [{ id: 14, name: 'Supplier Fourteen' }])
    fireEvent.click(screen.getByRole('option', { name: /Supplier Fourteen/ }))
    expect(changes.at(-1)).toBe('14')
    expect(input.getAttribute('value')).toBe('Supplier Fourteen')
    rendered.rerender(
      view({ ...supplierProps, filters: { model_name: 'other-model' } })
    )
    fireEvent.mouseDown(input, { button: 0 })
    await flush()
    await respond(1, [{ id: 15, name: 'Another supplier' }])
    expect(
      screen.queryByRole('option', { name: /Supplier Fourteen/ })
    ).toBeNull()
    expect(input.getAttribute('value')).toBe('Supplier Fourteen')
    expect(
      screen.getByRole('option', { name: /No recorded supplier/ })
    ).toBeTruthy()
    fireEvent.click(
      screen.getByRole('option', { name: /No recorded supplier/ })
    )
    expect(changes.at(-1)).toBe('0')
    expect(input.getAttribute('value')).toBe('No recorded supplier')
    rendered.rerender(view({ ...supplierProps, masked: true }))
    expect(screen.getByLabelText('Proxy supplier').getAttribute('type')).toBe(
      'password'
    )
    expect(screen.getByLabelText('Proxy supplier').getAttribute('value')).toBe(
      '0'
    )
    expect(screen.queryByRole('combobox')).toBeNull()
  })

  test('supplier keyboard search selects its server result instead of the static All option', async () => {
    const changes: string[] = []
    let applications = 0
    render(
      view({
        field: 'proxy_supplier_id',
        placeholder: 'Proxy supplier',
        onChange: (value) => changes.push(value),
        onApply: () => applications++,
      })
    )
    const input = screen.getByRole('combobox')
    fireEvent.mouseDown(input, { button: 0 })
    await flush()
    await respond(0, [{ id: 14, name: 'Old supplier' }])
    fireEvent.keyDown(input, { key: 'ArrowDown' })
    fireEvent.input(input, { target: { value: 'BBL' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(applications).toBe(0)
    expect(changes).not.toContain('14')
    await act(async () => {
      jest.advanceTimersByTime(300)
    })
    await flush()
    await respond(1, [{ id: 1, name: 'BBLABU' }])
    expect(screen.queryByRole('option', { name: /All suppliers/ })).toBeNull()
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(changes.at(-1)).toBe('1')
    expect(input.getAttribute('value')).toBe('BBLABU')
    expect(applications).toBe(0)
  })
})

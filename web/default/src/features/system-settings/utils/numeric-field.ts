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
import type { ChangeEvent } from 'react'
import type {
  ControllerRenderProps,
  FieldPath,
  FieldValues,
} from 'react-hook-form'

/**
 * Props produced by {@link safeNumberFieldProps} for a native
 * `<input type="number">`. They are intentionally narrow so consumers can
 * spread them onto our shared `Input` component without leaking the
 * react-hook-form internals (e.g. `disabled`) that need overriding per call.
 */
export type SafeNumberFieldProps = {
  value: number | string
  onChange: (event: ChangeEvent<HTMLInputElement>) => void
  onBlur: () => void
  name: string
  ref: (instance: HTMLInputElement | null) => void
}

export function parseDecimalValue(value: string): number | null {
  const normalized = value.trim().replace(',', '.')
  if (!normalized || normalized.endsWith('.')) {
    return null
  }

  const next = Number(normalized)
  return Number.isFinite(next) ? next : null
}

/** Normalize the editable decimal text used by settings fields. */
export function normalizeDecimalInput(value: string): string | null {
  const normalized = value.trim().replace(',', '.')
  return !normalized || /^\d*(\.\d{0,2})?$/.test(normalized)
    ? normalized
    : null
}

export function safeDecimalFieldProps<
  TFieldValues extends FieldValues,
  TName extends FieldPath<TFieldValues>,
>(field: ControllerRenderProps<TFieldValues, TName>): SafeNumberFieldProps {
  const raw = field.value as unknown
  const display: number | string =
    (typeof raw === 'number' && Number.isFinite(raw)) ||
    typeof raw === 'string'
      ? raw
      : ''

  return {
    value: display,
    onChange: (event) => {
      const normalized = normalizeDecimalInput(event.target.value)
      // Keep the raw decimal text in form state while the user is typing.
      // Converting to a number after the first digit would turn `0.` back
      // into `0`, making it impossible to enter values such as `0.01`.
      if (normalized !== null) {
        ;(field.onChange as (value: string) => void)(normalized)
      }
    },
    onBlur: field.onBlur,
    name: field.name,
    ref: field.ref,
  }
}

/**
 * Adapter for binding a react-hook-form numeric field to a native
 * `<input type="number">` without ever putting `NaN` into form state.
 *
 * Why this exists:
 * - `<input type="number">` reports `valueAsNumber === NaN` whenever the field
 *   is empty or holds an in-progress non-numeric token (e.g. just a minus
 *   sign or a trailing dot). Forwarding `NaN` to `field.onChange` makes Zod
 *   numeric validators (`z.number().min(...)`, `z.coerce.number()`, etc.)
 *   fail at submit time, so `form.handleSubmit` silently refuses to call
 *   `onSubmit` — the save button appears frozen with no toast and no error.
 * - The legacy Semi `InputNumber` avoids this by snapping the input back to
 *   the previous valid number. We replicate that behaviour by ignoring `NaN`
 *   updates: React's controlled-input reconciliation will restore the last
 *   valid value to the DOM on the next render.
 *
 * Display:
 * - When the underlying state is not a finite number, the prop returns `''`
 *   so the input visibly renders empty instead of literal "NaN".
 *
 * Usage:
 * ```tsx
 * <FormField
 *   control={form.control}
 *   name='performance_setting.monitor_cpu_threshold'
 *   render={({ field }) => (
 *     <Input type='number' min={0} {...safeNumberFieldProps(field)} />
 *   )}
 * />
 * ```
 */
export function safeNumberFieldProps<
  TFieldValues extends FieldValues,
  TName extends FieldPath<TFieldValues>,
>(field: ControllerRenderProps<TFieldValues, TName>): SafeNumberFieldProps {
  const raw = field.value as unknown
  const display: number | '' =
    typeof raw === 'number' && Number.isFinite(raw) ? raw : ''

  return {
    value: display,
    onChange: (event) => {
      const next = event.target.valueAsNumber
      if (Number.isFinite(next)) {
        ;(field.onChange as (value: number) => void)(next)
      }
    },
    onBlur: field.onBlur,
    name: field.name,
    ref: field.ref,
  }
}

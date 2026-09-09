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
import * as z from 'zod'

export const createBotProtectionSchema = (t: (key: string) => string) =>
  z
    .object({
      RegistrationRateLimitEnabled: z.boolean(),
      RegistrationRateLimitAttempts: z.number().int().min(1).max(1000),
      RegistrationRateLimitSuccesses: z.number().int().min(1).max(1000),
      RegistrationRateLimitWindowMinutes: z.number().int().min(1).max(1440),
      TurnstileCheckEnabled: z.boolean(),
      TurnstileSiteKey: z.string().optional(),
      TurnstileSecretKey: z.string().optional(),
    })
    .refine(
      (values) =>
        values.RegistrationRateLimitSuccesses <=
        values.RegistrationRateLimitAttempts,
      {
        message: t('Successful registration limit cannot exceed attempt limit'),
        path: ['RegistrationRateLimitSuccesses'],
      }
    )

export type BotProtectionFormValues = z.infer<
  ReturnType<typeof createBotProtectionSchema>
>

export type BotProtectionOptionUpdate = {
  key: keyof BotProtectionFormValues
  value: string | boolean | number
}

const botProtectionOptionOrder = [
  'RegistrationRateLimitAttempts',
  'RegistrationRateLimitSuccesses',
  'RegistrationRateLimitWindowMinutes',
  'RegistrationRateLimitEnabled',
  'TurnstileSiteKey',
  'TurnstileSecretKey',
  'TurnstileCheckEnabled',
] as const satisfies ReadonlyArray<keyof BotProtectionFormValues>

export function getBotProtectionOptionUpdates(
  values: BotProtectionFormValues,
  defaultValues: BotProtectionFormValues
): BotProtectionOptionUpdate[] {
  return botProtectionOptionOrder.flatMap((key) => {
    const value = values[key]
    if (value === defaultValues[key]) return []
    if (key === 'TurnstileSecretKey' && !value) return []

    return [{ key, value: value ?? '' }]
  })
}

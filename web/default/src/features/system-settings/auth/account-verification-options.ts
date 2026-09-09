/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/
export const accountVerificationProviders = ['email', 'telegram', 'github'] as const

export type AccountVerificationProvider =
  (typeof accountVerificationProviders)[number]

export function parseAccountVerificationProviders(
  value: string
): AccountVerificationProvider[] {
  const selected = value
    .split(',')
    .map((item) => item.trim().toLowerCase())
    .filter((item): item is AccountVerificationProvider =>
      accountVerificationProviders.includes(item as AccountVerificationProvider)
    )
  return selected.length > 0
    ? [...new Set(selected)]
    : ['email']
}

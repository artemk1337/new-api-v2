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

export function isValidAmountCashbackConfig(value: unknown): boolean {
  if (!Array.isArray(value)) {
    return false
  }

  const amounts = new Set<number>()
  return value.every((item) => {
    if (!item || typeof item !== 'object' || Array.isArray(item)) {
      return false
    }

    const record = item as Record<string, unknown>
    const amount = record.min_amount
    const percent = record.cashback_percent
    const referralPercent = record.referral_cashback_percent
    if (
      typeof amount !== 'number' ||
      !Number.isFinite(amount) ||
      amount < 0 ||
      typeof percent !== 'number' ||
      !Number.isFinite(percent) ||
      percent < 0 ||
      percent > 100 ||
      (referralPercent !== undefined &&
        (typeof referralPercent !== 'number' ||
          !Number.isFinite(referralPercent) ||
          referralPercent < percent ||
          referralPercent > 100)) ||
      amounts.has(amount)
    ) {
      return false
    }

    amounts.add(amount)
    return true
  })
}

export function normalizeAmountCashbackConfig(value: string): string {
  return value.trim() || '[]'
}

export function formatAmountCashbackThreshold(
  amount: number,
  tokenAmounts: boolean,
  locale?: Intl.LocalesArgument
): string {
  const formatted = new Intl.NumberFormat(locale, {
    maximumFractionDigits: 20,
  }).format(amount)
  return tokenAmounts ? formatted : `$${formatted}`
}

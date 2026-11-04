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
import { useMemo } from 'react'

import { getOptionValue, useSystemOptions } from '../hooks/use-system-options'
import { buildModelSnapshots } from './model-pricing-snapshots'

export function usePricingSyncSnapshots() {
  const { data: optionsData } = useSystemOptions()
  const snapshots = useMemo(() => {
    const options = getOptionValue(optionsData?.data, {
      ModelPrice: '{}',
      ModelRatio: '{}',
      CompletionRatio: '{}',
      CacheRatio: '{}',
      CreateCacheRatio: '{}',
      ImageRatio: '{}',
      AudioRatio: '{}',
      AudioCompletionRatio: '{}',
      'billing_setting.billing_mode': '{}',
      'billing_setting.billing_expr': '{}',
    })
    return new Map(
      buildModelSnapshots({
        modelPrice: options.ModelPrice,
        modelRatio: options.ModelRatio,
        completionRatio: options.CompletionRatio,
        cacheRatio: options.CacheRatio,
        createCacheRatio: options.CreateCacheRatio,
        imageRatio: options.ImageRatio,
        audioRatio: options.AudioRatio,
        audioCompletionRatio: options.AudioCompletionRatio,
        billingMode: options['billing_setting.billing_mode'],
        billingExpr: options['billing_setting.billing_expr'],
      }).map((snapshot) => [snapshot.name, snapshot])
    )
  }, [optionsData])
  const quotaPerUnit = Number(
    optionsData?.data.find((option) => option.key === 'QuotaPerUnit')?.value ??
      500000
  )

  return { snapshots, quotaPerUnit }
}

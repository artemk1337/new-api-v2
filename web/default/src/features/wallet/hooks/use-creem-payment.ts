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
import { useState, useCallback } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { requestCreemPayment, isApiSuccess } from '../api'
import {
  getPaymentErrorMessage,
  isSafeHttpRedirectUrl,
  redirectToPaymentPage,
} from '../lib/payment'

export function getSafeCreemCheckoutUrl(
  data: { checkout_url?: string } | undefined
): string | null {
  const checkoutUrl = data?.checkout_url?.trim()
  return checkoutUrl && isSafeHttpRedirectUrl(checkoutUrl) ? checkoutUrl : null
}

export function isIncompleteSuccessfulCreemPaymentResponse(response: {
  success?: boolean
  message?: string
  data?: { checkout_url?: string }
}): boolean {
  return isApiSuccess(response) && !response.data?.checkout_url?.trim()
}

/**
 * Hook for handling Creem payment processing
 */
export function useCreemPayment() {
  const [processing, setProcessing] = useState(false)

  const processCreemPayment = useCallback(async (productId: string) => {
    setProcessing(true)
    try {
      const response = await requestCreemPayment({
        product_id: productId,
        payment_method: 'creem',
      })

      const checkoutUrl = getSafeCreemCheckoutUrl(response.data)
      if (isApiSuccess(response) && checkoutUrl) {
        redirectToPaymentPage(checkoutUrl)
        toast.success(i18next.t('Redirecting to Creem checkout...'))
        return true
      }

      if (isApiSuccess(response) && response.data?.checkout_url) {
        toast.error(i18next.t('Invalid payment redirect URL'))
        return false
      }

      if (isIncompleteSuccessfulCreemPaymentResponse(response)) {
        toast.error(i18next.t('Payment request failed'))
        return false
      }

      toast.error(
        getPaymentErrorMessage(response, i18next.t('Payment request failed'))
      )
      return false
    } catch {
      toast.error(i18next.t('Payment request failed'))
      return false
    } finally {
      setProcessing(false)
    }
  }, [])

  return { processing, processCreemPayment }
}

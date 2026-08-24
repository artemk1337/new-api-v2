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
import {
  calculateAmount,
  calculateStripeAmount,
  calculateWaffoPancakeAmount,
  calculateYooKassaAmount,
  calculateNOWPaymentsAmount,
  requestPayment,
  requestStripePayment,
  requestYooKassaPayment,
  requestNOWPaymentsPayment,
  isApiSuccess,
} from '../api'
import {
  isStripePayment,
  isWaffoPancakePayment,
  isYooKassaPayment,
  isNOWPaymentsPayment,
  getPaymentErrorMessage,
  isSafeHttpRedirectUrl,
  redirectToPaymentPage,
  submitPaymentForm,
} from '../lib'

function getStringField(data: unknown, field: string): string | null {
  if (!data || typeof data !== 'object') {
    return null
  }
  const value = (data as Record<string, unknown>)[field]
  return typeof value === 'string' && value.trim() ? value : null
}

export function isIncompleteSuccessfulPaymentResponse(
  response: { success?: boolean; message?: string; data?: unknown; url?: unknown },
  paymentType: string
): boolean {
  if (!isApiSuccess(response)) {
    return false
  }

  if (isStripePayment(paymentType)) {
    return getStringField(response.data, 'pay_link') === null
  }
  if (isYooKassaPayment(paymentType)) {
    return getStringField(response.data, 'confirmation_url') === null
  }
  if (isNOWPaymentsPayment(paymentType)) {
    return getStringField(response.data, 'payment_url') === null
  }
  return !response.data || typeof response.url !== 'string' || !response.url.trim()
}

// ============================================================================
// Payment Hook
// ============================================================================

export function usePayment() {
  const [amount, setAmount] = useState<number>(0)
  const [calculating, setCalculating] = useState(false)
  const [processing, setProcessing] = useState(false)

  // Calculate payment amount
  const calculatePaymentAmount = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setCalculating(true)

        const isStripe = isStripePayment(paymentType)
        const isPancake = isWaffoPancakePayment(paymentType)
        const isYooKassa = isYooKassaPayment(paymentType)
        const isNOWPayments = isNOWPaymentsPayment(paymentType)
        let response
        if (isStripe) {
          response = await calculateStripeAmount({ amount: topupAmount })
        } else if (isPancake) {
          response = await calculateWaffoPancakeAmount({ amount: topupAmount })
        } else if (isYooKassa) {
          response = await calculateYooKassaAmount({ amount: topupAmount })
        } else if (isNOWPayments) {
          response = await calculateNOWPaymentsAmount({ amount: topupAmount })
        } else {
          response = await calculateAmount({
            amount: topupAmount,
            payment_method: paymentType,
          })
        }

        if (isApiSuccess(response) && response.data) {
          const calculatedAmount = Number.parseFloat(response.data)
          setAmount(calculatedAmount)
          return calculatedAmount
        }

        // Don't show error for calculation, just set to 0
        setAmount(0)
        return 0
      } catch {
        setAmount(0)
        return 0
      } finally {
        setCalculating(false)
      }
    },
    []
  )

  // Process payment
  const processPayment = useCallback(
    async (topupAmount: number, paymentType: string) => {
      try {
        setProcessing(true)

        const isStripe = isStripePayment(paymentType)
        const isYooKassa = isYooKassaPayment(paymentType)
        const isNOWPayments = isNOWPaymentsPayment(paymentType)
        let response
        if (isStripe) {
          response = await requestStripePayment({
            amount: topupAmount,
            payment_method: 'stripe',
          })
        } else if (isYooKassa) {
          response = await requestYooKassaPayment({
            amount: topupAmount,
            payment_method: 'yookassa_sbp',
          })
        } else if (isNOWPayments) {
          response = await requestNOWPaymentsPayment({
            amount: topupAmount,
            payment_method: 'nowpayments',
          })
        } else {
          response = await requestPayment({
            amount: topupAmount,
            payment_method: paymentType,
          })
        }

        if (!isApiSuccess(response)) {
          toast.error(
            getPaymentErrorMessage(response, i18next.t('Payment request failed'))
          )
          return false
        }

        // Handle Stripe payment
        const payLink = getStringField(response.data, 'pay_link')
        if (isStripe && payLink) {
          if (!redirectToPaymentPage(payLink)) {
            toast.error(i18next.t('Invalid payment redirect URL'))
            return false
          }
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        const confirmationUrl = getStringField(
          response.data,
          'confirmation_url'
        )
        if (isYooKassa && confirmationUrl) {
          if (!isSafeHttpRedirectUrl(confirmationUrl)) {
            toast.error(i18next.t('Invalid payment redirect URL'))
            return false
          }
          window.location.href = confirmationUrl
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        const paymentUrl = getStringField(response.data, 'payment_url')
        if (isNOWPayments && paymentUrl) {
          if (!isSafeHttpRedirectUrl(paymentUrl)) {
            toast.error(i18next.t('Invalid payment redirect URL'))
            return false
          }
          window.location.href = paymentUrl
          toast.success(i18next.t('Redirecting to payment page...'))
          return true
        }

        // Handle non-Stripe payment
        if (!isStripe && !isYooKassa && !isNOWPayments && response.data) {
          const url = (response as unknown as { url?: string }).url
          if (url) {
            if (!isSafeHttpRedirectUrl(url)) {
              toast.error(i18next.t('Invalid payment redirect URL'))
              return false
            }
            submitPaymentForm(url, response.data)
            toast.success(i18next.t('Redirecting to payment page...'))
            return true
          }
        }

        if (isIncompleteSuccessfulPaymentResponse(response, paymentType)) {
          toast.error(i18next.t('Payment request failed'))
        }
        return false
      } catch {
        toast.error(i18next.t('Payment request failed'))
        return false
      } finally {
        setProcessing(false)
      }
    },
    []
  )

  return {
    amount,
    calculating,
    processing,
    calculatePaymentAmount,
    processPayment,
    setAmount,
  }
}

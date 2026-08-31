/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Loader2 } from 'lucide-react'
import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

import { requestDirectCryptoPayment } from '../api'
import { useTopupInfo } from '../hooks'
import { getPaymentErrorMessage, isPaymentMethodAmountEligible } from '../lib'
import {
  parseDirectCryptoInvoiceUrl,
  prepareDirectCryptoPayment,
} from '../lib/direct-crypto-checkout'
import type { DirectUSDTNetwork, PaymentMethod } from '../types'
import { DirectCryptoPaymentPage } from './direct-crypto-payment-page'

const networkLabels: Record<DirectUSDTNetwork, string> = {
  TRON: 'TRON (TRC20)',
  TON: 'TON',
  SOLANA: 'Solana',
}

function getDirectCryptoMethod(methods: PaymentMethod[] | undefined) {
  return methods?.find((method) => method.type === 'crypto_direct')
}

export function DirectCryptoNetworkPage({ amount }: { amount: number }) {
  const { t } = useTranslation()
  const { topupInfo, loading } = useTopupInfo()
  const [processingNetwork, setProcessingNetwork] =
    useState<DirectUSDTNetwork | null>(null)
  const [invoice, setInvoice] = useState<{
    network: DirectUSDTNetwork
    tradeNo: string
  } | null>(null)
  const creatingRef = useRef(false)
  const directMethod = getDirectCryptoMethod(topupInfo?.pay_methods)
  const availableNetworks = topupInfo?.crypto_networks ?? []
  const canCreateInvoice = Boolean(
    directMethod && isPaymentMethodAmountEligible(amount, directMethod)
  )

  const startPayment = async (network: DirectUSDTNetwork) => {
    if (creatingRef.current) return
    const payment = prepareDirectCryptoPayment(
      amount,
      availableNetworks,
      network,
      canCreateInvoice
    )
    if (!payment) {
      toast.error(t('Payment method is unavailable'))
      return
    }

    creatingRef.current = true
    setProcessingNetwork(network)
    try {
      const response = await requestDirectCryptoPayment(
        payment.network,
        payment.request
      )
      const paymentUrl = response.data?.payment_url ?? ''
      const invoice = parseDirectCryptoInvoiceUrl(paymentUrl)
      if (!response.success || !invoice) {
        toast.error(
          getPaymentErrorMessage(response, t('Payment request failed'))
        )
        return
      }
      window.history.replaceState({}, '', paymentUrl)
      setInvoice(invoice)
    } catch {
      toast.error(t('Payment request failed'))
    } finally {
      creatingRef.current = false
      setProcessingNetwork(null)
    }
  }

  if (invoice) {
    return (
      <DirectCryptoPaymentPage
        network={invoice.network}
        tradeNo={invoice.tradeNo}
      />
    )
  }

  let networkContent: React.ReactNode
  if (loading) {
    networkContent = <Loader2 className='mx-auto animate-spin' />
  } else if (availableNetworks.length === 0 || !canCreateInvoice) {
    networkContent = (
      <p className='text-destructive text-sm'>
        {t('No crypto networks are currently available.')}
      </p>
    )
  } else {
    networkContent = (
      <div className='grid gap-2'>
        {availableNetworks.map((network) => (
          <Button
            key={network}
            variant='outline'
            className='justify-start'
            disabled={processingNetwork !== null}
            onClick={() => void startPayment(network)}
          >
            {processingNetwork === network && (
              <Loader2 className='animate-spin' />
            )}
            {networkLabels[network]}
          </Button>
        ))}
      </div>
    )
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Crypto payment')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card className='mx-auto max-w-lg'>
          <CardHeader>
            <CardTitle>{t('Choose a network to continue')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-4'>
            <p className='text-muted-foreground text-sm'>
              {t('Amount')}: {amount}
            </p>
            {networkContent}
            <a
              href='/wallet'
              className='border-input hover:bg-accent hover:text-accent-foreground inline-flex h-9 w-full items-center justify-center rounded-md border text-sm'
            >
              {t('Back to wallet')}
            </a>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

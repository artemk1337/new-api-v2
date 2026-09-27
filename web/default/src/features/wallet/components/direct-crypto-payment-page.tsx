/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { Check, Copy, Loader2 } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  cancelDirectCryptoPayment,
  getDirectCryptoPaymentStatus,
} from '@/features/wallet/api'
import {
  formatDirectCryptoAmount,
  getDirectCryptoPaymentAddress,
} from '@/features/wallet/lib/direct-crypto-checkout'
import {
  getUSDTTrc20DisplayStatus,
  isUSDTTrc20TerminalStatus,
} from '@/features/wallet/lib/payment'
import type {
  DirectUSDTNetwork,
  DirectUSDTPaymentStatus,
} from '@/features/wallet/types'

function toMillis(value: number | string): number {
  if (typeof value === 'number') {
    return value < 10_000_000_000 ? value * 1000 : value
  }
  const numeric = Number(value)
  if (Number.isFinite(numeric)) return toMillis(numeric)
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}

const networkLabels: Record<DirectUSDTNetwork, string> = {
  TRON: 'TRON (TRC20)',
  TON: 'TON',
  SOLANA: 'Solana',
}

export function DirectCryptoPaymentPage({
  network,
  tradeNo,
}: {
  network: DirectUSDTNetwork
  tradeNo: string
}) {
  const { t } = useTranslation()
  const [payment, setPayment] = useState<DirectUSDTPaymentStatus | null>(null)
  const [error, setError] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const [copied, setCopied] = useState(false)
  const [cancelling, setCancelling] = useState(false)
  const [cancelError, setCancelError] = useState(false)
  const terminalRef = useRef(false)
  const requestSequenceRef = useRef(0)

  useEffect(() => {
    let cancelled = false
    terminalRef.current = false
    requestSequenceRef.current = 0
    setPayment(null)
    setError(false)
    setCancelError(false)
    const poll = async () => {
      const requestSequence = ++requestSequenceRef.current
      try {
        const response = await getDirectCryptoPaymentStatus(network, tradeNo)
        if (cancelled || requestSequence !== requestSequenceRef.current) return
        if (response.success && response.data) {
          setError(false)
          setPayment(response.data)
          if (isUSDTTrc20TerminalStatus(response.data.status)) {
            terminalRef.current = true
          }
        } else setError(true)
      } catch {
        if (!cancelled && requestSequence === requestSequenceRef.current) {
          setError(true)
        }
      }
    }
    void poll()
    const pollTimer = setInterval(() => {
      if (!terminalRef.current) void poll()
    }, 5000)
    const clockTimer = setInterval(() => setNow(Date.now()), 1000)
    return () => {
      cancelled = true
      clearInterval(pollTimer)
      clearInterval(clockTimer)
    }
  }, [network, tradeNo])

  const expiresAt = payment ? toMillis(payment.expires_at) : 0
  const secondsLeft = expiresAt
    ? Math.max(0, Math.ceil((expiresAt - now) / 1000))
    : 0
  const expiredByTime = Boolean(
    expiresAt && secondsLeft === 0 && payment?.status === 'pending'
  )
  const status = getUSDTTrc20DisplayStatus(payment?.status, expiredByTime)
  // Send to the immutable token destination (SPL token account on Solana or
  // jetton destination on TON), never an owner address. Fail closed if it is
  // missing instead of falling back to an address that could lose funds.
  const address = payment ? getDirectCryptoPaymentAddress(payment, network) : ''
  const qrValue = useMemo(() => address, [address])
  const amount = payment ? formatDirectCryptoAmount(payment.amount) : ''
  const tokenContract = payment?.token_contract || payment?.contract || ''

  const copyAddress = async () => {
    if (!address) return
    await navigator.clipboard.writeText(address)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const cancelPayment = async () => {
    if (!payment || cancelling || isUSDTTrc20TerminalStatus(payment.status)) {
      return
    }
    setCancelling(true)
    setCancelError(false)
    try {
      const response = await cancelDirectCryptoPayment(network, tradeNo)
      if (response.success || response.message === 'success') {
        setPayment({ ...payment, status: 'cancelled' })
      } else {
        setCancelError(true)
      }
    } catch {
      setCancelError(true)
    } finally {
      setCancelling(false)
    }
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>{t('Crypto payment')}</SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card className='mx-auto max-w-lg'>
          <CardHeader>
            <CardTitle>
              {status === 'pending'
                ? t('Send the exact amount')
                : t('Crypto payment')}
            </CardTitle>
          </CardHeader>
          <CardContent className='space-y-5'>
            {error && (
              <p className='text-destructive text-sm'>
                {t('Unable to load payment status')}
              </p>
            )}
            {payment ? (
              <>
                {status === 'pending' && (
                  <>
                    <div className='flex justify-center rounded-lg bg-white p-4'>
                      <QRCodeSVG value={qrValue} size={220} />
                    </div>
                    <div className='text-center'>
                      <p className='text-muted-foreground text-sm'>
                        {t('Amount')}
                      </p>
                      <p className='text-3xl font-semibold'>{amount} USDT</p>
                      <p className='text-muted-foreground text-sm'>
                        {networkLabels[network]} · USDT
                      </p>
                    </div>
                    <p
                      role='alert'
                      className='border-destructive/40 bg-destructive/10 text-destructive rounded-md border p-3 text-sm'
                    >
                      {t(
                        'Send exactly the amount shown. Transfers with a different amount are not credited automatically. If you have a problem with a transfer, contact technical support.'
                      )}
                    </p>
                    <div className='space-y-2'>
                      <p className='text-muted-foreground text-sm'>
                        {t('Payment address')} · USDT
                      </p>
                      <div className='flex items-center gap-2'>
                        <code className='bg-muted min-w-0 flex-1 rounded p-2 text-xs break-all'>
                          {address}
                        </code>
                        <Button
                          size='icon'
                          variant='outline'
                          onClick={copyAddress}
                          aria-label={t('Copy address')}
                        >
                          {copied ? <Check /> : <Copy />}
                        </Button>
                      </div>
                      <p className='text-muted-foreground text-xs'>
                        {t('Only send USDT on the {{network}} network.', {
                          network: networkLabels[network],
                        })}
                      </p>
                      {tokenContract && (
                        <p className='text-muted-foreground text-xs break-all'>
                          {t('Token contract')}: {tokenContract}
                        </p>
                      )}
                    </div>
                  </>
                )}
                <div className='text-center text-sm'>
                  {status === 'pending' && (
                    <span>
                      {t('Expires in {{minutes}}:{{seconds}}', {
                        minutes: Math.floor(secondsLeft / 60),
                        seconds: String(secondsLeft % 60).padStart(2, '0'),
                      })}
                    </span>
                  )}
                  {status === 'success' && (
                    <span className='text-green-600'>
                      {t('Payment confirmed')}
                    </span>
                  )}
                  {status === 'expired' && (
                    <span className='text-destructive'>
                      {t('Payment expired')}
                    </span>
                  )}
                  {status === 'failed' && (
                    <span className='text-destructive'>
                      {t('Payment failed')}
                    </span>
                  )}
                  {status === 'cancelled' && (
                    <span className='text-muted-foreground'>
                      {t('Payment cancelled')}
                    </span>
                  )}
                  {!status && <Loader2 className='mx-auto animate-spin' />}
                </div>
                {status === 'pending' && (
                  <div className='space-y-2'>
                    {cancelError && (
                      <p role='alert' className='text-destructive text-sm'>
                        {t('Unable to cancel payment')}
                      </p>
                    )}
                    <Button
                      variant='outline'
                      className='w-full'
                      onClick={() => void cancelPayment()}
                      disabled={cancelling}
                    >
                      {cancelling ? t('Processing...') : t('Cancel payment')}
                    </Button>
                  </div>
                )}
              </>
            ) : (
              <Loader2 className='mx-auto animate-spin' />
            )}
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

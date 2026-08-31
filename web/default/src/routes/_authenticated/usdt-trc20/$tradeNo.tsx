import { createFileRoute, Link } from '@tanstack/react-router'
import { Check, Copy, Loader2 } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { Button, buttonVariants } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { getUSDTTrc20PaymentStatus } from '@/features/wallet/api'
import {
  getUSDTTrc20DisplayStatus,
  isUSDTTrc20TerminalStatus,
} from '@/features/wallet/lib/payment'
import type { USDTTrc20PaymentStatus } from '@/features/wallet/types'

export const Route = createFileRoute('/_authenticated/usdt-trc20/$tradeNo')({
  component: USDTTrc20PaymentPage,
})

function toMillis(value: number | string): number {
  if (typeof value === 'number')
    return value < 10_000_000_000 ? value * 1000 : value
  const numeric = Number(value)
  if (Number.isFinite(numeric)) return toMillis(numeric)
  const parsed = Date.parse(value)
  return Number.isFinite(parsed) ? parsed : 0
}

function USDTTrc20PaymentPage() {
  const { t } = useTranslation()
  const { tradeNo } = Route.useParams()
  const [payment, setPayment] = useState<USDTTrc20PaymentStatus | null>(null)
  const [error, setError] = useState(false)
  const [now, setNow] = useState(() => Date.now())
  const [copied, setCopied] = useState(false)
  const terminalRef = useRef(false)

  useEffect(() => {
    let cancelled = false
    terminalRef.current = false
    const poll = async () => {
      try {
        const response = await getUSDTTrc20PaymentStatus(tradeNo)
        if (cancelled) return
        if (response.success && response.data) {
          // Keep the server's amount and status contract untouched. The
          // amount is a decimal string and must not be coerced through a
          // floating-point number before it is shown to the payer.
          setPayment(response.data)
          if (isUSDTTrc20TerminalStatus(response.data.status)) {
            terminalRef.current = true
            clearInterval(timer)
          }
        } else setError(true)
      } catch {
        if (!cancelled) setError(true)
      }
    }
    const timer = setInterval(() => {
      setNow(Date.now())
      if (!terminalRef.current) poll()
    }, 5000)
    poll()
    return () => {
      cancelled = true
      clearInterval(timer)
    }
  }, [tradeNo])

  const expiresAt = payment ? toMillis(payment.expires_at) : 0
  const secondsLeft = expiresAt
    ? Math.max(0, Math.ceil((expiresAt - now) / 1000))
    : 0
  const expiredByTime = Boolean(
    expiresAt && secondsLeft === 0 && payment?.status === 'pending'
  )
  const status = getUSDTTrc20DisplayStatus(payment?.status, expiredByTime)
  const minutes = Math.floor(secondsLeft / 60)
  const seconds = String(secondsLeft % 60).padStart(2, '0')
  const qrValue = useMemo(() => payment?.address ?? '', [payment?.address])
  const tokenContract = payment?.token_contract || payment?.contract || ''
  const amount = payment?.amount ?? ''

  const copyAddress = async () => {
    if (!payment?.address) return
    await navigator.clipboard.writeText(payment.address)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('USDT TRC20 payment')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Card className='mx-auto max-w-lg'>
          <CardHeader>
            <CardTitle>{t('Send the exact amount')}</CardTitle>
          </CardHeader>
          <CardContent className='space-y-5'>
            {error && (
              <p className='text-destructive text-sm'>
                {t('Unable to load payment status')}
              </p>
            )}
            {payment ? (
              <>
                <div className='flex justify-center rounded-lg bg-white p-4'>
                  <QRCodeSVG value={qrValue} size={220} />
                </div>
                <div className='text-center'>
                  <p className='text-muted-foreground text-sm'>{t('Amount')}</p>
                  <p className='text-3xl font-semibold'>{amount} USDT</p>
                  <p className='text-muted-foreground text-sm'>
                    {payment.network} · {payment.token}
                  </p>
                </div>
                <div className='space-y-2'>
                  <p className='text-muted-foreground text-sm'>
                    {t('Payment address')}
                  </p>
                  <div className='flex items-center gap-2'>
                    <code className='bg-muted min-w-0 flex-1 rounded p-2 text-xs break-all'>
                      {payment.address}
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
                    {t('Only send USDT on the TRON (TRC20) network.')}
                  </p>
                  <p className='text-muted-foreground text-xs break-all'>
                    {t('Token contract')}: {tokenContract}
                  </p>
                </div>
                <div className='text-center text-sm'>
                  {status === 'pending' && (
                    <span>
                      {t('Expires in {{minutes}}:{{seconds}}', {
                        minutes,
                        seconds,
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
                  {!status && <Loader2 className='mx-auto animate-spin' />}
                </div>
              </>
            ) : (
              <Loader2 className='mx-auto animate-spin' />
            )}
            <Link
              to='/wallet'
              className={buttonVariants({
                variant: 'outline',
                className: 'w-full',
              })}
            >
              {t('Back to wallet')}
            </Link>
          </CardContent>
        </Card>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}

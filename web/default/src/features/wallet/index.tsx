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
import { useState, useEffect, useCallback } from 'react'
import { useTranslation } from 'react-i18next'

import { SectionPageLayout } from '@/components/layout'
import { getSelf } from '@/lib/api'

import { syncYooKassaPayment, isApiSuccess } from './api'
import { BillingHistoryDialog } from './components/dialogs/billing-history-dialog'
import { CreemConfirmDialog } from './components/dialogs/creem-confirm-dialog'
import { TransferDialog } from './components/dialogs/transfer-dialog'
import {
  getRechargeValidationTarget,
  getWaffoPaymentMethod,
  RechargeFormCard,
  type RechargeValidationTarget,
} from './components/recharge-form-card'
import { SubscriptionPlansCard } from './components/subscription-plans-card'
import {
  RecentOperationsCard,
  WalletSummaryCard,
} from './components/wallet-sidebar-cards'
import { WalletStatsCard } from './components/wallet-stats-card'
import {
  useTopupInfo,
  usePayment,
  useAffiliate,
  useRedemption,
  useCreemPayment,
  useWaffoPayment,
  useWaffoPancakePayment,
} from './hooks'
import {
  getMinTopupAmount,
  getPaymentCheckoutKind,
  getPaymentMethodDisplayQuote,
} from './lib'
import type { UserWalletData, PaymentMethod, CreemProduct } from './types'

interface WalletProps {
  initialShowHistory?: boolean
}

export function Wallet(props: WalletProps) {
  const { t } = useTranslation()
  const [user, setUser] = useState<UserWalletData | null>(null)
  const [topupAmount, setTopupAmount] = useState(0)
  const [selectedPaymentMethod, setSelectedPaymentMethod] =
    useState<PaymentMethod>()
  const [selectedWaffoMethodIndex, setSelectedWaffoMethodIndex] =
    useState<number>()
  const [transferDialogOpen, setTransferDialogOpen] = useState(false)
  const [billingDialogOpen, setBillingDialogOpen] = useState(false)
  const [redemptionCode, setRedemptionCode] = useState('')
  const [creemDialogOpen, setCreemDialogOpen] = useState(false)
  const [selectedCreemProduct, setSelectedCreemProduct] =
    useState<CreemProduct | null>(null)
  const [showSubscriptionPanel, setShowSubscriptionPanel] = useState(true)
  const [validationTarget, setValidationTarget] =
    useState<RechargeValidationTarget | null>(null)

  const { topupInfo, loading: topupLoading } = useTopupInfo()
  const { processing, processPayment } = usePayment()
  const {
    affiliateLink,
    loading: affiliateLoading,
    transferQuota,
    transferring,
  } = useAffiliate()
  const { redeeming, redeemCode } = useRedemption()
  const { processing: creemProcessing, processCreemPayment } = useCreemPayment()
  const { processing: waffoProcessing, processWaffoPayment } = useWaffoPayment()
  const { processing: pancakeProcessing, processWaffoPancakePayment } =
    useWaffoPancakePayment()

  // Fetch and refresh user data
  const fetchUser = useCallback(async () => {
    try {
      const response = await getSelf()
      if (response.success && response.data) {
        setUser(response.data as UserWalletData)
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to fetch user data:', error)
    }
  }, [])

  useEffect(() => {
    fetchUser()
  }, [fetchUser])

  useEffect(() => {
    if (props.initialShowHistory) {
      setBillingDialogOpen(true)
      window.history.replaceState({}, '', window.location.pathname)
    }
  }, [props.initialShowHistory])

  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const tradeNoParam = params.get('trade_no')?.trim()
    if (params.get('payment_provider') !== 'yookassa' || !tradeNoParam) {
      return
    }
    const tradeNo = tradeNoParam

    let cancelled = false
    async function syncPayment() {
      try {
        const response = await syncYooKassaPayment({ trade_no: tradeNo })
        if (cancelled) {
          return
        }
        if (isApiSuccess(response)) {
          await fetchUser()
        }
      } finally {
        if (!cancelled) {
          setBillingDialogOpen(true)
          window.history.replaceState({}, '', window.location.pathname)
        }
      }
    }
    syncPayment()
    return () => {
      cancelled = true
    }
  }, [fetchUser])

  // Handle topup amount change
  const handleTopupAmountChange = (amount: number) => {
    setTopupAmount(amount)
  }

  // Handle payment method selection
  const handlePaymentMethodSelect = (method: PaymentMethod) => {
    setSelectedPaymentMethod(method)
    setSelectedWaffoMethodIndex(undefined)
  }

  // Handle payment submission from the summary.
  const handlePaymentSubmit = async () => {
    if (!selectedPaymentMethod) return

    let success = false
    const checkoutKind = getPaymentCheckoutKind(selectedPaymentMethod.type)
    if (checkoutKind === 'waffo') {
      success = await processWaffoPayment(topupAmount, selectedWaffoMethodIndex)
    } else if (checkoutKind === 'waffo-pancake') {
      success = await processWaffoPancakePayment(topupAmount)
    } else {
      success = await processPayment(topupAmount, selectedPaymentMethod.type)
    }

    if (success) {
      await fetchUser()
    }
  }

  // Handle redemption
  const handleRedeem = async () => {
    if (!redemptionCode) return

    const success = await redeemCode(redemptionCode)
    if (success) {
      setRedemptionCode('')
      await fetchUser()
    }
  }

  // Handle transfer
  const handleTransfer = async (amount: number) => {
    const success = await transferQuota(amount)
    if (success) {
      await fetchUser()
    }
    return success
  }

  // Handle Creem product selection
  const handleCreemProductSelect = (product: CreemProduct) => {
    setSelectedCreemProduct(product)
    setCreemDialogOpen(true)
  }

  // Handle Creem payment confirmation
  const handleCreemConfirm = async () => {
    if (!selectedCreemProduct) return

    const success = await processCreemPayment(selectedCreemProduct.productId)
    if (success) {
      setCreemDialogOpen(false)
      setSelectedCreemProduct(null)
      await fetchUser()
    }
  }

  const handleWaffoMethodSelect = (
    method: Parameters<typeof getWaffoPaymentMethod>[0],
    index: number
  ) => {
    setSelectedWaffoMethodIndex(index)
    setSelectedPaymentMethod(getWaffoPaymentMethod(method))
  }

  const handleValidationRequest = useCallback(() => {
    setValidationTarget(
      getRechargeValidationTarget(
        topupAmount,
        getMinTopupAmount(topupInfo),
        Boolean(selectedPaymentMethod)
      )
    )
  }, [selectedPaymentMethod, topupAmount, topupInfo])

  useEffect(() => {
    if (!validationTarget) return
    const nextTarget = getRechargeValidationTarget(
      topupAmount,
      getMinTopupAmount(topupInfo),
      Boolean(selectedPaymentMethod)
    )
    if (nextTarget !== validationTarget) {
      setValidationTarget(null)
    }
  }, [selectedPaymentMethod, topupAmount, topupInfo, validationTarget])

  const handleSubscriptionAvailabilityChange = useCallback(
    (available: boolean) => {
      setShowSubscriptionPanel(available)
    },
    []
  )

  const selectedPaymentQuote = selectedPaymentMethod
    ? getPaymentMethodDisplayQuote(
        topupAmount,
        selectedPaymentMethod,
        topupInfo?.cashback ?? []
      )
    : null

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>
          <span className='block text-xl sm:text-2xl'>
            {t('Balance top-up')}
          </span>
        </SectionPageLayout.Title>
        <SectionPageLayout.Content>
          <div className='flex w-full flex-col gap-4 sm:gap-5'>
            <div className='grid gap-4 xl:grid-cols-[minmax(0,2fr)_minmax(320px,0.95fr)] xl:items-start'>
              <div id='wallet-add-funds' className='scroll-mt-4 space-y-4'>
                <RechargeFormCard
                  topupInfo={topupInfo}
                  topupAmount={topupAmount}
                  onTopupAmountChange={handleTopupAmountChange}
                  onPaymentMethodSelect={handlePaymentMethodSelect}
                  selectedPaymentMethod={selectedPaymentMethod}
                  selectedWaffoMethodIndex={selectedWaffoMethodIndex}
                  redemptionCode={redemptionCode}
                  onRedemptionCodeChange={setRedemptionCode}
                  onRedeem={handleRedeem}
                  redeeming={redeeming}
                  topupLink={topupInfo?.topup_link}
                  loading={topupLoading}
                  creemProducts={topupInfo?.creem_products}
                  enableCreemTopup={topupInfo?.enable_creem_topup}
                  onCreemProductSelect={handleCreemProductSelect}
                  enableWaffoTopup={topupInfo?.enable_waffo_topup}
                  waffoPayMethods={topupInfo?.waffo_pay_methods}
                  waffoMinTopup={topupInfo?.waffo_min_topup}
                  onWaffoMethodSelect={handleWaffoMethodSelect}
                  enableWaffoPancakeTopup={
                    topupInfo?.enable_waffo_pancake_topup
                  }
                  affiliateUser={user}
                  affiliateLink={affiliateLink}
                  onTransferAffiliate={() => setTransferDialogOpen(true)}
                  affiliateLoading={affiliateLoading}
                  affiliateComplianceConfirmed={
                    topupInfo?.payment_compliance_confirmed !== false
                  }
                  validationTarget={validationTarget}
                  onValidationRequest={handleValidationRequest}
                />
              </div>

              <aside className='space-y-4'>
                <WalletStatsCard user={user} loading={!user} />
                <WalletSummaryCard
                  topupAmount={topupAmount}
                  selectedPaymentMethod={selectedPaymentMethod}
                  cashback={topupInfo?.cashback ?? []}
                  onPay={handlePaymentSubmit}
                  onPayUnavailable={handleValidationRequest}
                  payDisabled={
                    topupAmount <= 0 ||
                    topupAmount < getMinTopupAmount(topupInfo) ||
                    !selectedPaymentMethod ||
                    !selectedPaymentQuote ||
                    processing ||
                    pancakeProcessing ||
                    waffoProcessing
                  }
                />
                <RecentOperationsCard />
              </aside>
            </div>

            {showSubscriptionPanel && (
              <SubscriptionPlansCard
                topupInfo={topupInfo}
                onAvailabilityChange={handleSubscriptionAvailabilityChange}
                userQuota={user?.quota}
                onPurchaseSuccess={fetchUser}
              />
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <TransferDialog
        open={transferDialogOpen}
        onOpenChange={setTransferDialogOpen}
        onConfirm={handleTransfer}
        availableQuota={user?.aff_quota ?? 0}
        transferring={transferring}
      />

      <BillingHistoryDialog
        open={billingDialogOpen}
        onOpenChange={setBillingDialogOpen}
      />

      <CreemConfirmDialog
        open={creemDialogOpen}
        onOpenChange={setCreemDialogOpen}
        onConfirm={handleCreemConfirm}
        product={selectedCreemProduct}
        processing={creemProcessing}
      />
    </>
  )
}

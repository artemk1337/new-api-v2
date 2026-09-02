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
import { SettingsPage } from '../components/settings-page'
import { resolveCryptoTailLimitSettings } from '../integrations/crypto-payment-settings'
import type { BillingSettings } from '../types'
import {
  BILLING_DEFAULT_SECTION,
  getBillingSectionContent,
  getBillingSectionMeta,
} from './section-registry.tsx'

const defaultBillingSettings: BillingSettings = {
  QuotaForNewUser: 0,
  PreConsumedQuota: 0,
  QuotaForInviter: 0,
  QuotaForInvitee: 0,
  ReferralDepositPercent: 0,
  ReferralRequiredTopUpUSD: 100,
  TopUpLink: '',
  'general_setting.docs_link': '',
  'quota_setting.enable_free_model_pre_consume': true,
  QuotaPerUnit: 500000,
  USDExchangeRate: 7,
  'general_setting.quota_display_type': 'USD',
  'general_setting.custom_currency_symbol': '¤',
  'general_setting.custom_currency_exchange_rate': 1,
  DisplayInCurrencyEnabled: true,
  DisplayTokenStatEnabled: true,
  'currency_exchange_rate.provider': 'bybit_p2p',
  'currency_exchange_rate.update_interval': 'day',
  ModelPrice: '',
  ModelRatio: '',
  CacheRatio: '',
  CreateCacheRatio: '',
  CompletionRatio: '',
  ImageRatio: '',
  AudioRatio: '',
  AudioCompletionRatio: '',
  ExposeRatioEnabled: false,
  'billing_setting.billing_mode': '{}',
  'billing_setting.billing_expr': '{}',
  'billing_setting.task_price_unit': '{}',
  'tool_price_setting.prices': '{}',
  TopupGroupRatio: '',
  GroupRatio: '',
  PricingGroups: '',
  GroupGroupRatio: '',
  DefaultUseAutoGroup: false,
  'group_ratio_setting.group_special_usable_group': '{}',
  PayAddress: '',
  EpayId: '',
  EpayKey: '',
  Price: 7.3,
  MinTopUp: 1,
  PaymentPendingTTLMinutes: 1440,
  CustomCallbackAddress: '',
  PayMethods: '',
  PaymentMethodAvailableIcons: '',
  'payment_setting.amount_options': '',
  'payment_setting.amount_cashback': '',
  'payment_setting.compliance_confirmed': false,
  'payment_setting.compliance_terms_version': '',
  'payment_setting.compliance_confirmed_at': 0,
  'payment_setting.compliance_confirmed_by': 0,
  'payment_setting.compliance_confirmed_ip': '',
  StripeApiSecret: '',
  StripeWebhookSecret: '',
  StripePriceId: '',
  StripeUnitPrice: 8.0,
  StripeMinTopUp: 1,
  StripePromotionCodesEnabled: false,
  WaffoEnabled: false,
  WaffoApiKey: '',
  WaffoPrivateKey: '',
  WaffoPublicCert: '',
  WaffoSandboxPublicCert: '',
  WaffoSandboxApiKey: '',
  WaffoSandboxPrivateKey: '',
  WaffoSandbox: false,
  WaffoMerchantId: '',
  WaffoCurrency: 'USD',
  WaffoUnitPrice: 1,
  WaffoMinTopUp: 1,
  WaffoNotifyUrl: '',
  WaffoReturnUrl: '',
  WaffoPayMethods: '[]',
  WaffoPancakeMerchantID: '',
  WaffoPancakePrivateKey: '',
  WaffoPancakeReturnURL: '',
  WaffoPancakeMinTopUp: 1,
  YooKassaEnabled: false,
  YooKassaShopID: '',
  YooKassaSecretKey: '',
  YooKassaReturnURL: '',
  YooKassaPaymentMethods: 'sbp',
  NOWPaymentsEnabled: false,
  NOWPaymentsAPIKey: '',
  NOWPaymentsIPNSecret: '',
  NOWPaymentsPriceCurrency: 'usd',
  NOWPaymentsPayCurrency: '',
  NOWPaymentsIPNCallbackURL: '',
  USDTTRC20Enabled: false,
  USDTTRC20ReceivingAddress: '',
  USDTTONReceivingAddress: '',
  USDTSolanaReceivingAddress: '',
  USDTSolanaReceivingTokenAccount: '',
  USDTTRC20AmountTailLimitUnits: 10000,
  USDTTRC20AmountPrecision: 6,
  USDTTRC20APIKey: '',
  WaffoPancakeStoreID: '',
  WaffoPancakeProductID: '',
  'checkin_setting.enabled': false,
  'checkin_setting.min_quota': 1000,
  'checkin_setting.max_quota': 10000,
}

export function BillingSettings() {
  return (
    <SettingsPage
      routePath='/_authenticated/system-settings/billing/$section'
      defaultSettings={defaultBillingSettings}
      defaultSection={BILLING_DEFAULT_SECTION}
      getSectionContent={getBillingSectionContent}
      getSectionMeta={getBillingSectionMeta}
      resolveSettings={resolveCryptoTailLimitSettings}
    />
  )
}

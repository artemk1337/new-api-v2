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
import { CheckinSettingsSection } from '../general/checkin-settings-section'
import { PlatformCurrenciesSection } from '../general/platform-currencies-section'
import { QuotaSettingsSection } from '../general/quota-settings-section'
import { ReferralSettingsSection } from '../general/referral-settings-section'
import {
  CRYPTO_AMOUNT_TAIL_DEFAULT_UNITS,
  microUnitsToDecimalUsdt,
} from '../integrations/crypto-payment-settings'
import { PaymentSettingsSection } from '../integrations/payment-settings-section'
import { RatioSettingsCard } from '../models/ratio-settings-card'
import type { BillingSettings } from '../types'
import { createSectionRegistry } from '../utils/section-registry'

const getModelDefaults = (settings: BillingSettings) => ({
  ModelPrice: settings.ModelPrice,
  ModelRatio: settings.ModelRatio,
  CacheRatio: settings.CacheRatio,
  CreateCacheRatio: settings.CreateCacheRatio,
  CompletionRatio: settings.CompletionRatio,
  ImageRatio: settings.ImageRatio,
  AudioRatio: settings.AudioRatio,
  AudioCompletionRatio: settings.AudioCompletionRatio,
  ExposeRatioEnabled: settings.ExposeRatioEnabled,
  BillingMode: settings['billing_setting.billing_mode'],
  BillingExpr: settings['billing_setting.billing_expr'],
  TaskPriceUnit: settings['billing_setting.task_price_unit'],
})

const getGroupDefaults = (settings: BillingSettings) => ({
  TopupGroupRatio: settings.TopupGroupRatio,
  GroupRatio: settings.GroupRatio,
  PricingGroups: settings.PricingGroups,
  GroupGroupRatio: settings.GroupGroupRatio,
  DefaultUseAutoGroup: settings.DefaultUseAutoGroup,
  GroupSpecialUsableGroup:
    settings['group_ratio_setting.group_special_usable_group'],
})

const BILLING_SECTIONS = [
  {
    id: 'quota',
    titleKey: 'System menu: Quota',
    build: (settings: BillingSettings) => (
      <QuotaSettingsSection
        defaultValues={{
          QuotaForNewUser: settings.QuotaForNewUser,
          PreConsumedQuota: settings.PreConsumedQuota,
          TopUpLink: settings.TopUpLink,
          general_setting: {
            docs_link: settings['general_setting.docs_link'],
          },
          quota_setting: {
            enable_free_model_pre_consume:
              settings['quota_setting.enable_free_model_pre_consume'],
          },
        }}
      />
    ),
  },
  {
    id: 'referral-program',
    titleKey: 'Referral Program',
    build: (settings: BillingSettings) => (
      <ReferralSettingsSection
        defaultValues={{
          QuotaForInviter: settings.QuotaForInviter,
          QuotaForInvitee: settings.QuotaForInvitee,
          ReferralDepositPercent: settings.ReferralDepositPercent,
        }}
        complianceConfirmed={
          (settings['payment_setting.compliance_confirmed'] ?? false) &&
          settings['payment_setting.compliance_terms_version'] === 'v1'
        }
      />
    ),
  },
  {
    id: 'model-pricing',
    titleKey: 'Models',
    build: (settings: BillingSettings) => (
      <RatioSettingsCard
        titleKey='Models'
        modelDefaults={getModelDefaults(settings)}
        groupDefaults={getGroupDefaults(settings)}
        toolPricesDefault={settings['tool_price_setting.prices']}
        visibleTabs={['models', 'tool-prices', 'upstream-sync']}
      />
    ),
  },
  {
    id: 'platform-currencies',
    titleKey: 'Platform currencies',
    build: () => <PlatformCurrenciesSection />,
  },
  {
    id: 'group-pricing',
    titleKey: 'Groups',
    build: (settings: BillingSettings) => (
      <RatioSettingsCard
        titleKey='Groups'
        modelDefaults={getModelDefaults(settings)}
        groupDefaults={getGroupDefaults(settings)}
        toolPricesDefault={settings['tool_price_setting.prices']}
        visibleTabs={['groups']}
      />
    ),
  },
  {
    id: 'payment',
    titleKey: 'Payment Gateway',
    build: (settings: BillingSettings) => (
      <PaymentSettingsSection
        topupGroupRatio={settings.TopupGroupRatio}
        defaultValues={{
          PayAddress: settings.PayAddress,
          EpayId: settings.EpayId,
          EpayKey: settings.EpayKey,
          MinTopUp: settings.MinTopUp,
          PaymentPendingTTLMinutes: settings.PaymentPendingTTLMinutes,
          CustomCallbackAddress: settings.CustomCallbackAddress,
          PayMethods: settings.PayMethods,
          PaymentMethodAvailableIcons: settings.PaymentMethodAvailableIcons ?? '',
          AmountOptions: settings['payment_setting.amount_options'],
          AmountCashback: settings['payment_setting.amount_cashback'],
          StripeApiSecret: settings.StripeApiSecret,
          StripeWebhookSecret: settings.StripeWebhookSecret,
          StripePriceId: settings.StripePriceId,
          StripeUnitPrice: settings.StripeUnitPrice,
          StripeMinTopUp: settings.StripeMinTopUp,
          StripePromotionCodesEnabled: settings.StripePromotionCodesEnabled,
          CreemApiKey: settings.CreemApiKey,
          CreemWebhookSecret: settings.CreemWebhookSecret,
          CreemTestMode: settings.CreemTestMode,
          CreemProducts: settings.CreemProducts,
          YooKassaEnabled: settings.YooKassaEnabled ?? false,
          YooKassaShopID: settings.YooKassaShopID ?? '',
          YooKassaSecretKey: settings.YooKassaSecretKey ?? '',
          YooKassaReturnURL: settings.YooKassaReturnURL ?? '',
          YooKassaPaymentMethods: settings.YooKassaPaymentMethods ?? 'sbp',
          NOWPaymentsEnabled: settings.NOWPaymentsEnabled ?? false,
          NOWPaymentsAPIKey: settings.NOWPaymentsAPIKey ?? '',
          NOWPaymentsIPNSecret: settings.NOWPaymentsIPNSecret ?? '',
          NOWPaymentsIPNCallbackURL: settings.NOWPaymentsIPNCallbackURL ?? '',
          USDTTRC20Enabled: settings.USDTTRC20Enabled ?? false,
          USDTTRC20ReceivingAddress: settings.USDTTRC20ReceivingAddress ?? '',
          USDTTRC20APIKey: settings.USDTTRC20APIKey ?? '',
          USDTTONReceivingAddress: settings.USDTTONReceivingAddress ?? '',
          USDTSolanaReceivingAddress: settings.USDTSolanaReceivingAddress ?? '',
          USDTSolanaReceivingTokenAccount:
            settings.USDTSolanaReceivingTokenAccount ?? '',
          USDTTRC20AmountTailLimitUnits: microUnitsToDecimalUsdt(
            settings.USDTTRC20AmountTailLimitUnits ??
              CRYPTO_AMOUNT_TAIL_DEFAULT_UNITS
          ),
        }}
        waffoDefaultValues={{
          WaffoEnabled: settings.WaffoEnabled ?? false,
          WaffoApiKey: settings.WaffoApiKey ?? '',
          WaffoPrivateKey: settings.WaffoPrivateKey ?? '',
          WaffoPublicCert: settings.WaffoPublicCert ?? '',
          WaffoSandboxPublicCert: settings.WaffoSandboxPublicCert ?? '',
          WaffoSandboxApiKey: settings.WaffoSandboxApiKey ?? '',
          WaffoSandboxPrivateKey: settings.WaffoSandboxPrivateKey ?? '',
          WaffoSandbox: settings.WaffoSandbox ?? false,
          WaffoMerchantId: settings.WaffoMerchantId ?? '',
          WaffoCurrency: settings.WaffoCurrency ?? 'USD',
          WaffoUnitPrice: settings.WaffoUnitPrice ?? 1,
          WaffoMinTopUp: settings.WaffoMinTopUp ?? 1,
          WaffoNotifyUrl: settings.WaffoNotifyUrl ?? '',
          WaffoReturnUrl: settings.WaffoReturnUrl ?? '',
          WaffoPayMethods: settings.WaffoPayMethods ?? '[]',
        }}
        waffoPancakeDefaultValues={{
          WaffoPancakeMerchantID: settings.WaffoPancakeMerchantID ?? '',
          WaffoPancakePrivateKey: settings.WaffoPancakePrivateKey ?? '',
          WaffoPancakeReturnURL: settings.WaffoPancakeReturnURL ?? '',
          WaffoPancakeMinTopUp: settings.WaffoPancakeMinTopUp ?? 1,
        }}
        waffoPancakeProvisionedStoreID={settings.WaffoPancakeStoreID ?? ''}
        waffoPancakeProvisionedProductID={settings.WaffoPancakeProductID ?? ''}
      />
    ),
  },
  {
    id: 'checkin',
    titleKey: 'Check-in Rewards',
    build: (settings: BillingSettings) => (
      <CheckinSettingsSection
        defaultValues={{
          enabled: settings['checkin_setting.enabled'],
          minQuota: settings['checkin_setting.min_quota'],
          maxQuota: settings['checkin_setting.max_quota'],
        }}
      />
    ),
  },
] as const

export type BillingSectionId = (typeof BILLING_SECTIONS)[number]['id']

const billingRegistry = createSectionRegistry<
  BillingSectionId,
  BillingSettings
>({
  sections: BILLING_SECTIONS,
  defaultSection: 'quota',
  basePath: '/system-settings/billing',
  urlStyle: 'path',
})

export const BILLING_SECTION_IDS = billingRegistry.sectionIds
export const BILLING_DEFAULT_SECTION = billingRegistry.defaultSection
export const getBillingSectionNavItems = billingRegistry.getSectionNavItems
export const getBillingSectionContent = billingRegistry.getSectionContent
export const getBillingSectionMeta = billingRegistry.getSectionMeta

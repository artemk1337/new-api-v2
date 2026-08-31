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
import { zodResolver } from '@hookform/resolvers/zod'
import { useQueryClient } from '@tanstack/react-query'
import { Code2, Eye, ShieldAlert } from 'lucide-react'
import * as React from 'react'
import { useForm, type Resolver } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'
import { useSystemConfigStore } from '@/stores/system-config-store'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { safeNumberFieldProps } from '../utils/numeric-field'
import {
  isValidAmountCashbackConfig,
  normalizeAmountCashbackConfig,
} from './amount-cashback'
import { AmountCashbackVisualEditor } from './amount-cashback-visual-editor'
import { AmountOptionsVisualEditor } from './amount-options-visual-editor'
import {
  buildPaymentSettingsPayload,
  getPaymentSettingsSaveErrorMessage,
  savePaymentSettings,
  shouldUpdateCreemSecret,
} from './creem-config-api'
import { CreemProductsVisualEditor } from './creem-products-visual-editor'
import {
  cryptoAmountTailVariants,
  decimalUsdtToMicroUnits,
  CRYPTO_PAYMENT_CURRENCY,
  shouldUpdateCryptoPaymentCredential,
} from './crypto-payment-settings'
import { PaymentCurrencyField } from './payment-currency-field'
import { parseAvailablePaymentIcons } from './payment-method-icons'
import type { TopupGroupOption } from './payment-method-dialog'
import { PaymentMethodsVisualEditor } from './payment-methods-visual-editor'
import {
  formatJsonForEditor,
  getJsonError,
  normalizeJsonForComparison,
  removeTrailingSlash,
} from './utils'
import { saveWaffoPancakeConfig } from './waffo-pancake-api'
import {
  WaffoPancakeSettingsSection,
  type WaffoPancakeBinding,
  type WaffoPancakeSettingsValues,
} from './waffo-pancake-settings-section'
import {
  type PayMethod,
  WaffoSettingsSection,
  type WaffoSettingsValues,
} from './waffo-settings-section'

function isHttpOriginUrl(value: string) {
  const trimmed = value.trim()
  if (!trimmed) return true

  try {
    const url = new URL(trimmed)
    const isHttpProtocol = url.protocol === 'http:' || url.protocol === 'https:'
    const hasNoPath = url.pathname === '' || url.pathname === '/'
    return isHttpProtocol && hasNoPath && !url.search && !url.hash
  } catch {
    return false
  }
}

const paymentSchema = z
  .object({
    PayAddress: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, 'Provide a valid callback URL starting with http:// or https://'),
    EpayId: z.string(),
    EpayKey: z.string(),
    MinTopUp: z.coerce.number().min(0),
    PaymentPendingTTLMinutes: z.coerce.number().int().min(1),
    CustomCallbackAddress: z
      .string()
      .refine(
        isHttpOriginUrl,
        'Enter only a top-level callback domain, for example https://api.example.com, without any path.'
      ),
    PayMethods: z.string().superRefine((value, ctx) => {
      const error = getJsonError(value)
      if (error) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: error,
        })
      }
    }),
    PaymentMethodAvailableIcons: z.string(),
    AmountOptions: z.string().superRefine((value, ctx) => {
      const error = getJsonError(value, (parsed) => Array.isArray(parsed))
      if (error) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: error,
        })
      }
    }),
    AmountCashback: z.string().superRefine((value, ctx) => {
      const error = getJsonError(value, isValidAmountCashbackConfig)
      if (error) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: error,
        })
      }
    }),
    StripeApiSecret: z.string(),
    StripeWebhookSecret: z.string(),
    StripePriceId: z.string(),
    StripeUnitPrice: z.coerce.number().min(0),
    StripeMinTopUp: z.coerce.number().min(0),
    StripePromotionCodesEnabled: z.boolean(),
    CreemApiKey: z.string(),
    CreemWebhookSecret: z.string(),
    CreemTestMode: z.boolean(),
    CreemProducts: z.string().superRefine((value, ctx) => {
      const error = getJsonError(value, (parsed) => Array.isArray(parsed))
      if (error) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          message: error,
        })
      }
    }),
    WaffoEnabled: z.boolean(),
    WaffoApiKey: z.string(),
    WaffoPrivateKey: z.string(),
    WaffoPublicCert: z.string(),
    WaffoSandboxPublicCert: z.string(),
    WaffoSandboxApiKey: z.string(),
    WaffoSandboxPrivateKey: z.string(),
    WaffoSandbox: z.boolean(),
    WaffoMerchantId: z.string(),
    WaffoCurrency: z.string(),
    WaffoUnitPrice: z.coerce.number().min(0),
    WaffoMinTopUp: z.coerce.number().min(0.01),
    WaffoNotifyUrl: z.string(),
    WaffoReturnUrl: z.string(),
    WaffoPancakeMerchantID: z.string(),
    WaffoPancakePrivateKey: z.string(),
    WaffoPancakeReturnURL: z.string(),
    WaffoPancakeMinTopUp: z.coerce.number().min(0.01),
    YooKassaEnabled: z.boolean(),
    YooKassaShopID: z.string(),
    YooKassaSecretKey: z.string(),
    YooKassaReturnURL: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, 'Provide a valid URL starting with http:// or https://'),
    YooKassaPaymentMethods: z.string().refine((value) => {
      return value.trim().toLowerCase() === 'sbp'
    }, 'Only SBP is supported for YooKassa payments'),
    NOWPaymentsEnabled: z.boolean(),
    NOWPaymentsAPIKey: z.string(),
    NOWPaymentsIPNSecret: z.string(),
    NOWPaymentsIPNCallbackURL: z.string().refine((value) => {
      const trimmed = value.trim()
      if (!trimmed) return true
      return /^https?:\/\//.test(trimmed)
    }, 'Provide a valid URL starting with http:// or https://'),
    USDTTRC20Enabled: z.boolean(),
    USDTTRC20ReceivingAddress: z.string(),
    USDTTRC20APIKey: z.string(),
    USDTTONReceivingAddress: z.string(),
    USDTSolanaReceivingAddress: z.string(),
    USDTSolanaReceivingTokenAccount: z.string(),
    USDTTRC20AmountTailLimitUnits: z.string().refine(
      (value) => decimalUsdtToMicroUnits(value) !== null,
      'Enter a value from 0.000002 to 0.01 USDT with up to 6 decimals'
    ),
  })

type PaymentFormValues = z.infer<typeof paymentSchema>
type WaffoFormFieldValues = Omit<WaffoSettingsValues, 'WaffoPayMethods'>
type PaymentBaseFormValues = Omit<
  PaymentFormValues,
  keyof WaffoFormFieldValues | keyof WaffoPancakeSettingsValues
>

const paymentTabContentClassName = 'mt-6 min-w-0'

type PaymentSettingsSectionProps = {
  topupGroupRatio: string
  defaultValues: PaymentBaseFormValues
  waffoDefaultValues: WaffoSettingsValues
  waffoPancakeDefaultValues: WaffoPancakeSettingsValues
  waffoPancakeProvisionedStoreID?: string
  waffoPancakeProvisionedProductID?: string
}

function parseWaffoPayMethods(value: string): PayMethod[] {
  try {
    const parsed = JSON.parse(value || '[]')
    return Array.isArray(parsed) ? parsed : []
  } catch {
    return []
  }
}

function normalizeYooKassaPaymentMethods(_value: string): string {
  return 'sbp'
}

export function PaymentSettingsSection({
  topupGroupRatio,
  defaultValues,
  waffoDefaultValues,
  waffoPancakeDefaultValues,
  waffoPancakeProvisionedStoreID,
  waffoPancakeProvisionedProductID,
}: PaymentSettingsSectionProps) {
  const { t } = useTranslation()
  const tokenAmounts =
    useSystemConfigStore((state) => state.config.currency.quotaDisplayType) ===
    'TOKENS'
  const queryClient = useQueryClient()
  const initialFormValues = React.useMemo<PaymentFormValues>(
    () => ({
      ...defaultValues,
      ...waffoDefaultValues,
      ...waffoPancakeDefaultValues,
    }),
    [defaultValues, waffoDefaultValues, waffoPancakeDefaultValues]
  )
  const initialRef = React.useRef(initialFormValues)
  const defaultsSignature = React.useMemo(
    () => JSON.stringify(initialFormValues),
    [initialFormValues]
  )

  const [payMethodsVisualMode, setPayMethodsVisualMode] = React.useState(true)
  const [amountOptionsVisualMode, setAmountOptionsVisualMode] =
    React.useState(true)
  const [amountCashbackVisualMode, setAmountCashbackVisualMode] =
    React.useState(true)
  const [creemProductsVisualMode, setCreemProductsVisualMode] =
    React.useState(true)
  const [waffoPayMethods, setWaffoPayMethods] = React.useState<PayMethod[]>(
    () => parseWaffoPayMethods(waffoDefaultValues.WaffoPayMethods)
  )
  const [waffoPancakeSelection, setWaffoPancakeSelection] =
    React.useState<WaffoPancakeBinding>({
      storeID: waffoPancakeProvisionedStoreID ?? '',
      productID: waffoPancakeProvisionedProductID ?? '',
    })
  const [waffoPancakeSavedBinding, setWaffoPancakeSavedBinding] =
    React.useState<WaffoPancakeBinding>({
      storeID: waffoPancakeProvisionedStoreID ?? '',
      productID: waffoPancakeProvisionedProductID ?? '',
    })
  const topupGroups = React.useMemo<TopupGroupOption[]>(() => {
    try {
      const parsed = JSON.parse(topupGroupRatio || '{}')
      if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
        return []
      }

      return Object.entries(parsed).flatMap(([name, ratio]) =>
        typeof ratio === 'number' && Number.isFinite(ratio)
          ? [{ name, ratio }]
          : []
      )
    } catch {
      return []
    }
  }, [topupGroupRatio])

  React.useEffect(() => {
    setWaffoPayMethods(parseWaffoPayMethods(waffoDefaultValues.WaffoPayMethods))
  }, [waffoDefaultValues.WaffoPayMethods])

  React.useEffect(() => {
    const nextBinding = {
      storeID: waffoPancakeProvisionedStoreID ?? '',
      productID: waffoPancakeProvisionedProductID ?? '',
    }
    setWaffoPancakeSelection(nextBinding)
    setWaffoPancakeSavedBinding(nextBinding)
  }, [waffoPancakeProvisionedProductID, waffoPancakeProvisionedStoreID])

  const form = useForm<PaymentFormValues>({
    resolver: zodResolver(paymentSchema) as Resolver<PaymentFormValues>,
    mode: 'onChange', // Enable real-time validation
    defaultValues: {
      ...initialFormValues,
      PayMethods: formatJsonForEditor(initialFormValues.PayMethods),
      AmountOptions: formatJsonForEditor(initialFormValues.AmountOptions),
      AmountCashback: formatJsonForEditor(initialFormValues.AmountCashback),
      CreemProducts: formatJsonForEditor(initialFormValues.CreemProducts),
    },
  })

  const waffoCurrency = form.watch('WaffoCurrency')
  const availablePaymentIcons = parseAvailablePaymentIcons(
    form.watch('PaymentMethodAvailableIcons')
  )
  const [creemSecretClearRequested, setCreemSecretClearRequested] =
    React.useState({ apiKey: false, webhookSecret: false })

  const { isSubmitting, dirtyFields } = form.formState

  const setPaymentValue = React.useCallback(
    (
      key: keyof PaymentFormValues,
      value: PaymentFormValues[keyof PaymentFormValues]
    ) => {
      form.setValue(
        key as Parameters<typeof form.setValue>[0],
        value as Parameters<typeof form.setValue>[1],
        {
          shouldDirty: true,
          shouldValidate: true,
        }
      )
    },
    [form]
  )

  const setWaffoValue = React.useCallback(
    <K extends keyof WaffoFormFieldValues>(
      key: K,
      value: WaffoFormFieldValues[K]
    ) => {
      setPaymentValue(
        key as keyof PaymentFormValues,
        value as PaymentFormValues[keyof PaymentFormValues]
      )
    },
    [setPaymentValue]
  )

  const setWaffoPancakeValue = React.useCallback(
    <K extends keyof WaffoPancakeSettingsValues>(
      key: K,
      value: WaffoPancakeSettingsValues[K]
    ) => {
      setPaymentValue(
        key as keyof PaymentFormValues,
        value as PaymentFormValues[keyof PaymentFormValues]
      )
    },
    [setPaymentValue]
  )

  React.useEffect(() => {
    const parsedDefaults = JSON.parse(defaultsSignature) as PaymentFormValues
    initialRef.current = parsedDefaults
    setCreemSecretClearRequested({ apiKey: false, webhookSecret: false })
    form.reset({
      ...parsedDefaults,
      PayMethods: formatJsonForEditor(parsedDefaults.PayMethods),
      AmountOptions: formatJsonForEditor(parsedDefaults.AmountOptions),
      AmountCashback: formatJsonForEditor(parsedDefaults.AmountCashback),
      CreemProducts: formatJsonForEditor(parsedDefaults.CreemProducts),
    })
  }, [defaultsSignature, form])

  const onSubmit = async (values: PaymentFormValues) => {
    const sanitized = {
      PayAddress: removeTrailingSlash(values.PayAddress),
      EpayId: values.EpayId.trim(),
      EpayKey: values.EpayKey.trim(),
      MinTopUp: values.MinTopUp,
      PaymentPendingTTLMinutes: values.PaymentPendingTTLMinutes,
      CustomCallbackAddress: removeTrailingSlash(values.CustomCallbackAddress),
      PayMethods: values.PayMethods.trim(),
      PaymentMethodAvailableIcons: values.PaymentMethodAvailableIcons,
      AmountOptions: values.AmountOptions.trim(),
      AmountCashback: normalizeAmountCashbackConfig(values.AmountCashback),
      StripeApiSecret: values.StripeApiSecret.trim(),
      StripeWebhookSecret: values.StripeWebhookSecret.trim(),
      StripePriceId: values.StripePriceId.trim(),
      StripeUnitPrice: values.StripeUnitPrice,
      StripeMinTopUp: values.StripeMinTopUp,
      StripePromotionCodesEnabled: values.StripePromotionCodesEnabled,
      CreemApiKey: values.CreemApiKey.trim(),
      CreemWebhookSecret: values.CreemWebhookSecret.trim(),
      CreemTestMode: values.CreemTestMode,
      CreemProducts: values.CreemProducts.trim(),
      WaffoEnabled: values.WaffoEnabled,
      WaffoSandbox: values.WaffoSandbox,
      WaffoMerchantId: values.WaffoMerchantId.trim(),
      WaffoCurrency: values.WaffoCurrency.trim() || 'USD',
      WaffoUnitPrice: values.WaffoUnitPrice,
      WaffoMinTopUp: values.WaffoMinTopUp,
      WaffoNotifyUrl: values.WaffoNotifyUrl.trim(),
      WaffoReturnUrl: values.WaffoReturnUrl.trim(),
      WaffoPublicCert: values.WaffoPublicCert.trim(),
      WaffoSandboxPublicCert: values.WaffoSandboxPublicCert.trim(),
      WaffoApiKey: values.WaffoApiKey.trim(),
      WaffoPrivateKey: values.WaffoPrivateKey.trim(),
      WaffoSandboxApiKey: values.WaffoSandboxApiKey.trim(),
      WaffoSandboxPrivateKey: values.WaffoSandboxPrivateKey.trim(),
      WaffoPayMethods: JSON.stringify(waffoPayMethods),
      WaffoPancakeMerchantID: values.WaffoPancakeMerchantID.trim(),
      WaffoPancakePrivateKey: values.WaffoPancakePrivateKey.trim(),
      WaffoPancakeReturnURL: removeTrailingSlash(
        values.WaffoPancakeReturnURL.trim()
      ),
      WaffoPancakeMinTopUp: values.WaffoPancakeMinTopUp,
      YooKassaEnabled: values.YooKassaEnabled,
      YooKassaShopID: values.YooKassaShopID.trim(),
      YooKassaSecretKey: values.YooKassaSecretKey.trim(),
      YooKassaReturnURL: removeTrailingSlash(values.YooKassaReturnURL.trim()),
      YooKassaPaymentMethods: normalizeYooKassaPaymentMethods(
        values.YooKassaPaymentMethods
      ),
      NOWPaymentsEnabled: values.NOWPaymentsEnabled,
      NOWPaymentsAPIKey: values.NOWPaymentsAPIKey.trim(),
      NOWPaymentsIPNSecret: values.NOWPaymentsIPNSecret.trim(),
      NOWPaymentsIPNCallbackURL: removeTrailingSlash(
        values.NOWPaymentsIPNCallbackURL.trim()
      ),
      USDTTRC20Enabled: values.USDTTRC20Enabled,
      USDTTRC20ReceivingAddress: values.USDTTRC20ReceivingAddress.trim(),
      USDTTRC20APIKey: values.USDTTRC20APIKey.trim(),
      USDTTONReceivingAddress: values.USDTTONReceivingAddress.trim(),
      USDTSolanaReceivingAddress: values.USDTSolanaReceivingAddress.trim(),
      USDTSolanaReceivingTokenAccount:
        values.USDTSolanaReceivingTokenAccount.trim(),
      USDTTRC20AmountTailLimitUnits: decimalUsdtToMicroUnits(
        values.USDTTRC20AmountTailLimitUnits
      ) as number,
    }

    const initial = {
      PayAddress: removeTrailingSlash(initialRef.current.PayAddress),
      EpayId: initialRef.current.EpayId.trim(),
      EpayKey: initialRef.current.EpayKey.trim(),
      MinTopUp: initialRef.current.MinTopUp,
      PaymentPendingTTLMinutes: initialRef.current.PaymentPendingTTLMinutes,
      CustomCallbackAddress: removeTrailingSlash(
        initialRef.current.CustomCallbackAddress
      ),
      PayMethods: initialRef.current.PayMethods.trim(),
      PaymentMethodAvailableIcons: initialRef.current.PaymentMethodAvailableIcons,
      AmountOptions: initialRef.current.AmountOptions.trim(),
      AmountCashback: normalizeAmountCashbackConfig(
        initialRef.current.AmountCashback
      ),
      StripeApiSecret: initialRef.current.StripeApiSecret.trim(),
      StripeWebhookSecret: initialRef.current.StripeWebhookSecret.trim(),
      StripePriceId: initialRef.current.StripePriceId.trim(),
      StripeUnitPrice: initialRef.current.StripeUnitPrice,
      StripeMinTopUp: initialRef.current.StripeMinTopUp,
      StripePromotionCodesEnabled:
        initialRef.current.StripePromotionCodesEnabled,
      CreemApiKey: initialRef.current.CreemApiKey.trim(),
      CreemWebhookSecret: initialRef.current.CreemWebhookSecret.trim(),
      CreemTestMode: initialRef.current.CreemTestMode,
      CreemProducts: initialRef.current.CreemProducts.trim(),
      WaffoEnabled: initialRef.current.WaffoEnabled,
      WaffoSandbox: initialRef.current.WaffoSandbox,
      WaffoMerchantId: initialRef.current.WaffoMerchantId.trim(),
      WaffoCurrency: initialRef.current.WaffoCurrency.trim() || 'USD',
      WaffoUnitPrice: initialRef.current.WaffoUnitPrice,
      WaffoMinTopUp: initialRef.current.WaffoMinTopUp,
      WaffoNotifyUrl: initialRef.current.WaffoNotifyUrl.trim(),
      WaffoReturnUrl: initialRef.current.WaffoReturnUrl.trim(),
      WaffoPublicCert: initialRef.current.WaffoPublicCert.trim(),
      WaffoSandboxPublicCert: initialRef.current.WaffoSandboxPublicCert.trim(),
      WaffoApiKey: initialRef.current.WaffoApiKey.trim(),
      WaffoPrivateKey: initialRef.current.WaffoPrivateKey.trim(),
      WaffoSandboxApiKey: initialRef.current.WaffoSandboxApiKey.trim(),
      WaffoSandboxPrivateKey: initialRef.current.WaffoSandboxPrivateKey.trim(),
      WaffoPayMethods: JSON.stringify(
        parseWaffoPayMethods(waffoDefaultValues.WaffoPayMethods)
      ),
      WaffoPancakeMerchantID: initialRef.current.WaffoPancakeMerchantID.trim(),
      WaffoPancakePrivateKey: initialRef.current.WaffoPancakePrivateKey.trim(),
      WaffoPancakeReturnURL: removeTrailingSlash(
        initialRef.current.WaffoPancakeReturnURL.trim()
      ),
      WaffoPancakeMinTopUp: initialRef.current.WaffoPancakeMinTopUp,
      YooKassaEnabled: initialRef.current.YooKassaEnabled,
      YooKassaShopID: initialRef.current.YooKassaShopID.trim(),
      YooKassaSecretKey: initialRef.current.YooKassaSecretKey.trim(),
      YooKassaReturnURL: removeTrailingSlash(
        initialRef.current.YooKassaReturnURL.trim()
      ),
      YooKassaPaymentMethods: initialRef.current.YooKassaPaymentMethods.trim(),
      NOWPaymentsEnabled: initialRef.current.NOWPaymentsEnabled,
      NOWPaymentsAPIKey: initialRef.current.NOWPaymentsAPIKey.trim(),
      NOWPaymentsIPNSecret: initialRef.current.NOWPaymentsIPNSecret.trim(),
      NOWPaymentsIPNCallbackURL: removeTrailingSlash(
        initialRef.current.NOWPaymentsIPNCallbackURL.trim()
      ),
      USDTTRC20Enabled: initialRef.current.USDTTRC20Enabled,
      USDTTRC20ReceivingAddress:
        initialRef.current.USDTTRC20ReceivingAddress.trim(),
      USDTTRC20APIKey: initialRef.current.USDTTRC20APIKey.trim(),
      USDTTONReceivingAddress:
        initialRef.current.USDTTONReceivingAddress.trim(),
      USDTSolanaReceivingAddress:
        initialRef.current.USDTSolanaReceivingAddress.trim(),
      USDTSolanaReceivingTokenAccount:
        initialRef.current.USDTSolanaReceivingTokenAccount.trim(),
      USDTTRC20AmountTailLimitUnits: initialRef.current
        .USDTTRC20AmountTailLimitUnits,
    }

    const updates: Array<{ key: string; value: string | number | boolean }> = []

    if (sanitized.PayAddress !== initial.PayAddress) {
      updates.push({ key: 'PayAddress', value: sanitized.PayAddress })
    }

    if (sanitized.EpayId !== initial.EpayId) {
      updates.push({ key: 'EpayId', value: sanitized.EpayId })
    }

    if (sanitized.EpayKey && sanitized.EpayKey !== initial.EpayKey) {
      updates.push({ key: 'EpayKey', value: sanitized.EpayKey })
    }

    if (sanitized.MinTopUp !== initial.MinTopUp) {
      updates.push({ key: 'MinTopUp', value: sanitized.MinTopUp })
    }

    if (
      sanitized.PaymentPendingTTLMinutes !== initial.PaymentPendingTTLMinutes
    ) {
      updates.push({
        key: 'PaymentPendingTTLMinutes',
        value: sanitized.PaymentPendingTTLMinutes,
      })
    }

    if (sanitized.CustomCallbackAddress !== initial.CustomCallbackAddress) {
      updates.push({
        key: 'CustomCallbackAddress',
        value: sanitized.CustomCallbackAddress,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.PayMethods) !==
      normalizeJsonForComparison(initial.PayMethods)
    ) {
      updates.push({ key: 'PayMethods', value: sanitized.PayMethods })
    }
    if (
      sanitized.PaymentMethodAvailableIcons !==
      initial.PaymentMethodAvailableIcons
    ) {
      updates.push({
        key: 'PaymentMethodAvailableIcons',
        value: sanitized.PaymentMethodAvailableIcons,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.AmountOptions) !==
      normalizeJsonForComparison(initial.AmountOptions)
    ) {
      updates.push({
        key: 'payment_setting.amount_options',
        value: sanitized.AmountOptions,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.AmountCashback) !==
      normalizeJsonForComparison(initial.AmountCashback)
    ) {
      updates.push({
        key: 'payment_setting.amount_cashback',
        value: sanitized.AmountCashback,
      })
    }

    if (
      sanitized.StripeApiSecret &&
      sanitized.StripeApiSecret !== initial.StripeApiSecret
    ) {
      updates.push({
        key: 'StripeApiSecret',
        value: sanitized.StripeApiSecret,
      })
    }

    if (
      sanitized.StripeWebhookSecret &&
      sanitized.StripeWebhookSecret !== initial.StripeWebhookSecret
    ) {
      updates.push({
        key: 'StripeWebhookSecret',
        value: sanitized.StripeWebhookSecret,
      })
    }

    if (sanitized.StripePriceId !== initial.StripePriceId) {
      updates.push({ key: 'StripePriceId', value: sanitized.StripePriceId })
    }

    if (sanitized.StripeUnitPrice !== initial.StripeUnitPrice) {
      updates.push({
        key: 'StripeUnitPrice',
        value: sanitized.StripeUnitPrice,
      })
    }

    if (sanitized.StripeMinTopUp !== initial.StripeMinTopUp) {
      updates.push({ key: 'StripeMinTopUp', value: sanitized.StripeMinTopUp })
    }

    if (
      sanitized.StripePromotionCodesEnabled !==
      initial.StripePromotionCodesEnabled
    ) {
      updates.push({
        key: 'StripePromotionCodesEnabled',
        value: sanitized.StripePromotionCodesEnabled,
      })
    }

    const creemUpdate: {
      api_key?: string
      webhook_secret?: string
      test_mode?: boolean
      products?: string
    } = {}
    if (
      shouldUpdateCreemSecret(
        sanitized.CreemApiKey,
        initial.CreemApiKey,
        !!dirtyFields.CreemApiKey,
        creemSecretClearRequested.apiKey
      )
    ) {
      creemUpdate.api_key = sanitized.CreemApiKey
    }
    if (
      shouldUpdateCreemSecret(
        sanitized.CreemWebhookSecret,
        initial.CreemWebhookSecret,
        !!dirtyFields.CreemWebhookSecret,
        creemSecretClearRequested.webhookSecret
      )
    ) {
      creemUpdate.webhook_secret = sanitized.CreemWebhookSecret
    }
    if (sanitized.CreemTestMode !== initial.CreemTestMode) {
      creemUpdate.test_mode = sanitized.CreemTestMode
    }
    if (
      normalizeJsonForComparison(sanitized.CreemProducts) !==
      normalizeJsonForComparison(initial.CreemProducts)
    ) {
      creemUpdate.products = sanitized.CreemProducts
    }
    const hasCreemChanges = Object.keys(creemUpdate).length > 0

    if (sanitized.YooKassaEnabled !== initial.YooKassaEnabled) {
      updates.push({
        key: 'YooKassaEnabled',
        value: sanitized.YooKassaEnabled,
      })
    }

    if (sanitized.YooKassaShopID !== initial.YooKassaShopID) {
      updates.push({ key: 'YooKassaShopID', value: sanitized.YooKassaShopID })
    }

    if (
      sanitized.YooKassaSecretKey &&
      sanitized.YooKassaSecretKey !== initial.YooKassaSecretKey
    ) {
      updates.push({
        key: 'YooKassaSecretKey',
        value: sanitized.YooKassaSecretKey,
      })
    }

    if (sanitized.YooKassaReturnURL !== initial.YooKassaReturnURL) {
      updates.push({
        key: 'YooKassaReturnURL',
        value: sanitized.YooKassaReturnURL,
      })
    }

    if (sanitized.YooKassaPaymentMethods !== initial.YooKassaPaymentMethods) {
      updates.push({
        key: 'YooKassaPaymentMethods',
        value: sanitized.YooKassaPaymentMethods,
      })
    }

    if (sanitized.NOWPaymentsEnabled !== initial.NOWPaymentsEnabled) {
      updates.push({
        key: 'NOWPaymentsEnabled',
        value: sanitized.NOWPaymentsEnabled,
      })
    }

    if (
      sanitized.NOWPaymentsAPIKey &&
      sanitized.NOWPaymentsAPIKey !== initial.NOWPaymentsAPIKey
    ) {
      updates.push({
        key: 'NOWPaymentsAPIKey',
        value: sanitized.NOWPaymentsAPIKey,
      })
    }

    if (
      sanitized.NOWPaymentsIPNSecret &&
      sanitized.NOWPaymentsIPNSecret !== initial.NOWPaymentsIPNSecret
    ) {
      updates.push({
        key: 'NOWPaymentsIPNSecret',
        value: sanitized.NOWPaymentsIPNSecret,
      })
    }

    if (
      sanitized.NOWPaymentsIPNCallbackURL !== initial.NOWPaymentsIPNCallbackURL
    ) {
      updates.push({
        key: 'NOWPaymentsIPNCallbackURL',
        value: sanitized.NOWPaymentsIPNCallbackURL,
      })
    }

    if (
      sanitized.USDTTRC20ReceivingAddress !== initial.USDTTRC20ReceivingAddress
    ) {
      updates.push({
        key: 'USDTTRC20ReceivingAddress',
        value: sanitized.USDTTRC20ReceivingAddress,
      })
    }
    if (
      shouldUpdateCryptoPaymentCredential(
        sanitized.USDTTRC20APIKey,
        initial.USDTTRC20APIKey
      )
    ) {
      updates.push({ key: 'USDTTRC20APIKey', value: sanitized.USDTTRC20APIKey })
    }
    if (sanitized.USDTTONReceivingAddress !== initial.USDTTONReceivingAddress) {
      updates.push({
        key: 'USDTTONReceivingAddress',
        value: sanitized.USDTTONReceivingAddress,
      })
    }
    if (
      sanitized.USDTSolanaReceivingAddress !==
      initial.USDTSolanaReceivingAddress
    ) {
      updates.push({
        key: 'USDTSolanaReceivingAddress',
        value: sanitized.USDTSolanaReceivingAddress,
      })
    }
    if (
      sanitized.USDTSolanaReceivingTokenAccount !==
      initial.USDTSolanaReceivingTokenAccount
    ) {
      updates.push({
        key: 'USDTSolanaReceivingTokenAccount',
        value: sanitized.USDTSolanaReceivingTokenAccount,
      })
    }
    if (
      sanitized.USDTTRC20AmountTailLimitUnits !==
      decimalUsdtToMicroUnits(initial.USDTTRC20AmountTailLimitUnits)
    ) {
      updates.push({
        key: 'USDTTRC20AmountTailLimitUnits',
        value: sanitized.USDTTRC20AmountTailLimitUnits,
      })
    }
    if (sanitized.WaffoEnabled !== initial.WaffoEnabled) {
      updates.push({ key: 'WaffoEnabled', value: sanitized.WaffoEnabled })
    }

    if (sanitized.WaffoSandbox !== initial.WaffoSandbox) {
      updates.push({ key: 'WaffoSandbox', value: sanitized.WaffoSandbox })
    }

    if (sanitized.WaffoMerchantId !== initial.WaffoMerchantId) {
      updates.push({
        key: 'WaffoMerchantId',
        value: sanitized.WaffoMerchantId,
      })
    }

    if (sanitized.WaffoCurrency !== initial.WaffoCurrency) {
      updates.push({ key: 'WaffoCurrency', value: sanitized.WaffoCurrency })
    }

    if (sanitized.WaffoUnitPrice !== initial.WaffoUnitPrice) {
      updates.push({ key: 'WaffoUnitPrice', value: sanitized.WaffoUnitPrice })
    }

    if (sanitized.WaffoMinTopUp !== initial.WaffoMinTopUp) {
      updates.push({ key: 'WaffoMinTopUp', value: sanitized.WaffoMinTopUp })
    }

    if (sanitized.WaffoNotifyUrl !== initial.WaffoNotifyUrl) {
      updates.push({ key: 'WaffoNotifyUrl', value: sanitized.WaffoNotifyUrl })
    }

    if (sanitized.WaffoReturnUrl !== initial.WaffoReturnUrl) {
      updates.push({ key: 'WaffoReturnUrl', value: sanitized.WaffoReturnUrl })
    }

    if (sanitized.WaffoPublicCert !== initial.WaffoPublicCert) {
      updates.push({
        key: 'WaffoPublicCert',
        value: sanitized.WaffoPublicCert,
      })
    }

    if (sanitized.WaffoSandboxPublicCert !== initial.WaffoSandboxPublicCert) {
      updates.push({
        key: 'WaffoSandboxPublicCert',
        value: sanitized.WaffoSandboxPublicCert,
      })
    }

    if (sanitized.WaffoApiKey) {
      updates.push({ key: 'WaffoApiKey', value: sanitized.WaffoApiKey })
    }

    if (sanitized.WaffoPrivateKey) {
      updates.push({
        key: 'WaffoPrivateKey',
        value: sanitized.WaffoPrivateKey,
      })
    }

    if (sanitized.WaffoSandboxApiKey) {
      updates.push({
        key: 'WaffoSandboxApiKey',
        value: sanitized.WaffoSandboxApiKey,
      })
    }

    if (sanitized.WaffoSandboxPrivateKey) {
      updates.push({
        key: 'WaffoSandboxPrivateKey',
        value: sanitized.WaffoSandboxPrivateKey,
      })
    }

    if (
      normalizeJsonForComparison(sanitized.WaffoPayMethods) !==
      normalizeJsonForComparison(initial.WaffoPayMethods)
    ) {
      updates.push({
        key: 'WaffoPayMethods',
        value: sanitized.WaffoPayMethods,
      })
    }

    const hasWaffoPancakeChanges =
      sanitized.WaffoPancakeMerchantID !== initial.WaffoPancakeMerchantID ||
      sanitized.WaffoPancakePrivateKey.length > 0 ||
      sanitized.WaffoPancakeReturnURL !== initial.WaffoPancakeReturnURL ||
      waffoPancakeSelection.storeID !== waffoPancakeSavedBinding.storeID ||
      waffoPancakeSelection.productID !== waffoPancakeSavedBinding.productID

    if (sanitized.WaffoPancakeMinTopUp !== initial.WaffoPancakeMinTopUp) {
      updates.push({
        key: 'WaffoPancakeMinTopUp',
        value: sanitized.WaffoPancakeMinTopUp,
      })
    }

    if (updates.length === 0 && !hasCreemChanges && !hasWaffoPancakeChanges) {
      toast.info(t('No changes to save'))
      return
    }

    try {
      await savePaymentSettings(
        buildPaymentSettingsPayload(
          updates,
          hasCreemChanges ? creemUpdate : undefined
        )
      )
    } catch (error) {
      toast.error(
        getPaymentSettingsSaveErrorMessage(
          error,
          t('Failed to save payment settings')
        )
      )
      return
    }
    queryClient.invalidateQueries({ queryKey: ['system-options'] })
    toast.success(t('Setting updated successfully'))

    if (!hasWaffoPancakeChanges) {
      return
    }

    if (!sanitized.WaffoPancakeMerchantID) {
      toast.error(t('Merchant ID is required'))
      return
    }

    if (!waffoPancakeSelection.storeID || !waffoPancakeSelection.productID) {
      toast.error(t('Pick or create both a store and a product before saving.'))
      return
    }

    try {
      const body = await saveWaffoPancakeConfig({
        merchantID: sanitized.WaffoPancakeMerchantID,
        privateKey: sanitized.WaffoPancakePrivateKey,
        returnURL: sanitized.WaffoPancakeReturnURL,
        storeID: waffoPancakeSelection.storeID,
        productID: waffoPancakeSelection.productID,
      })

      if (
        body?.message === 'success' &&
        typeof body.data === 'object' &&
        body.data
      ) {
        const saved = body.data as { product_id: string; store_id: string }
        const savedBinding = {
          storeID: saved.store_id,
          productID: saved.product_id,
        }
        setWaffoPancakeSavedBinding(savedBinding)
        setWaffoPancakeSelection(savedBinding)
        queryClient.invalidateQueries({ queryKey: ['system-options'] })
        toast.success(t('Waffo Pancake settings saved'))
        return
      }

      const reason = typeof body?.data === 'string' ? body.data : undefined
      toast.error(
        reason
          ? `${t('Waffo Pancake save failed')}: ${reason}`
          : t('Waffo Pancake save failed')
      )
    } catch (error) {
      toast.error(
        `${t('Waffo Pancake save failed')}: ${
          error instanceof Error ? error.message : String(error)
        }`
      )
    }
  }

  const currentFormValues = form.watch()
  const waffoValues: WaffoSettingsValues = {
    WaffoEnabled: currentFormValues.WaffoEnabled,
    WaffoApiKey: currentFormValues.WaffoApiKey,
    WaffoPrivateKey: currentFormValues.WaffoPrivateKey,
    WaffoPublicCert: currentFormValues.WaffoPublicCert,
    WaffoSandboxPublicCert: currentFormValues.WaffoSandboxPublicCert,
    WaffoSandboxApiKey: currentFormValues.WaffoSandboxApiKey,
    WaffoSandboxPrivateKey: currentFormValues.WaffoSandboxPrivateKey,
    WaffoSandbox: currentFormValues.WaffoSandbox,
    WaffoMerchantId: currentFormValues.WaffoMerchantId,
    WaffoCurrency: currentFormValues.WaffoCurrency,
    WaffoUnitPrice: currentFormValues.WaffoUnitPrice,
    WaffoMinTopUp: currentFormValues.WaffoMinTopUp,
    WaffoNotifyUrl: currentFormValues.WaffoNotifyUrl,
    WaffoReturnUrl: currentFormValues.WaffoReturnUrl,
    WaffoPayMethods: JSON.stringify(waffoPayMethods),
  }
  const waffoPancakeValues: WaffoPancakeSettingsValues = {
    WaffoPancakeMerchantID: currentFormValues.WaffoPancakeMerchantID,
    WaffoPancakePrivateKey: currentFormValues.WaffoPancakePrivateKey,
    WaffoPancakeReturnURL: currentFormValues.WaffoPancakeReturnURL,
    WaffoPancakeMinTopUp: currentFormValues.WaffoPancakeMinTopUp,
  }

  return (
    <SettingsSection title={t('Payment Gateway')}>
      <Form {...form}>
        <SettingsForm
          onSubmit={form.handleSubmit(onSubmit)}
          className='gap-y-8'
          data-no-autosubmit='true'
        >
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={isSubmitting}
            saveLabel='Save all settings'
          />
          <Tabs defaultValue='general' className='min-w-0'>
            <div className='overflow-x-auto pb-1'>
              <TabsList className='grid min-w-[68rem] grid-cols-9'>
                <TabsTrigger value='general'>{t('General')}</TabsTrigger>
                <TabsTrigger value='epay'>Epay</TabsTrigger>
                <TabsTrigger value='yookassa'>YooKassa</TabsTrigger>
                <TabsTrigger value='nowpayments'>NOWPayments</TabsTrigger>
                <TabsTrigger value='crypto'>{t('Crypto')}</TabsTrigger>
                <TabsTrigger value='stripe'>{t('Stripe')}</TabsTrigger>
                <TabsTrigger value='creem'>Creem</TabsTrigger>
                <TabsTrigger value='waffo-pancake'>Waffo Pancake</TabsTrigger>
                <TabsTrigger value='waffo'>Waffo</TabsTrigger>
              </TabsList>
            </div>

            <TabsContent value='general' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>
                    {t('General Settings')}
                  </h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Shared configuration for all payment gateways')}
                  </p>
                </div>

                <FormField
                  control={form.control}
                  name='PayMethods'
                  render={({ field }) => (
                    <FormItem>
                      <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                        <FormLabel>{t('Payment methods')}</FormLabel>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() =>
                            setPayMethodsVisualMode(!payMethodsVisualMode)
                          }
                          className='w-full sm:w-auto'
                        >
                          {payMethodsVisualMode ? (
                            <>
                              <Code2 className='mr-2 h-3 w-3' />
                              {t('JSON Editor')}
                            </>
                          ) : (
                            <>
                              <Eye className='mr-2 h-3 w-3' />
                              {t('Visual Editor')}
                            </>
                          )}
                        </Button>
                      </div>
                      <FormControl>
                        {payMethodsVisualMode ? (
                          <PaymentMethodsVisualEditor
                            value={field.value}
                            onChange={field.onChange}
                            topupGroups={topupGroups}
                            waffoCurrency={waffoCurrency}
                            defaultPendingTtlMinutes={
                              currentFormValues.PaymentPendingTTLMinutes
                            }
                            availableIcons={availablePaymentIcons}
                            onAvailableIconsChange={(value) =>
                              setPaymentValue(
                                'PaymentMethodAvailableIcons',
                                value
                              )
                            }
                          />
                        ) : (
                          <Textarea
                            rows={4}
                            placeholder={t(
                              '[{"name":"支付宝","type":"alipay","icon":"SiAlipay"}]'
                            )}
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        )}
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Configured as PayMethods JSON. The type value decides which payment flow is used: stripe for Stripe, waffo_pancake for Waffo Pancake, and other values are sent to Epay as the type parameter.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />

                <div className='grid gap-6 md:grid-cols-2 md:items-start'>
                  <FormField
                    control={form.control}
                    name='AmountOptions'
                    render={({ field }) => (
                      <FormItem>
                        <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                          <FormLabel>{t('Top-up amount options')}</FormLabel>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              setAmountOptionsVisualMode(
                                !amountOptionsVisualMode
                              )
                            }
                            className='w-full sm:w-auto'
                          >
                            {amountOptionsVisualMode ? (
                              <>
                                <Code2 className='mr-2 h-3 w-3' />
                                {t('JSON Editor')}
                              </>
                            ) : (
                              <>
                                <Eye className='mr-2 h-3 w-3' />
                                {t('Visual Editor')}
                              </>
                            )}
                          </Button>
                        </div>
                        <FormControl>
                          {amountOptionsVisualMode ? (
                            <AmountOptionsVisualEditor
                              value={field.value}
                              onChange={field.onChange}
                            />
                          ) : (
                            <Textarea
                              rows={4}
                              placeholder='[10, 20, 50, 100]'
                              {...field}
                              onChange={(event) =>
                                field.onChange(event.target.value)
                              }
                            />
                          )}
                        </FormControl>
                        <FormDescription>
                          {t('Preset recharge amounts (JSON array)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='AmountCashback'
                    render={({ field }) => (
                      <FormItem>
                        <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                          <FormLabel>{t('Amount cashback')}</FormLabel>
                          <Button
                            type='button'
                            variant='outline'
                            size='sm'
                            onClick={() =>
                              setAmountCashbackVisualMode(
                                !amountCashbackVisualMode
                              )
                            }
                            className='w-full sm:w-auto'
                          >
                            {amountCashbackVisualMode ? (
                              <>
                                <Code2 className='mr-2 h-3 w-3' />
                                {t('JSON Editor')}
                              </>
                            ) : (
                              <>
                                <Eye className='mr-2 h-3 w-3' />
                                {t('Visual Editor')}
                              </>
                            )}
                          </Button>
                        </div>
                        <FormControl>
                          {amountCashbackVisualMode ? (
                            <AmountCashbackVisualEditor
                              value={field.value}
                              onChange={field.onChange}
                              tokenAmounts={tokenAmounts}
                            />
                          ) : (
                            <Textarea
                              rows={4}
                              placeholder='[{"min_amount":100,"cashback_percent":1}]'
                              {...field}
                              onChange={(event) =>
                                field.onChange(event.target.value)
                              }
                            />
                          )}
                        </FormControl>
                        <FormDescription>
                          {t(
                            tokenAmounts
                              ? 'Cashback thresholds use token amounts; min_amount is the minimum token recharge amount'
                              : 'Cashback thresholds use USD amounts; min_amount is the minimum USD recharge amount'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent value='epay' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>{t('Epay Gateway')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Configuration for Epay payment integration')}
                  </p>
                </div>

                <PaymentCurrencyField value='USD' fixedCurrency='USD' />

                <Alert>
                  <ShieldAlert className='h-4 w-4' />
                  <AlertTitle>{t('Epay safety reminder')}</AlertTitle>
                  <AlertDescription>
                    {t(
                      'Epay is a payment protocol, not a specific official website. Verify the provider yourself and do not trust random third-party Epay deployments.'
                    )}
                  </AlertDescription>
                </Alert>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='PayAddress'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Epay endpoint')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('https://pay.example.com')}
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Base address provided by your Epay service')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='CustomCallbackAddress'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Callback address')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('https://gateway.example.com')}
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Only enter the site origin, for example https://api.example.com. Do not include any path such as /api/user/epay/notify. Leave blank to use the server address.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='EpayId'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Epay merchant ID')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder='10001'
                            autoComplete='off'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='EpayKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Epay secret key')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={t('Enter new key to update')}
                            autoComplete='new-password'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Leave blank unless rotating the secret')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent
              value='yookassa'
              className={paymentTabContentClassName}
            >
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>
                    {t('YooKassa Gateway')}
                  </h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Configuration for YooKassa payment integration')}
                  </p>
                </div>

                <PaymentCurrencyField value='RUB' fixedCurrency='RUB' />

                <div className='rounded-md bg-purple-50 p-4 text-sm text-purple-900 dark:bg-purple-950 dark:text-purple-100'>
                  <p className='mb-2 font-medium'>
                    {t('Webhook Configuration:')}
                  </p>
                  <ul className='list-inside list-disc space-y-1'>
                    <li>
                      {t('Webhook URL:')}{' '}
                      <code className='rounded bg-purple-100 px-1 py-0.5 text-xs dark:bg-purple-900'>
                        {'<ServerAddress>/api/user/yookassa/notify'}
                      </code>
                    </li>
                    <li>{t('Configure in your YooKassa dashboard')}</li>
                  </ul>
                </div>

                <FormField
                  control={form.control}
                  name='YooKassaEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Enable YooKassa payments')}</FormLabel>
                        <FormDescription>
                          {t('Show SBP / YooKassa in recharge options')}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='YooKassaShopID'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Shop ID')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('YooKassa shop ID')}
                            autoComplete='off'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='YooKassaSecretKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Secret key')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={t(
                              'YooKassa secret key (leave blank unless updating)'
                            )}
                            autoComplete='new-password'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Masked value means the saved key is unchanged')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='YooKassaReturnURL'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Payment return URL')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder='https://example.com'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Leave blank to use the public server address')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent
              value='nowpayments'
              className={paymentTabContentClassName}
            >
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>NOWPayments</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Accept cryptocurrency payments through NOWPayments')}
                  </p>
                </div>

                <PaymentCurrencyField value='USDT' fixedCurrency='USDT' />

                <div className='rounded-md bg-orange-50 p-4 text-sm text-orange-900 dark:bg-orange-950 dark:text-orange-100'>
                  <p className='mb-2 font-medium'>
                    {t('Webhook Configuration:')}
                  </p>
                  <p>
                    {t(
                      'Set this IPN callback URL in the NOWPayments dashboard:'
                    )}{' '}
                    <code className='rounded bg-orange-100 px-1 py-0.5 text-xs dark:bg-orange-900'>
                      {form.watch('NOWPaymentsIPNCallbackURL') ||
                        '<ServerAddress>/api/nowpayments/webhook'}
                    </code>
                  </p>
                </div>

                <FormField
                  control={form.control}
                  name='NOWPaymentsEnabled'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>
                          {t('Enable NOWPayments payments')}
                        </FormLabel>
                        <FormDescription>
                          {t('Show Crypto / NOWPayments in recharge options')}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='NOWPaymentsAPIKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('API key')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            autoComplete='new-password'
                            placeholder={t('Leave blank unless updating')}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Masked value means the saved key is unchanged')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='NOWPaymentsIPNSecret'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('IPN secret')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            autoComplete='new-password'
                            placeholder={t('Leave blank unless updating')}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Used to verify payment webhooks')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='NOWPaymentsIPNCallbackURL'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('IPN callback URL')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder='https://example.com/api/nowpayments/webhook'
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Crypto payments are enabled when API key, IPN secret, and this URL are set.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </TabsContent>

            <TabsContent value='crypto' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>{t('Crypto')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Direct USDT payments on supported networks')}
                  </p>
                </div>
                <div className='grid gap-6 md:grid-cols-2'>
                  <PaymentCurrencyField
                    value={CRYPTO_PAYMENT_CURRENCY}
                    fixedCurrency={CRYPTO_PAYMENT_CURRENCY}
                  />
                  <FormField
                    control={form.control}
                    name='USDTTRC20AmountTailLimitUnits'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Amount precision')}</FormLabel>
                        <FormControl>
                          <Input
                            type='text'
                            inputMode='decimal'
                            placeholder='0.001'
                            maxLength={8}
                            {...field}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Enter the precision for the random payment tail in USDT.'
                          )}{' '}
                          {t('USDT minimum selectable step: 0.000002.')}{' '}
                          {t('Available variants: {{count}}', {
                            count: cryptoAmountTailVariants(field.value) ?? '—',
                          })}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>
                <FormField
                  control={form.control}
                  name='USDTTRC20ReceivingAddress'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('USDT TRON receiving address')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Supported for checkout. The address is validated before saving.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='USDTTRC20APIKey'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('TronGrid API key')}</FormLabel>
                      <FormControl>
                        <Input
                          type='password'
                          autoComplete='new-password'
                          placeholder={t('Leave blank unless updating')}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Required to monitor USDT TRC-20 transfers. This is not a wallet private key.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='USDTTONReceivingAddress'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('USDT TON receiving address')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Used for USDT TON checkout. The address is validated before saving.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='USDTSolanaReceivingAddress'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('USDT Solana receiving address')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Wallet owner address for Solana USDT. The token account below is the exact payment destination.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
                <FormField
                  control={form.control}
                  name='USDTSolanaReceivingTokenAccount'
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>{t('USDT Solana receiving token account')}</FormLabel>
                      <FormControl>
                        <Input {...field} />
                      </FormControl>
                      <FormDescription>
                        {t(
                          'Exact SPL token account where users must send USDT. The address is validated before saving.'
                        )}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </TabsContent>

            <TabsContent value='stripe' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>{t('Stripe Gateway')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Configuration for Stripe payment integration')}
                  </p>
                </div>

                <PaymentCurrencyField value='USD' fixedCurrency='USD' />

                <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
                  <p className='mb-2 font-medium'>
                    {t('Webhook Configuration:')}
                  </p>
                  <ul className='list-inside list-disc space-y-1'>
                    <li>
                      {t('Webhook URL:')}{' '}
                      <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                        {'<ServerAddress>/api/stripe/webhook'}
                      </code>
                    </li>
                    <li>
                      {t('Required events:')}{' '}
                      <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                        {t('checkout.session.completed')}
                      </code>{' '}
                      {t('and')}{' '}
                      <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                        {t('checkout.session.expired')}
                      </code>
                    </li>
                    <li>
                      {t('Configure at:')}{' '}
                      <a
                        href='https://dashboard.stripe.com/developers'
                        target='_blank'
                        rel='noreferrer'
                        className='underline hover:no-underline'
                      >
                        {t('Stripe Dashboard')}
                      </a>
                    </li>
                  </ul>
                </div>

                <div className='grid gap-6 md:grid-cols-3'>
                  <FormField
                    control={form.control}
                    name='StripeApiSecret'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('API secret')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={t('sk_xxx or rk_xxx')}
                            autoComplete='new-password'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Stripe API key (leave blank unless updating)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='StripeWebhookSecret'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Webhook secret')}</FormLabel>
                        <FormControl>
                          <Input
                            type='password'
                            placeholder={t('whsec_xxx')}
                            autoComplete='new-password'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Webhook signing secret (leave blank unless updating)'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='StripePriceId'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Price ID')}</FormLabel>
                        <FormControl>
                          <Input
                            placeholder={t('price_xxx')}
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Stripe product price ID')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <div className='grid gap-6 md:grid-cols-3'>
                  <FormField
                    control={form.control}
                    name='StripeUnitPrice'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Unit price (USD)')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            step='0.01'
                            min={0}
                            {...safeNumberFieldProps(field)}
                          />
                        </FormControl>
                        <FormDescription>
                          {t(
                            'The provider will charge in this currency using its configured USD rate.'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='StripeMinTopUp'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Minimum top-up (USD)')}</FormLabel>
                        <FormControl>
                          <Input
                            type='number'
                            step='0.01'
                            min={0}
                            {...safeNumberFieldProps(field)}
                          />
                        </FormControl>
                        <FormDescription>
                          {t('Minimum recharge amount in USD')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='StripePromotionCodesEnabled'
                    render={({ field }) => (
                      <SettingsSwitchItem>
                        <SettingsSwitchContent>
                          <FormLabel>{t('Promotion codes')}</FormLabel>
                          <FormDescription>
                            {t('Allow users to enter promo codes')}
                          </FormDescription>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                          />
                        </FormControl>
                      </SettingsSwitchItem>
                    )}
                  />
                </div>
              </div>
            </TabsContent>

            <TabsContent value='creem' className={paymentTabContentClassName}>
              <div className='space-y-4'>
                <div>
                  <h3 className='text-lg font-medium'>{t('Creem Gateway')}</h3>
                  <p className='text-muted-foreground text-sm'>
                    {t('Configuration for Creem payment integration')}
                  </p>
                </div>

                <div className='rounded-md bg-blue-50 p-4 text-sm text-blue-900 dark:bg-blue-950 dark:text-blue-100'>
                  <p className='mb-2 font-medium'>
                    {t('Webhook Configuration:')}
                  </p>
                  <ul className='list-inside list-disc space-y-1'>
                    <li>
                      {t('Webhook URL:')}{' '}
                      <code className='rounded bg-blue-100 px-1 py-0.5 text-xs dark:bg-blue-900'>
                        {'<ServerAddress>/api/creem/webhook'}
                      </code>
                    </li>
                    <li>{t('Configure in your Creem dashboard')}</li>
                  </ul>
                </div>

                <div className='grid gap-6 md:grid-cols-2'>
                  <FormField
                    control={form.control}
                    name='CreemApiKey'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('API Key')}</FormLabel>
                        <FormControl>
                          <div className='flex gap-2'>
                            <Input
                              type='password'
                              placeholder={t('Enter Creem API key')}
                              autoComplete='new-password'
                              {...field}
                              onChange={(event) => {
                                setCreemSecretClearRequested((previous) => ({
                                  ...previous,
                                  apiKey: false,
                                }))
                                field.onChange(event.target.value)
                              }}
                            />
                            <Button
                              type='button'
                              variant='outline'
                              onClick={() => {
                                setCreemSecretClearRequested((previous) => ({
                                  ...previous,
                                  apiKey: true,
                                }))
                                field.onChange('')
                              }}
                            >
                              {t('Clear')}
                            </Button>
                          </div>
                        </FormControl>
                        <FormDescription>
                          {t('Creem API key (leave blank unless updating)')}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />

                  <FormField
                    control={form.control}
                    name='CreemWebhookSecret'
                    render={({ field }) => (
                      <FormItem>
                        <FormLabel>{t('Webhook Secret')}</FormLabel>
                        <FormControl>
                          <div className='flex gap-2'>
                            <Input
                              type='password'
                              placeholder={t('Enter webhook secret')}
                              autoComplete='new-password'
                              {...field}
                              onChange={(event) => {
                                setCreemSecretClearRequested((previous) => ({
                                  ...previous,
                                  webhookSecret: false,
                                }))
                                field.onChange(event.target.value)
                              }}
                            />
                            <Button
                              type='button'
                              variant='outline'
                              onClick={() => {
                                setCreemSecretClearRequested((previous) => ({
                                  ...previous,
                                  webhookSecret: true,
                                }))
                                field.onChange('')
                              }}
                            >
                              {t('Clear')}
                            </Button>
                          </div>
                        </FormControl>
                        <FormDescription>
                          {t(
                            'Webhook signing secret (leave blank unless updating)'
                          )}
                        </FormDescription>
                        <FormMessage />
                      </FormItem>
                    )}
                  />
                </div>

                <FormField
                  control={form.control}
                  name='CreemTestMode'
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{t('Test Mode')}</FormLabel>
                        <FormDescription>
                          {t('Enable test mode for Creem payments')}
                        </FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                    </SettingsSwitchItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name='CreemProducts'
                  render={({ field }) => (
                    <FormItem>
                      <div className='mb-2 flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
                        <FormLabel>{t('Products')}</FormLabel>
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() =>
                            setCreemProductsVisualMode(!creemProductsVisualMode)
                          }
                          className='w-full sm:w-auto'
                        >
                          {creemProductsVisualMode ? (
                            <>
                              <Code2 className='mr-2 h-3 w-3' />
                              {t('JSON Editor')}
                            </>
                          ) : (
                            <>
                              <Eye className='mr-2 h-3 w-3' />
                              {t('Visual Editor')}
                            </>
                          )}
                        </Button>
                      </div>
                      <FormControl>
                        {creemProductsVisualMode ? (
                          <CreemProductsVisualEditor
                            value={field.value}
                            onChange={field.onChange}
                          />
                        ) : (
                          <Textarea
                            rows={4}
                            placeholder='[{"name":"Basic","productId":"prod_xxx","price":10,"quota":500000,"currency":"USD"}]'
                            {...field}
                            onChange={(event) =>
                              field.onChange(event.target.value)
                            }
                          />
                        )}
                      </FormControl>
                      <FormDescription>
                        {t('Configure Creem products. Provide a JSON array.')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              </div>
            </TabsContent>

            <TabsContent
              value='waffo-pancake'
              className={paymentTabContentClassName}
            >
              <PaymentCurrencyField value='USD' fixedCurrency='USD' />
              <WaffoPancakeSettingsSection
                defaultValues={waffoPancakeDefaultValues}
                values={waffoPancakeValues}
                onValueChange={setWaffoPancakeValue}
                selectedBinding={waffoPancakeSelection}
                savedBinding={waffoPancakeSavedBinding}
                onSelectedBindingChange={setWaffoPancakeSelection}
              />
            </TabsContent>

            <TabsContent value='waffo' className={paymentTabContentClassName}>
              <WaffoSettingsSection
                values={waffoValues}
                onValueChange={setWaffoValue}
                payMethods={waffoPayMethods}
                onPayMethodsChange={setWaffoPayMethods}
              />
            </TabsContent>
          </Tabs>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}

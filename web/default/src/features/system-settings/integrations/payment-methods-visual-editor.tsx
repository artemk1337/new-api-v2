import { Lightbulb, Pencil, Plus, Search, Trash2 } from 'lucide-react'
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
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { ReactIconByName } from '@/components/react-icon-by-name'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'

import { safeJsonParseWithValidation } from '../utils/json-parser'
import { isArray } from '../utils/json-validators'
import {
  PaymentMethodDialog,
  type PaymentMethodData,
  type TopupGroupOption,
} from './payment-method-dialog'
import {
  BUILT_IN_PAYMENT_ICONS,
  serializeAvailablePaymentIcons,
} from './payment-method-icons'
import { getPaymentMethodMinimumForDisplay } from './payment-method-minimum'
import {
  CRYPTO_PAYMENT_TYPE,
  getPaymentTypeOptions,
  normalizePaymentMethodType,
} from './payment-method-options'

type PaymentMethodsVisualEditorProps = {
  value: string
  onChange: (value: string) => void
  topupGroups: TopupGroupOption[]
  waffoCurrency?: string
  defaultPendingTtlMinutes?: number
  availableIcons?: string[]
  onAvailableIconsChange?: (value: string) => void
}

const PAYMENT_TYPE_ICON_NAMES: Record<string, string> = {
  alipay: 'SiAlipay',
  crypto_direct: 'LuWalletCards',
  nowpayments: 'LuBitcoin',
  stripe: 'SiStripe',
  waffo: 'LuCreditCard',
  waffo_pancake: 'LuCreditCard',
  wxpay: 'SiWechat',
}

function getPaymentMethodDisplayName(method: PaymentMethodData) {
  return normalizePaymentMethodType(method.type) === CRYPTO_PAYMENT_TYPE
    ? 'Crypto'
    : method.name
}

function getPaymentTypeDisplayName(
  method: PaymentMethodData,
  t: (key: string) => string
) {
  const builtIn = getPaymentTypeOptions(t).find(
    (option) => option.value === normalizePaymentMethodType(method.type)
  )
  return builtIn?.name ?? getPaymentMethodDisplayName(method)
}

function getDefaultIconName(type: string) {
  return PAYMENT_TYPE_ICON_NAMES[type] ?? ''
}

function getEffectiveIconName(method: PaymentMethodData) {
  return (
    method.icon || getDefaultIconName(normalizePaymentMethodType(method.type))
  )
}

function normalizePaymentMethodData(
  method: PaymentMethodData
): PaymentMethodData {
  const type = normalizePaymentMethodType(method.type)
  return {
    ...method,
    type,
    name: type === CRYPTO_PAYMENT_TYPE ? 'Crypto' : method.name,
  }
}

export function getPaymentMethodPendingTtl(
  method: PaymentMethodData,
  defaultPendingTtlMinutes = 1440
) {
  const configured = Number(method.pending_ttl_minutes)
  if (Number.isInteger(configured) && configured > 0) return configured
  return method.type === 'yookassa_sbp' ? 15 : defaultPendingTtlMinutes
}

export function isPaymentMethodData(item: unknown): item is PaymentMethodData {
  return (
    typeof item === 'object' &&
    item !== null &&
    'name' in item &&
    'type' in item &&
    typeof item.name === 'string' &&
    typeof item.type === 'string' &&
    (!('icon' in item) || typeof item.icon === 'string') &&
    (!('min_topup' in item) || typeof item.min_topup === 'string') &&
    (!('pending_ttl_minutes' in item) ||
      typeof item.pending_ttl_minutes === 'string') &&
    (!('color' in item) || typeof item.color === 'string') &&
    (!('topup_group' in item) || typeof item.topup_group === 'string') &&
    (!('currency' in item) || typeof item.currency === 'string') &&
    (!('admin_only' in item) ||
      item.admin_only === undefined ||
      typeof item.admin_only === 'boolean' ||
      item.admin_only === 'true' ||
      item.admin_only === 'false') &&
    (!('AdminOnly' in item) ||
      item.AdminOnly === undefined ||
      typeof item.AdminOnly === 'boolean' ||
      item.AdminOnly === 'true' ||
      item.AdminOnly === 'false')
  )
}

export function PaymentMethodsVisualEditor({
  value,
  onChange,
  topupGroups,
  waffoCurrency,
  defaultPendingTtlMinutes = 1440,
  availableIcons = [...BUILT_IN_PAYMENT_ICONS],
  onAvailableIconsChange,
}: PaymentMethodsVisualEditorProps) {
  const { t } = useTranslation()
  const paymentTemplates = [
    {
      name: 'СБП / YooKassa',
      template: {
        icon: 'LuCreditCard',
        name: 'СБП / YooKassa',
        type: 'yookassa_sbp',
      },
    },
    {
      name: t('Epay Alipay'),
      template: {
        icon: getDefaultIconName('alipay'),
        name: '支付宝',
        type: 'alipay',
      },
    },
    {
      name: t('Epay WeChat Pay'),
      template: {
        icon: getDefaultIconName('wxpay'),
        name: '微信',
        type: 'wxpay',
      },
    },
    {
      name: t('Stripe'),
      template: {
        icon: getDefaultIconName('stripe'),
        min_topup: '10',
        name: 'Stripe',
        type: 'stripe',
      },
    },
    {
      name: 'Waffo Pancake',
      template: {
        icon: getDefaultIconName('waffo_pancake'),
        name: 'Waffo Pancake',
        type: 'waffo_pancake',
      },
    },
    {
      name: t('Waffo'),
      template: {
        icon: 'LuCreditCard',
        name: t('Waffo'),
        type: 'waffo',
      },
    },
    {
      name: 'NOWPayments',
      template: {
        icon: 'LuBitcoin',
        name: 'NOWPayments',
        type: 'nowpayments',
      },
    },
    {
      name: t('Crypto'),
      template: {
        icon: 'LuWalletCards',
        name: 'Crypto',
        type: CRYPTO_PAYMENT_TYPE,
      },
    },
  ]
  const [searchText, setSearchText] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<PaymentMethodData | null>(null)

  const paymentMethods = useMemo(() => {
    const parsed = safeJsonParseWithValidation<unknown[]>(value, {
      fallback: [],
      validator: isArray,
      validatorMessage: 'Payment methods must be a JSON array',
      context: 'payment methods',
    })

    const seenTypes = new Set<string>()
    return parsed
      .filter(isPaymentMethodData)
      .map((method) => ({
        ...method,
        type: normalizePaymentMethodType(method.type),
        name:
          normalizePaymentMethodType(method.type) === CRYPTO_PAYMENT_TYPE
            ? 'Crypto'
            : method.name,
      }))
      .filter((method) => {
        if (method.type !== CRYPTO_PAYMENT_TYPE) return true
        if (seenTypes.has(method.type)) return false
        seenTypes.add(method.type)
        return true
      })
  }, [value])

  const filteredMethods = useMemo(() => {
    if (!searchText) return paymentMethods
    const lowerSearch = searchText.toLowerCase()
    return paymentMethods.filter(
      (method) =>
        method.name.toLowerCase().includes(lowerSearch) ||
        method.type.toLowerCase().includes(lowerSearch) ||
        getEffectiveIconName(method).toLowerCase().includes(lowerSearch)
    )
  }, [paymentMethods, searchText])

  const handleSave = (data: PaymentMethodData) => {
    const parsed = safeJsonParseWithValidation<unknown[]>(value, {
      fallback: [],
      validator: isArray,
      silent: true,
    })

    const updatedArray = parsed.map((item) => {
      if (!isPaymentMethodData(item)) return item
      const { currency: _legacyCurrency, ...metadata } =
        item as PaymentMethodData & {
          currency?: string
        }
      return normalizePaymentMethodData(metadata)
    })

    // Legacy configurations may contain one entry per network. Keep one
    // canonical Crypto entry when saving any change from the editor.
    const dedupedArray = updatedArray.filter((item, index, array) => {
      if (!isPaymentMethodData(item)) return true
      return (
        item.type !== CRYPTO_PAYMENT_TYPE ||
        array.findIndex(
          (candidate) =>
            isPaymentMethodData(candidate) &&
            candidate.type === CRYPTO_PAYMENT_TYPE
        ) === index
      )
    })

    if (editData) {
      const normalizedEditType = normalizePaymentMethodType(editData.type)
      const index = dedupedArray.findIndex(
        (item): item is PaymentMethodData =>
          typeof item === 'object' &&
          item !== null &&
          'name' in item &&
          'type' in item &&
          (item.name === editData.name ||
            normalizedEditType === CRYPTO_PAYMENT_TYPE) &&
          normalizePaymentMethodType(String(item.type)) === normalizedEditType
      )
      if (index !== -1) {
        dedupedArray[index] = normalizePaymentMethodData(data)
      } else {
        dedupedArray.push(normalizePaymentMethodData(data))
      }
    } else {
      dedupedArray.push(normalizePaymentMethodData(data))
    }

    onChange(JSON.stringify(dedupedArray, null, 2))
  }

  const handleDelete = (method: PaymentMethodData) => {
    const parsed = safeJsonParseWithValidation<unknown[]>(value, {
      fallback: [],
      validator: isArray,
      silent: true,
    })

    const updatedArray = parsed.filter(
      (item) =>
        !(
          typeof item === 'object' &&
          item !== null &&
          'name' in item &&
          'type' in item &&
          (item.name === method.name ||
            normalizePaymentMethodType(method.type) === CRYPTO_PAYMENT_TYPE) &&
          normalizePaymentMethodType(String(item.type)) ===
            normalizePaymentMethodType(method.type)
        )
    )

    onChange(JSON.stringify(updatedArray, null, 2))
  }

  const handleEdit = (method: PaymentMethodData) => {
    setEditData(method)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  const handleInsertTemplate = (template: PaymentMethodData) => {
    const configuredGroups = topupGroups.filter((group) => group.name.trim())
    const defaultGroup =
      configuredGroups.find(
        (group) => group.name.trim().toLowerCase() === 'default'
      ) ?? configuredGroups[0]
    if (!defaultGroup) return

    const parsed = safeJsonParseWithValidation<unknown[]>(value, {
      fallback: [],
      validator: isArray,
      silent: true,
    })

    // Check if template already exists
    const normalizedTemplate = normalizePaymentMethodData({
      ...template,
      topup_group: defaultGroup.name.trim(),
    })
    const exists = parsed.some(
      (item) =>
        typeof item === 'object' &&
        item !== null &&
        'type' in item &&
        'name' in item &&
        normalizePaymentMethodType(String(item.type)) ===
          normalizedTemplate.type
    )

    if (!exists) {
      parsed.push(normalizedTemplate)
      onChange(JSON.stringify(parsed, null, 2))
    }
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center'>
        <div className='relative flex-1'>
          <Search className='text-muted-foreground absolute top-2.5 left-2.5 h-4 w-4' />
          <Input
            placeholder={t('Search payment methods...')}
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            className='pl-9'
          />
        </div>
        <div className='flex gap-2'>
          <Popover>
            <PopoverTrigger
              render={
                <Button variant='outline' className='flex-1 sm:flex-none' />
              }
            >
              <Lightbulb className='h-4 w-4 sm:mr-2' />
              <span className='sm:inline'>{t('Templates')}</span>
            </PopoverTrigger>
            <PopoverContent className='w-60'>
              <div className='space-y-2'>
                <p className='text-muted-foreground text-xs'>
                  {t('Quick insert payment entries')}
                </p>
                <div className='space-y-1'>
                  {paymentTemplates.map((item) => (
                    <Button
                      key={item.name}
                      type='button'
                      variant='ghost'
                      className='w-full justify-start text-sm'
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        handleInsertTemplate(item.template)
                      }}
                    >
                      <Plus className='mr-2 h-3 w-3' />
                      {item.name}
                    </Button>
                  ))}
                </div>
              </div>
            </PopoverContent>
          </Popover>
          <Button
            type='button'
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              handleAdd()
            }}
            className='flex-1 sm:flex-none'
          >
            <Plus className='h-4 w-4 sm:mr-2' />
            <span className='sm:inline'>{t('Add method')}</span>
          </Button>
        </div>
      </div>

      {onAvailableIconsChange && (
        <div className='rounded-md border p-3'>
          <Popover>
            <PopoverTrigger
              render={<Button type='button' variant='outline' size='sm' />}
            >
              {t('Icon library')}
            </PopoverTrigger>
            <PopoverContent className='w-80'>
              <div className='space-y-3'>
                <div>
                  <p className='font-medium'>{t('Icon library')}</p>
                  <p className='text-muted-foreground text-xs'>
                    {t(
                      'Choose which icons are available when editing payment methods.'
                    )}
                  </p>
                </div>
                <div className='grid grid-cols-2 gap-2'>
                  {BUILT_IN_PAYMENT_ICONS.map((iconName) => {
                    const checked = availableIcons.includes(iconName)
                    return (
                      <label
                        key={iconName}
                        className='hover:bg-muted flex cursor-pointer items-center gap-2 rounded p-1.5 text-xs'
                      >
                        <Checkbox
                          checked={checked}
                          onCheckedChange={(next) => {
                            const selected = new Set(availableIcons)
                            if (next) selected.add(iconName)
                            else selected.delete(iconName)
                            onAvailableIconsChange(
                              serializeAvailablePaymentIcons([...selected])
                            )
                          }}
                        />
                        <ReactIconByName name={iconName} className='size-4' />
                        <span className='truncate'>{iconName}</span>
                      </label>
                    )
                  })}
                </div>
              </div>
            </PopoverContent>
          </Popover>
        </div>
      )}

      {filteredMethods.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed p-8 text-center text-sm'>
          {searchText
            ? t('No payment methods match your search')
            : t(
                'No payment methods configured. Click "Add method" or use templates to get started.'
              )}
        </div>
      ) : (
        <div className='rounded-md border'>
          {/* Desktop table view */}
          <StaticDataTable
            className='hidden rounded-none border-0 md:block'
            data={filteredMethods}
            getRowKey={(method, index) => `${method.type}-${index}`}
            columns={[
              {
                id: 'name',
                header: t('Name'),
                cellClassName: 'font-medium',
                cell: (method) => getPaymentMethodDisplayName(method),
              },
              {
                id: 'type',
                header: t('Payment type'),
                cell: (method) => getPaymentTypeDisplayName(method, t),
              },
              {
                id: 'icon',
                header: t('Icon'),
                cell: (method) => {
                  const iconName = getEffectiveIconName(method)

                  return iconName ? (
                    <div className='flex items-center gap-2'>
                      <ReactIconByName
                        name={iconName}
                        className='text-muted-foreground size-5 shrink-0'
                        title={getPaymentTypeDisplayName(method, t)}
                        aria-label={getPaymentTypeDisplayName(method, t)}
                      />
                    </div>
                  ) : (
                    <span className='text-muted-foreground text-sm'>—</span>
                  )
                },
              },
              {
                id: 'min-top-up',
                header: t('Min Top-up'),
                cell: (method) => {
                  const minimum = getPaymentMethodMinimumForDisplay(
                    method.type,
                    method.min_topup
                  )
                  return minimum ? (
                    <span className='font-mono text-sm'>{minimum}</span>
                  ) : (
                    <span className='text-muted-foreground text-sm'>—</span>
                  )
                },
              },
              {
                id: 'topup-group',
                header: t('Top-up coefficient group'),
                cell: (method) => method.topup_group || '—',
              },
              {
                id: 'pending-ttl',
                header: t('Payment TTL'),
                cell: (method) => (
                  <span className='font-mono text-sm'>
                    {getPaymentMethodPendingTtl(
                      method,
                      defaultPendingTtlMinutes
                    )}{' '}
                    {t('min')}
                  </span>
                ),
              },
              {
                id: 'actions',
                header: t('Actions'),
                className: 'text-right',
                cellClassName: 'text-right',
                cell: (method) => (
                  <StaticRowActions
                    editLabel={t('Edit')}
                    deleteLabel={t('Delete')}
                    menuLabel={t('Open menu')}
                    onEdit={() => handleEdit(method)}
                    onDelete={() => handleDelete(method)}
                  />
                ),
              },
            ]}
          />

          {/* Mobile card view */}
          <div className='divide-y md:hidden'>
            {filteredMethods.map((method) => {
              const iconName = getEffectiveIconName(method)
              const methodKey = [
                method.type,
                method.name,
                method.icon,
                method.min_topup,
                method.pending_ttl_minutes,
                method.color,
              ]
                .filter(Boolean)
                .join('-')

              return (
                <div key={methodKey} className='p-4'>
                  <div className='mb-3 flex items-start justify-between'>
                    <div className='flex-1'>
                      <div className='mb-1 font-medium'>
                        {getPaymentMethodDisplayName(method)}
                      </div>
                      <span className='text-muted-foreground text-xs'>
                        {getPaymentTypeDisplayName(method, t)}
                      </span>
                    </div>
                    <div className='flex gap-1'>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={(e) => {
                          e.preventDefault()
                          e.stopPropagation()
                          handleEdit(method)
                        }}
                      >
                        <Pencil className='h-4 w-4' />
                      </Button>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={(e) => {
                          e.preventDefault()
                          e.stopPropagation()
                          handleDelete(method)
                        }}
                      >
                        <Trash2 className='h-4 w-4' />
                      </Button>
                    </div>
                  </div>
                  <div className='space-y-2 text-sm'>
                    <div className='flex items-center gap-2'>
                      <span className='text-muted-foreground min-w-20'>
                        {t('Icon')}
                      </span>
                      {iconName ? (
                        <div className='flex min-w-0 items-center gap-2'>
                          <ReactIconByName
                            name={iconName}
                            className='text-muted-foreground size-5 shrink-0'
                            title={iconName}
                          />
                          <span className='text-muted-foreground truncate font-mono text-xs'>
                            {iconName}
                          </span>
                        </div>
                      ) : (
                        <span className='text-muted-foreground text-xs'>—</span>
                      )}
                    </div>
                    <div className='flex items-center gap-2'>
                      <span className='text-muted-foreground min-w-20'>
                        {t('Payment TTL')}
                      </span>
                      <span className='font-mono'>
                        {getPaymentMethodPendingTtl(
                          method,
                          defaultPendingTtlMinutes
                        )}{' '}
                        {t('min')}
                      </span>
                    </div>
                    {getPaymentMethodMinimumForDisplay(
                      method.type,
                      method.min_topup
                    ) && (
                      <div className='flex items-center gap-2'>
                        <span className='text-muted-foreground min-w-20'>
                          {t('Min Top-up:')}
                        </span>
                        <span className='font-mono'>
                          {getPaymentMethodMinimumForDisplay(
                            method.type,
                            method.min_topup
                          )}
                        </span>
                      </div>
                    )}
                    <div className='flex items-center gap-2'>
                      <span className='text-muted-foreground min-w-20'>
                        {t('Top-up coefficient group')}
                      </span>
                      <span>{method.topup_group || '—'}</span>
                    </div>
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      <PaymentMethodDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        topupGroups={topupGroups}
        waffoCurrency={waffoCurrency}
        defaultPendingTtlMinutes={defaultPendingTtlMinutes}
        availableIcons={availableIcons}
      />
    </div>
  )
}

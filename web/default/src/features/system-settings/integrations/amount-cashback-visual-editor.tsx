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
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useState, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { StaticRowActions } from '@/components/data-table/static/static-row-actions'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'

import { safeJsonParseWithValidation } from '../utils/json-parser'
import { isObjectRecord } from '../utils/json-validators'
import { formatAmountCashbackThreshold } from './amount-cashback'
import {
  AmountCashbackDialog,
  type AmountCashbackData,
} from './amount-cashback-dialog'

type AmountCashbackVisualEditorProps = {
  value: string
  onChange: (value: string) => void
  tokenAmounts: boolean
}

type ParsedAmountCashbacks = {
  cashbacks: AmountCashbackData[]
  thresholdMode: boolean
}

function parseAmountCashbacks(value: string): ParsedAmountCashbacks {
  let parsed: unknown
  try {
    parsed = JSON.parse(value || '{}')
  } catch {
    parsed = {}
  }

  if (Array.isArray(parsed)) {
    const cashbacks = parsed
      .map((item) => {
        if (!item || typeof item !== 'object') {
          return null
        }
        const record = item as Record<string, unknown>
        const amount = Number(record.min_amount)
        const cashbackPercent = Number(record.cashback_percent)
        if (!Number.isFinite(amount) || !Number.isFinite(cashbackPercent)) {
          return null
        }
        return {
          amount,
          cashbackPercent,
        }
      })
      .filter(
        (item): item is AmountCashbackData =>
          item !== null &&
          item.amount >= 0 &&
          item.cashbackPercent >= 0 &&
          item.cashbackPercent <= 100
      )
      .sort((a, b) => a.amount - b.amount)

    return { cashbacks, thresholdMode: true }
  }

  const parsedObject = safeJsonParseWithValidation<Record<string, unknown>>(
    value,
    {
      fallback: {},
      validator: isObjectRecord,
      validatorMessage: 'Amount cashback must be a JSON object',
      context: 'amount cashback',
    }
  )
  const cashbacks = Object.entries(parsedObject)
    .map(([amount, rate]) => ({
      amount: Number.parseInt(amount, 10),
      cashbackPercent:
        typeof rate === 'number' ? rate : Number.parseFloat(String(rate)),
    }))
    .filter(
      (item) =>
        Number.isFinite(item.amount) &&
        Number.isFinite(item.cashbackPercent) &&
        item.amount >= 0 &&
        item.cashbackPercent >= 0 &&
        item.cashbackPercent <= 100
    )
    .sort((a, b) => a.amount - b.amount)

  return { cashbacks, thresholdMode: false }
}

function stringifyAmountCashbacks(
  cashbacks: AmountCashbackData[],
  _thresholdMode: boolean
): string {
  const sorted = [...cashbacks].sort((a, b) => a.amount - b.amount)
  return JSON.stringify(
    sorted.map((item) => ({
      min_amount: item.amount,
      cashback_percent: item.cashbackPercent,
    })),
    null,
    2
  )
}

export function AmountCashbackVisualEditor({
  value,
  onChange,
  tokenAmounts,
}: AmountCashbackVisualEditorProps) {
  const { t } = useTranslation()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editData, setEditData] = useState<AmountCashbackData | null>(null)

  const parsedCashbacks = useMemo(() => parseAmountCashbacks(value), [value])
  const cashbacks = parsedCashbacks.cashbacks

  const handleSave = (data: AmountCashbackData) => {
    const nextCashbacks = cashbacks.filter((item) => {
      if (editData && editData.amount !== data.amount) {
        return item.amount !== editData.amount && item.amount !== data.amount
      }
      return item.amount !== data.amount
    })

    nextCashbacks.push(data)

    onChange(
      stringifyAmountCashbacks(nextCashbacks, parsedCashbacks.thresholdMode)
    )
  }

  const handleDelete = (amount: number) => {
    onChange(
      stringifyAmountCashbacks(
        cashbacks.filter((item) => item.amount !== amount),
        parsedCashbacks.thresholdMode
      )
    )
  }

  const handleEdit = (cashback: AmountCashbackData) => {
    setEditData(cashback)
    setDialogOpen(true)
  }

  const handleAdd = () => {
    setEditData(null)
    setDialogOpen(true)
  }

  const formatPercentage = (percent: number) => {
    return `${percent}%`
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between'>
        <p className='text-muted-foreground text-sm'>
          {t('Configure cashback rates based on recharge amounts')}
        </p>
        <Button
          type='button'
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            handleAdd()
          }}
          size='sm'
          className='w-full sm:w-auto'
        >
          <Plus className='h-4 w-4 sm:mr-2' />
          <span className='sm:inline'>{t('Add cashback tier')}</span>
        </Button>
      </div>

      {cashbacks.length === 0 ? (
        <div className='text-muted-foreground rounded-lg border border-dashed p-6 text-center text-sm'>
          {t(
            'No cashback tiers configured. Click "Add cashback tier" to get started.'
          )}
        </div>
      ) : (
        <div className='rounded-md border'>
          {/* Desktop table view */}
          <StaticDataTable
            className='hidden rounded-none border-0 sm:block'
            data={cashbacks}
            getRowKey={(cashback) => cashback.amount}
            columns={[
              {
                id: 'amount',
                header: tokenAmounts
                  ? t('Recharge Amount (Tokens)')
                  : t('Recharge Amount (USD)'),
                cell: (cashback) => (
                  <span className='font-mono text-sm'>
                    {formatAmountCashbackThreshold(
                      cashback.amount,
                      tokenAmounts
                    )}
                  </span>
                ),
              },
              {
                id: 'cashback',
                header: t('Cashback'),
                cell: (cashback) => (
                  <StatusBadge
                    variant={cashback.cashbackPercent > 0 ? 'info' : 'neutral'}
                    className='font-mono'
                    copyable={false}
                  >
                    {formatPercentage(cashback.cashbackPercent)}
                  </StatusBadge>
                ),
              },
              {
                id: 'actions',
                header: t('Actions'),
                className: 'text-right',
                cellClassName: 'text-right',
                cell: (cashback) => (
                  <StaticRowActions
                    editLabel={t('Edit')}
                    deleteLabel={t('Delete')}
                    menuLabel={t('Open menu')}
                    onEdit={() => handleEdit(cashback)}
                    onDelete={() => handleDelete(cashback.amount)}
                  />
                ),
              },
            ]}
          />

          {/* Mobile card view */}
          <div className='divide-y sm:hidden'>
            {cashbacks.map((cashback) => (
              <div key={cashback.amount} className='p-4'>
                <div className='mb-3 flex items-start justify-between'>
                  <div className='flex-1'>
                    <div className='text-muted-foreground mb-1 text-xs'>
                      {tokenAmounts
                        ? t('Recharge Amount (Tokens)')
                        : t('Recharge Amount (USD)')}
                    </div>
                    <div className='mb-2 font-mono text-base font-medium'>
                      {formatAmountCashbackThreshold(
                        cashback.amount,
                        tokenAmounts
                      )}
                    </div>
                    <div className='flex items-center gap-2 text-sm'>
                      <span className='text-muted-foreground'>
                        {t('Cashback')}:
                      </span>
                      <StatusBadge
                        variant={
                          cashback.cashbackPercent > 0 ? 'info' : 'neutral'
                        }
                        className='font-mono'
                        copyable={false}
                      >
                        {formatPercentage(cashback.cashbackPercent)}
                      </StatusBadge>
                    </div>
                  </div>
                  <div className='flex gap-1'>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      aria-label={t('Edit cashback tier')}
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        handleEdit(cashback)
                      }}
                    >
                      <Pencil className='h-4 w-4' />
                    </Button>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      aria-label={t('Delete cashback tier')}
                      onClick={(e) => {
                        e.preventDefault()
                        e.stopPropagation()
                        handleDelete(cashback.amount)
                      }}
                    >
                      <Trash2 className='h-4 w-4' />
                    </Button>
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      <AmountCashbackDialog
        open={dialogOpen}
        onOpenChange={setDialogOpen}
        onSave={handleSave}
        editData={editData}
        tokenAmounts={tokenAmounts}
      />
    </div>
  )
}
